import asyncio
import json
import unittest

import httpx
from fastapi.testclient import TestClient

from assistants import trace
from assistants.app import create_app
from assistants.llm import Disabled

from .fakes import CONFIG, FakeAuth, FakeBusiness, FakeLLM, PROFILES, extraction, item
from .test_dashboard_assistant import RECEIPTS


class MemSink:
    def __init__(self):
        self.traces = []

    def submit(self, t):
        self.traces.append(t)


def client(llm, sink):
    app = create_app(CONFIG, authenticator=FakeAuth(), business=FakeBusiness(PROFILES), llm=llm, trace_sink=sink)
    return TestClient(app)


def names(t):
    return [s["name"] for s in t["steps"]]


class AssistantTraceTest(unittest.TestCase):
    def test_order_draft_records_why_without_the_persons_words(self):
        sink = MemSink()
        llm = FakeLLM(extraction([item("zzmarker rice 10 bags", "rice", 10), item("zzmarker widgets", "unknown")]))
        r = client(llm, sink).post("/api/v1/agent/order-assistant/drafts", json={"message": "zzmarker rice 10 bags and widgets"},
                                   headers={"Authorization": "Bearer store", "X-Correlation-ID": "corr-7"})
        self.assertEqual(r.status_code, 200, r.text)
        self.assertEqual(len(sink.traces), 1)
        t = sink.traces[0]
        self.assertEqual((t["agent"], t["operation"], t["status"], t["actor_id"], t["correlation_id"]),
                         ("order-assistant", "draft", "ok", "u-store", "corr-7"))
        self.assertEqual(names(t), ["load_context", "read_order_text", "match item 1", "match item 2", "draft ready"])
        outcomes = [s["outcome"] for s in t["steps"]]
        self.assertEqual(outcomes[2], "matched")
        self.assertEqual(outcomes[3], "asked: not_in_catalog")
        self.assertTrue(all(s["reason"] for s in t["steps"]))
        self.assertEqual(t["input_chars"], len("zzmarker rice 10 bags and widgets"))
        self.assertNotIn("zzmarker", json.dumps(t))
        self.assertNotIn("Bearer", json.dumps(t))

    def test_dashboard_records_dropped_cards(self):
        sink = MemSink()
        bad = dict(RECEIPTS, cards=["receipts", "sql"])
        r = client(FakeLLM(bad), sink).post("/api/v1/agent/dashboard-assistant/drafts", json={"message": "receipts"},
                                            headers={"Authorization": "Bearer store"})
        self.assertEqual(r.status_code, 200, r.text)
        t = sink.traces[0]
        self.assertEqual((t["agent"], t["status"]), ("dashboard-assistant", "ok"))
        self.assertEqual(names(t)[:2], ["update_dashboard", "validate_cards"])
        self.assertEqual(t["steps"][1]["outcome"], "cleaned")

    def test_denied_and_unavailable_requests_are_traced(self):
        sink = MemSink()
        c = client(FakeLLM(), sink)
        self.assertEqual(c.post("/api/v1/agent/order-assistant/drafts", json={"message": "rice"}, headers={"Authorization": "Bearer dispatcher"}).status_code, 403)
        self.assertEqual((sink.traces[0]["status"], sink.traces[0]["steps"][0]["name"]), ("denied", "authorize"))

        c = client(Disabled(), sink)
        self.assertEqual(c.post("/api/v1/agent/dashboard-assistant/drafts", json={"message": "x"}, headers={"Authorization": "Bearer store"}).status_code, 503)
        last = sink.traces[-1]
        self.assertEqual(last["status"], "error")
        self.assertEqual(last["steps"][-1]["name"], "llm_unavailable")

    def test_status_probe_is_not_traced_and_no_sink_is_fine(self):
        sink = MemSink()
        client(FakeLLM(), sink).get("/api/v1/agent/assistants/status", headers={"Authorization": "Bearer store"})
        self.assertEqual(sink.traces, [])
        r = client(FakeLLM(extraction([item("rice", "rice", 1)])), None).post(
            "/api/v1/agent/order-assistant/drafts", json={"message": "rice"}, headers={"Authorization": "Bearer store"})
        self.assertEqual(r.status_code, 200)


class RecorderTest(unittest.TestCase):
    def test_detail_is_bounded_and_secret_keys_dropped(self):
        sink = MemSink()
        rec = trace.Recorder(sink, "a", "op")
        rec.add(trace.KIND_TOOL, "x" * 500, "r" * 1000, "ok", {"bearerToken": "t", "apiKey": "k", "note": "n" * 1000, "nested": {"a": 1}, "count": 3})
        rec.finish("ok")
        rec.finish("error")
        self.assertEqual(len(sink.traces), 1)
        step = sink.traces[0]["steps"][0]
        self.assertEqual((len(step["name"]), len(step["reason"])), (trace.MAX_NAME, trace.MAX_REASON))
        self.assertEqual(set(step["detail"]), {"note", "nested", "count"})
        self.assertEqual(len(step["detail"]["note"]), trace.MAX_DETAIL_STRING)

    def test_http_sink_posts_with_bearer_and_survives_failures(self):
        seen = []

        def handler(request: httpx.Request):
            seen.append((str(request.url), request.headers.get("authorization"), json.loads(request.content)))
            return httpx.Response(202)

        async def scenario():
            sink = trace.HttpSink("http://manager:8085/", "tok", httpx.AsyncClient(transport=httpx.MockTransport(handler)))
            rec = trace.Recorder(sink, "order-assistant", "draft", "c1")
            rec.add(trace.KIND_DECISION, "n", "r")
            rec.finish("ok")
            await sink.flush()

            def boom(request):
                raise httpx.ConnectError("down")

            down = trace.HttpSink("http://manager:8085", "", httpx.AsyncClient(transport=httpx.MockTransport(boom)))
            rec = trace.Recorder(down, "a", "b")
            rec.finish("ok")
            await down.flush()  # must not raise

        asyncio.run(scenario())
        self.assertEqual(seen[0][0], "http://manager:8085/api/v1/traces")
        self.assertEqual(seen[0][1], "Bearer tok")
        self.assertEqual(seen[0][2]["correlation_id"], "c1")


if __name__ == "__main__":
    unittest.main()
