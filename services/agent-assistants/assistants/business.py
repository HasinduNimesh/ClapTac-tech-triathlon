"""Read-only calls to business services, made with the caller's own token.

There is no database driver in this service. Each business service keeps
enforcing its own role and outlet scope, so the assistants can only see what
the signed-in person can already see.
"""

import httpx

MAX_RESPONSE_BYTES = 512 << 10


class BusinessError(Exception):
    def __init__(self, status: int, message: str):
        super().__init__(message)
        self.status = status


class BusinessClient:
    def __init__(self, shared_url: str, order_url: str, http: httpx.AsyncClient | None = None):
        self.shared_url = shared_url
        self.order_url = order_url
        self._http = http or httpx.AsyncClient(timeout=httpx.Timeout(5.0), follow_redirects=False)

    async def get_json(self, base_url: str, path: str, token: str, correlation_id: str = "") -> dict:
        if not base_url:
            raise BusinessError(503, "service URL is not configured")
        headers = {"Accept": "application/json", "Authorization": f"Bearer {token}"}
        if correlation_id:
            headers["X-Correlation-ID"] = correlation_id
        try:
            resp = await self._http.get(base_url + path, headers=headers)
        except httpx.HTTPError as exc:
            raise BusinessError(502, "business service unavailable") from exc
        if len(resp.content) > MAX_RESPONSE_BYTES:
            raise BusinessError(502, "business response too large")
        if resp.status_code != 200:
            raise BusinessError(resp.status_code, "business service rejected the request")
        try:
            body = resp.json()
        except ValueError as exc:
            raise BusinessError(502, "business service returned invalid JSON") from exc
        if not isinstance(body, dict):
            raise BusinessError(502, "business service returned an unexpected shape")
        return body

    async def outlet(self, token: str, outlet_id: str, correlation_id: str = "") -> dict:
        body = await self.get_json(self.shared_url, f"/api/v1/shared/outlets/{outlet_id}", token, correlation_id)
        return body.get("outlet") or {}

    async def orders(self, token: str, correlation_id: str = "") -> list[dict]:
        body = await self.get_json(self.order_url, "/api/v1/orders", token, correlation_id)
        items = body.get("items") or []
        return [o for o in items if isinstance(o, dict)]

    async def aclose(self) -> None:
        await self._http.aclose()
