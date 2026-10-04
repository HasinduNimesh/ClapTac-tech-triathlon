import unittest

from fastapi.testclient import TestClient

from assistants.app import create_app
from assistants.llm import Disabled

from .fakes import CONFIG, FakeAuth, FakeBusiness, FakeLLM, PROFILES, extraction, item
from .test_dashboard_assistant import RECEIPTS


def client(llm=None, orders=None):
    business = FakeBusiness(PROFILES, orders)
    app = create_app(CONFIG, authenticator=FakeAuth(), business=business, llm=llm or FakeLLM())
    return TestClient(app), business


class AppTest(unittest.TestCase):
    def test_health_and_metrics(self):
        c, _ = client()
        self.assertEqual(c.get("/health/live").json()["service"], "agent-assistants")
        self.assertEqual(c.get("/health/ready").status_code, 200)
        self.assertIn("http_requests_total", c.get("/metrics").text)

    def test_requires_token_and_store_manager(self):
        c, _ = client()
        r = c.post("/api/v1/agent/order-assistant/drafts", json={"message": "rice"})
        self.assertEqual(r.status_code, 401)
        self.assertEqual(r.headers["content-type"], "application/problem+json")
        r = c.post("/api/v1/agent/order-assistant/drafts", json={"message": "rice"}, headers={"Authorization": "Bearer dispatcher"})
        self.assertEqual(r.status_code, 403)

    def test_order_draft_uses_callers_token_and_never_submits(self):
        llm = FakeLLM(extraction([item("rice 10 bags", "rice", 10)]))
        c, business = client(llm)
        r = c.post("/api/v1/agent/order-assistant/drafts", json={"message": "rice 10 bags"},
                   headers={"Authorization": "Bearer store", "X-Correlation-ID": "corr-1"})
        self.assertEqual(r.status_code, 200, r.text)
        self.assertEqual(r.headers["X-Correlation-ID"], "corr-1")
        self.assertFalse(r.json()["submitted"])
        self.assertEqual(r.json()["lines"][0]["quantity"], 10)
        self.assertEqual(set(business.tokens), {"store"})

    def test_message_is_validated(self):
        c, _ = client()
        headers = {"Authorization": "Bearer store"}
        self.assertEqual(c.post("/api/v1/agent/order-assistant/drafts", json={"message": " "}, headers=headers).status_code, 400)
        self.assertEqual(c.post("/api/v1/agent/order-assistant/drafts", json={"message": "x" * 2001}, headers=headers).status_code, 400)
        self.assertEqual(c.post("/api/v1/agent/order-assistant/drafts", content=b"[]", headers=headers).status_code, 400)
        self.assertEqual(c.post("/api/v1/agent/order-assistant/drafts", content=b"x" * 40000, headers=headers).status_code, 413)

    def test_provider_missing_returns_agent_unavailable(self):
        c, _ = client(Disabled())
        headers = {"Authorization": "Bearer store"}
        self.assertEqual(c.get("/api/v1/agent/assistants/status", headers=headers).json(), {"orderAssistant": False, "dashboardAssistant": False})
        r = c.post("/api/v1/agent/dashboard-assistant/drafts", json={"message": "receipts"}, headers=headers)
        self.assertEqual(r.status_code, 503)
        self.assertIn("agent_unavailable", r.json()["detail"])

    def test_dashboard_draft_cleans_incoming_draft(self):
        llm = FakeLLM(RECEIPTS)
        c, _ = client(llm)
        r = c.post("/api/v1/agent/dashboard-assistant/drafts", headers={"Authorization": "Bearer store"},
                   json={"message": "reorder", "draft": {"name": "x", "cards": ["sql", "receipts"]}})
        self.assertEqual(r.status_code, 200, r.text)
        self.assertNotIn("sql", llm.calls[0]["user"])
        self.assertEqual(len(r.json()["draft"]["cards"]), 4)


if __name__ == "__main__":
    unittest.main()
