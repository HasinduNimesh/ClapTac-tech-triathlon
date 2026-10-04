"""A1 Order assistant: casual order text -> draft order lines for the manager to confirm.

    load_context -> read_text (LLM) -> match_products (code) -> END

It never submits. The web form stays the only way to send an order, so the
normal Order received workflow (W1) and its cutoff rules still apply.
"""

from datetime import date
from typing import Any, TypedDict

from langgraph.graph import END, START, StateGraph

from .. import trace
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
    trace.of(config).add(
        trace.KIND_TOOL, "load_context",
        "Read the outlet's brand to pick the right product catalog, and its recent orders so 'same as last time' can be resolved. "
        "Both reads use the caller's own token.",
        "ok", {"brand": str(outlet.get("brand", "")), "recentOrders": len(own[:8])})
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
    items = args.get("items") if isinstance(args.get("items"), list) else []
    trace.of(config).add(
        trace.KIND_LLM, "read_order_text",
        "The model only transcribes what was written (product family, literal size, quantity) into one fixed function. "
        "It does not choose products or quantities; code does that next.",
        f"{len(items)} item(s) read", {"items": len(items), "copyPrevious": str((args.get("copyPrevious") or {}).get("kind", "none")),
                                       "namedDate": bool(args.get("neededBy"))})
    return {"extraction": args}


def match_products(state: OrderState, config) -> dict:
    catalog = load_catalog()
    families = catalog.families(state["brand"])
    extraction: dict[str, Any] = state.get("extraction") or {}
    rec = trace.of(config)
    lines, questions = [], []
    items = extraction.get("items") if isinstance(extraction.get("items"), list) else []
    for number, item in enumerate(items[:MAX_ITEMS], start=1):
        if not isinstance(item, dict):
            continue
        line, question = resolve_item(item, families)
        if line:
            lines.append(line)
        if question:
            questions.append(question)
        family = str(item.get("family") or "unknown")
        # Only the family (from the fixed catalog list) and product name are recorded, never the person's words.
        reason, outcome, detail = _item_reason(line, question, family if family in families else "unknown")
        rec.add(trace.KIND_DECISION, f"match item {number}", reason, outcome, detail)
    previous, missing = pick_previous(state.get("history", []), extraction.get("copyPrevious"), state["today"])
    if (extraction.get("copyPrevious") or {}).get("kind", "none") not in ("none", None):
        rec.add(trace.KIND_DECISION, "find earlier order",
                "The text referred to an earlier order, so the outlet's own recent orders were searched in code; the model does not pick it.",
                "found" if previous else "not found", {"orderRef": (previous or {}).get("orderRef", "")})
    needed_by, bad_date = resolve_needed_by(extraction.get("neededBy"), state["today"])
    if extraction.get("neededBy"):
        rec.add(trace.KIND_GUARDRAIL, "check delivery date",
                "A delivery date was named, so code checked it is a real date that is not in the past and is within 60 days.",
                "accepted" if needed_by else "asked")
    questions += [q for q in (missing, bad_date) if q]
    rec.add(trace.KIND_DECISION, "draft ready",
            "The result is only a draft that fills the form; the store manager still presses Submit, so cutoff and order rules are unchanged.",
            f"{len(lines)} line(s), {len(questions)} question(s)", {"lines": len(lines), "questions": len(questions)})
    return {"lines": lines, "questions": questions, "previous_order": previous, "needed_by": needed_by}


def _item_reason(line: dict | None, question: dict | None, family: str) -> tuple[str, str, dict]:
    if line:
        return (f"Matched the {family} family to {line['name']} and converted the quantity to {line['quantity']} {line['pack']}(s) in code.",
                "matched", {"productId": line["productId"], "packs": line["quantity"]})
    kind = (question or {}).get("kind", "")
    reasons = {
        "not_in_catalog": "That family is not in this outlet's brand catalog, so it became a question instead of guessing a product.",
        "choose_product": f"The {family} family has more than one size and the text did not name exactly one, so the manager is asked to choose.",
        "quantity": f"No usable whole quantity was written for the {family} family (or it said 'as usual', which past orders cannot answer), so the manager is asked.",
    }
    return reasons.get(kind, "It could not be resolved safely, so the manager is asked."), f"asked: {kind}", {"question": kind}


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
