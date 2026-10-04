"""HTTP surface of the order and dashboard assistants (A1, A2).

Same edge contract as the Go services: /health/live, /health/ready, /metrics,
X-Correlation-ID, security headers, problem+json errors, a request body limit,
and route-template metric labels.
"""

import json
import logging
import time
import uuid
from contextlib import asynccontextmanager
from datetime import datetime

from fastapi import FastAPI, Request
from fastapi.responses import JSONResponse, Response
from prometheus_client import CONTENT_TYPE_LATEST, generate_latest

from . import audit, config as config_module, telemetry, trace as trace_module
from .auth import Authenticator, AuthError, Profile, bearer_token, resolve_profile
from .business import BusinessClient, BusinessError
from .dashboard_assistant import graph as dashboard_graph
from .dashboard_assistant.spec import clean_spec
from .llm import Disabled, LLMUnavailable, OpenAICompatible
from .order_assistant import graph as order_graph
from .order_assistant.resolve import COLOMBO

SERVICE = "agent-assistants"
MAX_BODY_BYTES = 32 << 10
MAX_ORDER_TEXT = 2000
MAX_DASHBOARD_TEXT = 1000
STORE_MANAGER = "STORE_MANAGER"

logging.basicConfig(level=logging.INFO, format="%(message)s")
logging.getLogger("httpx").setLevel(logging.WARNING)


def problem(status: int, title: str, detail: str) -> JSONResponse:
    return JSONResponse({"type": "about:blank", "title": title, "status": status, "detail": detail},
                        status_code=status, media_type="application/problem+json")


def create_app(cfg=None, *, authenticator=None, business=None, llm=None, trace_sink=None) -> FastAPI:
    cfg = cfg or config_module.load()
    state = {
        "auth": authenticator or Authenticator(cfg.oidc_jwks_url, cfg.oidc_issuer, cfg.oidc_audience),
        "business": business or BusinessClient(cfg.shared_service_url, cfg.order_service_url),
        "llm": llm or (OpenAICompatible(cfg.llm_base_url, cfg.llm_model, cfg.llm_api_key) if cfg.llm_configured else Disabled()),
        "order_graph": order_graph.build_graph(),
        "dashboard_graph": dashboard_graph.build_graph(),
        # None turns tracing off; the assistants never depend on the agent manager.
        "trace": trace_sink or (trace_module.HttpSink(cfg.agent_trace_url, cfg.agent_trace_token) if cfg.agent_trace_url else None),
    }

    @asynccontextmanager
    async def lifespan(_app):
        yield
        await state["business"].aclose()
        await state["llm"].aclose()
        if state["trace"] is not None and hasattr(state["trace"], "aclose"):
            await state["trace"].aclose()

    app = FastAPI(title="Waypoint order and dashboard assistants", docs_url=None, redoc_url=None, openapi_url=None, lifespan=lifespan)

    @app.middleware("http")
    async def edge(request: Request, call_next):
        correlation_id = request.headers.get("X-Correlation-ID") or str(uuid.uuid4())
        request.state.correlation_id = correlation_id
        started = time.perf_counter()
        length = request.headers.get("content-length")
        if length and length.isdigit() and int(length) > MAX_BODY_BYTES:
            response = problem(413, "Payload Too Large", "request body is too large")
        else:
            try:
                response = await call_next(request)
            except Exception:  # noqa: BLE001 - mirror chi Recoverer
                logging.getLogger(SERVICE).exception("unhandled_error")
                response = problem(500, "Internal Server Error", "unexpected error")
        route = request.scope.get("route")
        path = getattr(route, "path", "unmatched")
        status = str(response.status_code)
        telemetry.REQUESTS.labels(SERVICE, request.method, path, status).inc()
        telemetry.LATENCY.labels(SERVICE, request.method, path).observe(time.perf_counter() - started)
        if response.status_code >= 400:
            telemetry.ERRORS.labels(SERVICE, request.method, path, status).inc()
        response.headers["X-Correlation-ID"] = correlation_id
        response.headers["X-Request-ID"] = correlation_id
        response.headers["X-Content-Type-Options"] = "nosniff"
        response.headers["X-Frame-Options"] = "DENY"
        response.headers["Referrer-Policy"] = "same-origin"
        response.headers["Cache-Control"] = "no-store"
        return response

    @app.get("/health/live")
    async def live():
        return {"status": "live", "service": SERVICE}

    @app.get("/health/ready")
    async def ready():
        return {"status": "ready", "service": SERVICE}

    @app.get("/metrics")
    async def metrics():
        return Response(generate_latest(), media_type=CONTENT_TYPE_LATEST)

    async def store_manager(request: Request) -> tuple[Profile, str]:
        token = bearer_token(request.headers.get("Authorization"))
        subject = await state["auth"].subject(token)
        profile = await resolve_profile(state["business"], token, subject)
        if not profile.has_role(STORE_MANAGER) or not profile.outlet_ids:
            raise AuthError(403, "Store Manager with an outlet required")
        return profile, token

    async def json_body(request: Request) -> dict:
        raw = await request.body()
        if len(raw) > MAX_BODY_BYTES:
            raise ValueError("request body is too large")
        body = json.loads(raw or b"{}")
        if not isinstance(body, dict):
            raise ValueError("JSON object required")
        return body

    def text_field(body: dict, limit: int) -> str:
        message = body.get("message")
        if not isinstance(message, str) or not message.strip() or len(message) > limit:
            raise ValueError(f"message must contain 1-{limit} characters")
        return message.strip()

    async def run(assistant: str, request: Request, handler):
        started = time.perf_counter()
        # The status endpoint is a health-style probe, not an agent decision, so it is not traced.
        rec = (trace_module.Recorder(state["trace"], f"{assistant}-assistant", "draft", request.state.correlation_id)
               if assistant != "status" else trace_module.NOOP)
        request.state.trace = rec
        outcome = "error"
        try:
            result = await handler()
            telemetry.ASSISTANT_REQUESTS.labels(assistant, "ok").inc()
            outcome = "ok"
            return result
        except AuthError as exc:
            telemetry.ASSISTANT_REQUESTS.labels(assistant, "denied").inc()
            rec.add(trace_module.KIND_GUARDRAIL, "authorize", f"Refused: {exc}.", "denied")
            outcome = "denied"
            return problem(exc.status, "Unauthorized" if exc.status == 401 else "Forbidden", str(exc))
        except (ValueError, json.JSONDecodeError) as exc:
            telemetry.ASSISTANT_REQUESTS.labels(assistant, "invalid").inc()
            rec.add(trace_module.KIND_GUARDRAIL, "validate_request", f"The request was rejected before any model call: {exc}.", "rejected")
            return problem(400, "Bad Request", str(exc))
        except LLMUnavailable:
            telemetry.ASSISTANT_REQUESTS.labels(assistant, "unavailable").inc()
            rec.add(trace_module.KIND_ERROR, "llm_unavailable",
                    "The model provider is not configured or failed, so the person is told to fill in the form by hand instead of getting a guess.", "error")
            audit.publish(audit.ACTION_AGENT_FAILED, "", request.state.correlation_id, {"assistant": assistant, "stage": "provider"})
            return problem(503, "Service Unavailable", "agent_unavailable: fill in the form by hand")
        except BusinessError as exc:
            telemetry.ASSISTANT_REQUESTS.labels(assistant, "business_error").inc()
            status = exc.status if exc.status in (401, 403, 404) else 502
            rec.add(trace_module.KIND_ERROR, "business_service", "A read-only call to a business service was denied or failed, so no draft was produced.", f"HTTP {exc.status}")
            titles = {401: "Unauthorized", 403: "Forbidden", 404: "Not Found", 502: "Bad Gateway"}
            return problem(status, titles[status], "business service denied or rejected the request")
        finally:
            telemetry.ASSISTANT_LATENCY.labels(assistant).observe(time.perf_counter() - started)
            rec.finish(outcome)

    @app.get("/api/v1/agent/assistants/status")
    async def status(request: Request):
        async def handler():
            await store_manager(request)
            available = not isinstance(state["llm"], Disabled)
            return {"orderAssistant": available, "dashboardAssistant": available}
        return await run("status", request, handler)

    @app.post("/api/v1/agent/order-assistant/drafts")
    async def order_draft(request: Request):
        async def handler():
            profile, token = await store_manager(request)
            message = text_field(await json_body(request), MAX_ORDER_TEXT)
            request.state.trace.set_actor(profile.user_id)
            request.state.trace.input(audit.text_hash(message), len(message))
            result = await state["order_graph"].ainvoke(
                {"message": message, "today": datetime.now(COLOMBO).date(), "token": token,
                 "correlation_id": request.state.correlation_id, "outlet_id": profile.outlet_ids[0]},
                config={"configurable": {"business": state["business"], "llm": state["llm"], "trace": request.state.trace}},
            )
            body = order_graph.response(result)
            audit.publish(audit.ACTION_ORDER_DRAFTED, profile.user_id, request.state.correlation_id, {
                "textHash": audit.text_hash(message), "lines": len(body["lines"]),
                "questions": len(body["questions"]), "copiedOrder": (body["previousOrder"] or {}).get("orderRef", ""),
                "catalogVersion": body["catalogVersion"],
            }, resource_id=profile.outlet_ids[0])
            return body
        return await run("order", request, handler)

    @app.post("/api/v1/agent/dashboard-assistant/drafts")
    async def dashboard_draft(request: Request):
        async def handler():
            profile, _token = await store_manager(request)
            body = await json_body(request)
            message = text_field(body, MAX_DASHBOARD_TEXT)
            request.state.trace.set_actor(profile.user_id)
            request.state.trace.input(audit.text_hash(message), len(message))
            draft, _ = clean_spec(body.get("draft"), "")
            result = await state["dashboard_graph"].ainvoke(
                {"message": message, "draft": draft, "locale": body.get("locale") if body.get("locale") in ("en", "si", "ta") else "en"},
                config={"configurable": {"llm": state["llm"], "trace": request.state.trace}},
            )
            out = dashboard_graph.response(result)
            audit.publish(audit.ACTION_DASHBOARD_DRAFTED, profile.user_id, request.state.correlation_id, {
                "textHash": audit.text_hash(message), "cards": out["draft"]["cards"], "filter": out["draft"]["filter"],
                "asked": bool(out["question"]), "rejected": len(out["rejected"]),
            })
            return out
        return await run("dashboard", request, handler)

    return app


def main() -> None:
    import uvicorn

    uvicorn.run(create_app(), host="0.0.0.0", port=8080, log_level="warning", access_log=False, proxy_headers=True)


if __name__ == "__main__":
    main()
