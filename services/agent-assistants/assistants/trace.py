"""Agent traces: what an assistant did during one request, and why.

Same schema as pkg/agenttrace in Go; both are sent to the agent manager
(infrastructure/agent-manager). Tracing is best-effort: with no AGENT_TRACE_URL, or
when the manager is slow or down, requests are unaffected.

Traces hold bounded facts only (names, counts, hashes, short reasons). Never
put the raw order text, bearer tokens or provider keys in a step.
"""

import asyncio
import json
import logging
import uuid
from datetime import datetime, timezone

import httpx

KIND_DECISION = "decision"
KIND_LLM = "llm"
KIND_TOOL = "tool"
KIND_GUARDRAIL = "guardrail"
KIND_ERROR = "error"

MAX_STEPS = 100
MAX_REASON = 400
MAX_NAME = 120
MAX_DETAIL_KEYS = 16
MAX_DETAIL_STRING = 200
MAX_IN_FLIGHT = 64
_SECRET_WORDS = ("token", "secret", "password", "authorization", "apikey", "api_key", "accesskey", "signedurl")

logger = logging.getLogger("agent-assistants")


def _now() -> str:
    return datetime.now(timezone.utc).isoformat()


def _clip(value, limit: int) -> str:
    return str(value)[:limit]


def _detail(raw: dict | None) -> dict | None:
    if not raw:
        return None
    out = {}
    for key, value in raw.items():
        if len(out) >= MAX_DETAIL_KEYS:
            break
        if any(word in str(key).lower() for word in _SECRET_WORDS):
            continue
        if value is None or isinstance(value, (bool, int, float)):
            out[_clip(key, MAX_NAME)] = value
        elif isinstance(value, str):
            out[_clip(key, MAX_NAME)] = _clip(value, MAX_DETAIL_STRING)
        else:
            out[_clip(key, MAX_NAME)] = _clip(json.dumps(value, default=str, separators=(",", ":")), MAX_DETAIL_STRING)
    return out or None


class HttpSink:
    """Posts finished traces without blocking the request. Drops when too many are in flight."""

    def __init__(self, base_url: str, token: str = "", http: httpx.AsyncClient | None = None):
        self._url = base_url.rstrip("/") + "/api/v1/traces"
        self._headers = {"Content-Type": "application/json"}
        if token:
            self._headers["Authorization"] = f"Bearer {token}"
        self._http = http or httpx.AsyncClient(timeout=httpx.Timeout(3.0))
        self._pending: set[asyncio.Task] = set()

    def submit(self, trace: dict) -> None:
        if len(self._pending) >= MAX_IN_FLIGHT:
            return
        task = asyncio.get_running_loop().create_task(self._post(trace))
        self._pending.add(task)
        task.add_done_callback(self._pending.discard)

    async def _post(self, trace: dict) -> None:
        try:
            await self._http.post(self._url, json=trace, headers=self._headers)
        except httpx.HTTPError:
            logger.warning("agent_trace_unavailable")

    async def flush(self) -> None:
        if self._pending:
            await asyncio.gather(*list(self._pending), return_exceptions=True)

    async def aclose(self) -> None:
        await self.flush()
        await self._http.aclose()


class Recorder:
    """Collects steps for one request. Every method is a no-op when there is no sink."""

    def __init__(self, sink, agent: str, operation: str, correlation_id: str = "", actor_id: str = ""):
        self._sink = sink
        self._done = False
        self._trace = {
            "id": uuid.uuid4().hex[:16], "correlation_id": _clip(correlation_id, MAX_NAME), "agent": agent,
            "operation": operation, "actor_id": _clip(actor_id, MAX_NAME), "started_at": _now(), "status": "error", "steps": [],
        }

    def set_actor(self, actor_id: str) -> None:
        self._trace["actor_id"] = _clip(actor_id, MAX_NAME)

    def input(self, digest: str, chars: int) -> None:
        self._trace["input_hash"], self._trace["input_chars"] = digest, chars

    def add(self, kind: str, name: str, reason: str, outcome: str = "", detail: dict | None = None) -> None:
        steps = self._trace["steps"]
        if self._sink is None or self._done or len(steps) >= MAX_STEPS:
            return
        step = {"seq": len(steps) + 1, "at": _now(), "kind": kind, "name": _clip(name, MAX_NAME),
                "reason": _clip(reason, MAX_REASON), "outcome": _clip(outcome, MAX_NAME)}
        cleaned = _detail(detail)
        if cleaned:
            step["detail"] = cleaned
        steps.append(step)

    def finish(self, status: str) -> None:
        if self._sink is None or self._done:
            return
        self._done = True
        self._trace["status"] = status
        self._trace["ended_at"] = _now()
        self._sink.submit(self._trace)


NOOP = Recorder(None, "", "")


def of(config) -> Recorder:
    """The recorder for this graph run, or a no-op one when the caller did not pass any."""
    return (config or {}).get("configurable", {}).get("trace") or NOOP
