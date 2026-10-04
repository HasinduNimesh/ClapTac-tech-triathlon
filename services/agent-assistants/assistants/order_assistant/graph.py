"""A1 Order assistant: casual order text -> draft order lines for the manager to confirm.

    load_context -> read_text (LLM) -> match_products (code) -> END

It never submits. The web form stays the only way to send an order, so the
normal Order received workflow (W1) and its cutoff rules still apply.
"""

from datetime import date
from typing import Any, TypedDict

from langgraph.graph import END, START, StateGraph

from ..llm import UNTRUSTED_NOTE
from .catalog import load_catalog
from .resolve import WEEKDAYS, group_drafts, pick_previous, resolve_item, resolve_needed_by

MAX_ITEMS = 25


class OrderState(TypedDict, total=False):
    message: str
    today: date
    token: str
    correlation_id: str
    outlet_id: str
    brand: str
    history: list[dict]
    extraction: dict
    lines: list[dict]
    questions: list[dict]
    previous_order: dict | None
    needed_by: str | None


def _function(families: list[str]) -> dict:
    return {
        "name": "read_order_text",
        "description": "Record what the store manager wrote, item by item. Do not guess quantities or sizes.",
        "parameters": {
            "type": "object",
            "additionalProperties": False,
            "required": ["items", "copyPrevious", "neededBy"],
            "properties": {
                "items": {
                    "type": "array",
                    "maxItems": MAX_ITEMS,
                    "items": {
                        "type": "object",
                        "additionalProperties": False,
                        "required": ["text", "family", "size", "quantity", "quantityUnit", "asUsual"],
                        "properties": {
                            "text": {"type": "string", "description": "The words the person used for this item, copied exactly."},
                            "family": {"type": "string", "enum": families + ["unknown"], "description": "Product family from the list, or unknown."},
                            "size": {"type": ["string", "null"], "description": "Only a size the person literally wrote, like '1 L' or '500 ml'. Null for vague words like 'big'."},
                            "quantity": {"type": ["number", "null"], "description": "Number the person wrote, or null."},
                            "quantityUnit": {"type": "string", "enum": ["pack", "item", "unspecified"], "description": "pack for crates, cases, bags, trays, cartons, bales, bundles; item for single bottles, cups, pieces."},
                            "asUsual": {"type": "boolean", "description": "True when the person said 'as usual' or similar instead of a quantity."},
                        },
                    },
                },
                "copyPrevious": {
                    "type": "object",
                    "additionalProperties": False,
                    "required": ["kind", "weekday"],
                    "properties": {
                        "kind": {"type": "string", "enum": ["none", "last_order", "weekday", "last_week"]},
                        "weekday": {"type": ["string", "null"], "enum": WEEKDAYS + [None]},
                    },
                },
                "neededBy": {"type": ["string", "null"], "description": "Delivery date YYYY-MM-DD only if the person named one."},
            },
        },
    }


SYSTEM = (
    "You read a store manager's order written the way they would say it to a colleague. "
    "Fill read_order_text only. Map each item to one family from the allowed list or 'unknown'. "
    "Copy sizes and numbers exactly as written; never fill in a size, quantity or product yourself. "
    "'Same as last Tuesday' is copyPrevious weekday tuesday; 'same as last week' is last_week; "
    "'same as last time' is last_order. " + UNTRUSTED_NOTE
)


async def load_context(state: OrderState, config) -> dict:
    business = config["configurable"]["business"]
    outlet = await business.outlet(state["token"], state["outlet_id"], state.get("correlation_id", ""))
    orders = await business.orders(state["token"], state.get("correlation_id", ""))
    # Only this outlet's orders are returned for a Store Manager; keep the last few.
    own = [o for o in orders if o.get("outletId") == state["outlet_id"]]
    own.sort(key=lambda o: str(o.get("createdAt", "")), reverse=True)
    return {"brand": str(outlet.get("brand", "")), "history": own[:8]}


async def read_text(state: OrderState, config) -> dict:
    llm = config["configurable"]["llm"]
    catalog = load_catalog()
    families = catalog.families(state["brand"])
    listing = "; ".join(
        f"{family} ({', '.join(p.name + ' per ' + p.pack + ' of ' + str(p.unitsPerPack) for p in products)})"
        for family, products in families.items()
    )
    today = state["today"]
    system = f"{SYSTEM}\nToday is {WEEKDAYS[today.weekday()]} {today.isoformat()}.\nAllowed families: {listing}"
    args = await llm.call_function(system, state["message"], _function(sorted(families)))
    return {"extraction": args}


def match_products(state: OrderState) -> dict:
    catalog = load_catalog()
    families = catalog.families(state["brand"])
    extraction: dict[str, Any] = state.get("extraction") or {}
    lines, questions = [], []
    items = extraction.get("items") if isinstance(extraction.get("items"), list) else []
    for item in items[:MAX_ITEMS]:
        if not isinstance(item, dict):
            continue
        line, question = resolve_item(item, families)
        if line:
            lines.append(line)
        if question:
            questions.append(question)
    previous, missing = pick_previous(state.get("history", []), extraction.get("copyPrevious"), state["today"])
    needed_by, bad_date = resolve_needed_by(extraction.get("neededBy"), state["today"])
    questions += [q for q in (missing, bad_date) if q]
    return {"lines": lines, "questions": questions, "previous_order": previous, "needed_by": needed_by}


def build_graph():
    graph = StateGraph(OrderState)
    graph.add_node("load_context", load_context)
    graph.add_node("read_text", read_text)
    graph.add_node("match_products", match_products)
    graph.add_edge(START, "load_context")
    graph.add_edge("load_context", "read_text")
    graph.add_edge("read_text", "match_products")
    graph.add_edge("match_products", END)
    return graph.compile()


def response(state: OrderState) -> dict:
    catalog = load_catalog()
    lines = state.get("lines", [])
    return {
        "drafts": group_drafts(lines),
        "lines": lines,
        "questions": state.get("questions", []),
        "previousOrder": state.get("previous_order"),
        "neededBy": state.get("needed_by"),
        "products": [p.public() for p in catalog.products(state.get("brand", ""))],
        "catalogVersion": catalog.version,
        "submitted": False,
    }
