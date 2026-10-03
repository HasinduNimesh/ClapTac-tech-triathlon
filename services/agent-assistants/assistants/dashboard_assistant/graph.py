"""A2 Dashboard assistant: a plain request -> a dashboard draft for the live preview.

    interpret (LLM) -> validate (code) -> END

The conversation is stateless: the browser sends the current draft with each
message, so Cancel leaves nothing behind. Saving is a separate call the person
makes to shared-service; this graph never saves, and never reads business data.
"""

import json
from typing import TypedDict

from langgraph.graph import END, START, StateGraph

from ..llm import UNTRUSTED_NOTE
from .spec import CARDS, FILTERS, MAX_NAME, clean_spec


class DashboardState(TypedDict, total=False):
    message: str
    draft: dict
    locale: str
    proposal: dict
    spec: dict
    question: str | None
    reply: str
    rejected: list[str]


FUNCTION = {
    "name": "update_dashboard",
    "description": "Return the complete dashboard after applying the request to the current draft.",
    "parameters": {
        "type": "object",
        "additionalProperties": False,
        "required": ["name", "cards", "filter", "question", "reply"],
        "properties": {
            "name": {"type": "string", "maxLength": MAX_NAME, "description": "Short dashboard name, e.g. Receipts and shortages."},
            "cards": {"type": "array", "maxItems": len(CARDS), "items": {"type": "string", "enum": list(CARDS)},
                      "description": "Card ids in display order."},
            "filter": {"type": "string", "enum": list(FILTERS), "description": "Which goods the cards cover."},
            "question": {"type": ["string", "null"], "description": "One short question only if the request is unclear, otherwise null."},
            "reply": {"type": "string", "maxLength": 300, "description": "One plain sentence saying what changed."},
        },
    },
}

SYSTEM = (
    "You build a store manager's dashboard by choosing and ordering cards from a fixed list. You cannot create "
    "any other kind of card or show anything else. Keep the cards in the order the person wants; when they ask to "
    "move or remove cards, return the whole list in the new order. If it is unclear whether they mean all goods "
    "or chilled only, keep the best draft and ask exactly one short question. Never claim you saved anything: the "
    "person saves with the Save dashboard button.\n"
    "Cards: " + "; ".join(f"{k} = {v}" for k, v in CARDS.items()) + ".\n"
    "filter is all, chilled or ambient. " + UNTRUSTED_NOTE
)

LANGUAGES = {"en": "English", "si": "Sinhala", "ta": "Tamil"}


async def interpret(state: DashboardState, config) -> dict:
    llm = config["configurable"]["llm"]
    draft = state.get("draft") or {"name": "", "cards": [], "filter": "all"}
    user = "Current draft (JSON):\n" + json.dumps(draft, separators=(",", ":")) + "\n\nRequest:\n" + state["message"]
    language = LANGUAGES.get(state.get("locale", "en"), "English")
    system = f"{SYSTEM}\nWrite the name, reply and question in plain {language}."
    proposal = await llm.call_function(system, user, FUNCTION)
    return {"proposal": proposal}


def validate(state: DashboardState) -> dict:
    proposal = state.get("proposal") or {}
    fallback = (state.get("draft") or {}).get("name") or "New dashboard"
    spec, rejected = clean_spec(proposal, fallback)
    question = proposal.get("question")
    question = " ".join(str(question).split())[:200] if question else None
    if not spec["cards"] and not question:
        question = "What would you like to keep an eye on, for example receipts to confirm or items that arrived short?"
    reply = " ".join(str(proposal.get("reply") or "").split())[:300] or "Here is your draft."
    return {"spec": spec, "question": question, "reply": reply, "rejected": rejected}


def build_graph():
    graph = StateGraph(DashboardState)
    graph.add_node("interpret", interpret)
    graph.add_node("validate", validate)
    graph.add_edge(START, "interpret")
    graph.add_edge("interpret", "validate")
    graph.add_edge("validate", END)
    return graph.compile()


def response(state: DashboardState) -> dict:
    return {
        "draft": state["spec"],
        "question": state.get("question"),
        "reply": state.get("reply", ""),
        "rejected": state.get("rejected", []),
        "saved": False,
    }
