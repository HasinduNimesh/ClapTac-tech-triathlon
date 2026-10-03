import asyncio
import unittest

from assistants.dashboard_assistant import graph as dashboard_graph
from assistants.dashboard_assistant.spec import CARDS, clean_spec

from .fakes import FakeLLM

RECEIPTS = {
    "name": "Receipts and shortages",
    "cards": ["short", "shortByWeek", "receipts", "deadlines"],
    "filter": "all",
    "question": "Do you want all goods, or chilled only?",
    "reply": "I built four cards for shortages and receipts.",
}


def run(proposal, draft=None, message="what arrived short this week and which receipts I still need to confirm", locale="en"):
    llm = FakeLLM(proposal)
    state = asyncio.run(dashboard_graph.build_graph().ainvoke(
        {"message": message, "draft": draft or {}, "locale": locale}, config={"configurable": {"llm": llm}}))
    return dashboard_graph.response(state), llm


class DashboardAssistantTest(unittest.TestCase):
    def test_demo_request_builds_four_cards(self):
        body, llm = run(RECEIPTS)
        self.assertEqual(body["draft"], {"name": "Receipts and shortages", "cards": ["short", "shortByWeek", "receipts", "deadlines"], "filter": "all"})
        self.assertEqual(body["question"], "Do you want all goods, or chilled only?")
        self.assertFalse(body["saved"])
        self.assertIn("Request:\nwhat arrived short", llm.calls[0]["user"])
        # The model may only choose from the web app's card ids.
        self.assertEqual(llm.calls[0]["function"]["parameters"]["properties"]["cards"]["items"]["enum"], list(CARDS))

    def test_follow_up_sends_current_draft_and_keeps_new_order(self):
        reordered = dict(RECEIPTS, cards=["deadlines", "short", "shortByWeek", "receipts"], question=None)
        body, llm = run(reordered, draft=clean_spec(RECEIPTS)[0], message="put the report-by deadlines first")
        self.assertEqual(body["draft"]["cards"][0], "deadlines")
        self.assertIn('"deadlines"', llm.calls[0]["user"])
        self.assertIsNone(body["question"])

    def test_unknown_cards_are_rejected_not_invented(self):
        body, _ = run({"name": "x", "cards": ["pie_chart", "other_store_sales", "receipts", "receipts"], "filter": "frozen", "question": None, "reply": "ok"})
        self.assertEqual(body["draft"]["cards"], ["receipts"])
        self.assertEqual(body["draft"]["filter"], "all")
        self.assertEqual(len(body["rejected"]), 2)

    def test_malformed_cards_are_rejected_without_crashing(self):
        body, _ = run({"name": "x", "cards": [{"type": "count"}, ["receipts"], 7, None, "short"], "filter": ["all"], "question": None, "reply": "ok"})
        self.assertEqual(body["draft"], {"name": "x", "cards": ["short"], "filter": "all"})
        self.assertEqual(len(body["rejected"]), 4)

    def test_name_is_trimmed_and_falls_back(self):
        spec, _ = clean_spec({"name": "  " + "n" * 200, "cards": ["chilled"]})
        self.assertEqual(len(spec["name"]), 60)
        spec, _ = clean_spec({"name": "", "cards": ["chilled"], "filter": "chilled"}, "Chilled")
        self.assertEqual(spec, {"name": "Chilled", "cards": ["chilled"], "filter": "chilled"})

    def test_replies_in_the_persons_language(self):
        _, llm = run(RECEIPTS, locale="si")
        self.assertIn("plain Sinhala", llm.calls[0]["system"])

    def test_empty_answer_asks_one_question(self):
        body, _ = run({"name": "", "cards": [], "filter": "all", "question": None, "reply": ""})
        self.assertEqual(body["draft"]["cards"], [])
        self.assertTrue(body["question"])


if __name__ == "__main__":
    unittest.main()
