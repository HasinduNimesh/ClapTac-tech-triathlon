from assistants.auth import AuthError
from assistants.config import Config

CONFIG = Config(environment="test", oidc_issuer="", oidc_audience="", oidc_jwks_url="", shared_service_url="http://shared",
                order_service_url="http://orders", llm_base_url="", llm_model="", llm_api_key="")


class FakeAuth:
    async def subject(self, token: str) -> str:
        if not token:
            raise AuthError(401, "missing bearer token")
        return token


class FakeLLM:
    def __init__(self, args: dict | None = None):
        self.args = args or {}
        self.calls = []

    async def call_function(self, system: str, user: str, function: dict) -> dict:
        self.calls.append({"system": system, "user": user, "function": function})
        return self.args

    async def aclose(self):
        return None


class FakeBusiness:
    shared_url = "http://shared"
    order_url = "http://orders"

    def __init__(self, profiles: dict, orders: list[dict] | None = None, brand: str = "Fresh"):
        self.profiles = profiles
        self.order_items = orders or []
        self.brand = brand
        self.tokens = []

    async def get_json(self, base_url, path, token, correlation_id=""):
        self.tokens.append(token)
        if path == "/api/v1/shared/profiles/me":
            return {"profile": self.profiles[token]}
        raise AssertionError(path)

    async def outlet(self, token, outlet_id, correlation_id=""):
        self.tokens.append(token)
        return {"id": outlet_id, "brand": self.brand}

    async def orders(self, token, correlation_id=""):
        self.tokens.append(token)
        return self.order_items

    async def aclose(self):
        return None


STORE = {"userId": "u-store", "subject": "store", "roles": ["STORE_MANAGER"], "outletIds": ["OUT034"]}
DISPATCHER = {"userId": "u-disp", "subject": "dispatcher", "roles": ["DISPATCHER"]}
PROFILES = {"store": STORE, "dispatcher": DISPATCHER}


def item(text, family, quantity=None, unit="pack", size=None, as_usual=False):
    return {"text": text, "family": family, "size": size, "quantity": quantity, "quantityUnit": unit, "asUsual": as_usual}


def extraction(items=(), kind="none", weekday=None, needed_by=None):
    return {"items": list(items), "copyPrevious": {"kind": kind, "weekday": weekday}, "neededBy": needed_by}
