"""The fixed dashboard vocabulary (contract shared with shared-service and the web app).

A dashboard is a name, an ordered list of card ids and a goods filter. The
card ids are the store-manager cards in apps/web/src/store-manager/dashboards.ts
(Figma "Create new dashboard"). The web app computes every card from the
person's own order tracking data, so a saved dashboard keeps working when the
assistant is off, and a spec can never point at another store's data: it has
no outlet or user fields at all.
"""

CARDS = {
    "deadlines": "Report-by deadlines: deliveries still to confirm, with the time shortages must be reported by",
    "receipts": "Receipts to confirm: how many delivered orders still need a receipt",
    "short": "Short this week: items that arrived short or damaged in the last 7 days",
    "ontime": "On-time arrivals: share of deliveries inside the receiving window",
    "shortByWeek": "Items short by week: bar chart of shortages per week",
    "deferrals": "Deferrals by reason: orders moved to a later run, grouped by reason",
    "chilled": "Chilled deliveries: chilled orders and their status",
    "arrivals": "Arrivals vs my window: planned arrival times against the receiving window",
}
FILTERS = ("all", "chilled", "ambient")
MAX_NAME = 60


def _name(value, fallback: str) -> str:
    text = " ".join(str(value or "").split())[:MAX_NAME]
    return text or fallback


def clean_spec(raw: dict | None, fallback_name: str = "New dashboard") -> tuple[dict, list[str]]:
    raw = raw if isinstance(raw, dict) else {}
    cards, rejected = [], []
    items = raw.get("cards") if isinstance(raw.get("cards"), list) else []
    for item in items:
        if item not in CARDS:
            rejected.append(f"unsupported card {str(item)[:30]!r}")
        elif item not in cards:
            cards.append(item)
    goods = raw.get("filter")
    return {
        "name": _name(raw.get("name"), fallback_name),
        "cards": cards,
        "filter": goods if goods in FILTERS else "all",
    }, rejected
