"""Environment configuration. Names match the Go services and docker-compose."""

import os
from dataclasses import dataclass


def _env(name: str, default: str = "") -> str:
    return os.environ.get(name, default).strip()


@dataclass(frozen=True)
class Config:
    environment: str
    oidc_issuer: str
    oidc_audience: str
    oidc_jwks_url: str
    shared_service_url: str
    order_service_url: str
    llm_base_url: str
    llm_model: str
    llm_api_key: str
    agent_trace_url: str = ""
    agent_trace_token: str = ""

    @property
    def llm_configured(self) -> bool:
        return bool(self.llm_base_url and self.llm_model)


def load() -> Config:
    if _env("AUTH_DISABLED", "false").lower() == "true":
        # The Go services only allow this locally; the assistants never allow it
        # because every business call must carry the signed-in user's token.
        raise RuntimeError("AUTH_DISABLED is not supported by agent-assistants")
    return Config(
        environment=_env("ENVIRONMENT", "local"),
        oidc_issuer=_env("OIDC_ISSUER"),
        oidc_audience=_env("OIDC_AUDIENCE"),
        oidc_jwks_url=_env("OIDC_JWKS_URL"),
        shared_service_url=_env("SHARED_SERVICE_URL").rstrip("/"),
        order_service_url=_env("ORDER_SERVICE_URL").rstrip("/"),
        llm_base_url=_env("LLM_BASE_URL"),
        llm_model=_env("LLM_MODEL"),
        llm_api_key=_env("LLM_API_KEY"),
        agent_trace_url=_env("AGENT_TRACE_URL").rstrip("/"),
        agent_trace_token=_env("AGENT_TRACE_INGEST_TOKEN"),
    )
