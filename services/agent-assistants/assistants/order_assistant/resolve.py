"""Deterministic part of the order assistant.

The language model only says *what the person wrote* (family, size words,
quantity, unit). Everything that touches real products, quantities and dates
happens here, so nothing can be invented: unknown products, unclear sizes and
missing quantities become questions for the store manager.
"""

import math
import re
from datetime import date, datetime, timedelta, timezone

from .catalog import Product, normalize_size

COLOMBO = timezone(timedelta(hours=5, minutes=30))
WEEKDAYS = ["monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday"]
MAX_PACKS_PER_LINE = 999


def _line(product: Product, packs: int, source: str) -> dict:
    return {
        "productId": product.id,
        "name": product.name,
        "pack": product.pack,
        "unitsPerPack": product.unitsPerPack,
        "quantity": packs,
        "weightKg": round(packs * product.packWeightKg, 2),
        "volumeM3": round(packs * product.packVolumeM3, 3),
        "temperature": product.temperature,
        "sourceText": source,
    }


def _question(kind: str, source: str, text: str, options: list[Product], suggested: int | None = None) -> dict:
    return {
        "kind": kind,
        "sourceText": source,
        "question": text,
        "options": [p.public() for p in options],
        "suggestedQuantity": suggested,
    }


def _packs(product: Product, quantity, unit: str) -> tuple[int | None, int | None]:
    """Returns (exact packs, suggested packs). Exact is None when the manager must confirm."""
    if not isinstance(quantity, (int, float)) or isinstance(quantity, bool) or quantity <= 0:
        return None, None
    if unit == "item" and product.unitsPerPack > 1:
        packs = quantity / product.unitsPerPack
    else:
        packs = quantity
    if packs > MAX_PACKS_PER_LINE:
        return None, None
    if float(packs).is_integer():
        return int(packs), int(packs)
    return None, max(1, math.ceil(packs))


def resolve_item(item: dict, families: dict[str, list[Product]]) -> tuple[dict | None, dict | None]:
    source = str(item.get("text") or "").strip()[:200]
    family = str(item.get("family") or "unknown")
    candidates = families.get(family, [])
    if not candidates:
        return None, _question("not_in_catalog", source, f'"{source}" is not in your product list. Add it by hand if needed.', [])

    wanted_size = normalize_size(item.get("size"))
    if len(candidates) > 1:
        sized = [p for p in candidates if wanted_size and normalize_size(p.size) == wanted_size]
        if len(sized) == 1:
            candidates = sized
        else:
            names = " or ".join(p.name for p in candidates)
            return None, _question("choose_product", source, f"Did you mean {names}?", candidates,
                                   _suggested_any(item, candidates))
    product = candidates[0]

    if item.get("asUsual") or item.get("quantity") in (None, 0):
        return None, _question("quantity", source,
                               f"How many {product.pack}s of {product.name}? Past orders do not record item lines yet.",
                               [product])
    exact, suggested = _packs(product, item.get("quantity"), str(item.get("quantityUnit") or "pack"))
    if exact is None:
        return None, _question("quantity", source,
                               f"{product.name} comes in {product.pack}s of {product.unitsPerPack}. How many {product.pack}s?",
                               [product], suggested)
    return _line(product, exact, source), None


def _suggested_any(item: dict, candidates: list[Product]) -> int | None:
    if str(item.get("quantityUnit") or "pack") != "pack":
        return None
    exact, _ = _packs(candidates[0], item.get("quantity"), "pack")
    return exact


def group_drafts(lines: list[dict]) -> list[dict]:
    """Ambient and chilled goods need separate orders (separate vehicles)."""
    drafts = []
    for temperature in ("ambient", "chilled"):
        group = [line for line in lines if line["temperature"] == temperature]
        if not group:
            continue
        drafts.append({
            "temperatureRequirement": temperature,
            "lines": group,
            "totals": {
                "orderUnits": sum(line["quantity"] for line in group),
                "orderWeightKg": round(sum(line["weightKg"] for line in group), 2),
                "orderVolumeM3": round(sum(line["volumeM3"] for line in group), 3),
            },
        })
    return drafts


def _created_local(order: dict) -> date | None:
    raw = str(order.get("createdAt") or "")
    raw = re.sub(r"(\.\d{6})\d+", r"\1", raw).replace("Z", "+00:00")
    try:
        parsed = datetime.fromisoformat(raw)
    except ValueError:
        return None
    if parsed.tzinfo is None:
        parsed = parsed.replace(tzinfo=timezone.utc)
    return parsed.astimezone(COLOMBO).date()


def pick_previous(orders: list[dict], copy: dict | None, today: date) -> tuple[dict | None, dict | None]:
    """Finds the order the person means by 'same as last Tuesday' / 'last week' / 'last time'."""
    kind = (copy or {}).get("kind") or "none"
    if kind == "none":
        return None, None
    # "Last time" includes an order placed earlier today; weekday and week references mean earlier days.
    dated = [(d, o) for o in orders if (d := _created_local(o)) is not None and d <= today]
    dated.sort(key=lambda pair: (pair[0], str(pair[1].get("createdAt", "")), str(pair[1].get("orderRef", ""))), reverse=True)
    earlier = [(d, o) for d, o in dated if d < today]
    match = None
    if kind == "last_order":
        match = dated[0] if dated else None
    elif kind == "weekday":
        weekday = str((copy or {}).get("weekday") or "").lower()
        if weekday in WEEKDAYS:
            index = WEEKDAYS.index(weekday)
            match = next(((d, o) for d, o in earlier if d.weekday() == index and (today - d).days <= 14), None)
    elif kind == "last_week":
        window = [(d, o) for d, o in earlier if 7 <= (today - d).days <= 13]
        match = next(((d, o) for d, o in window if d.weekday() == today.weekday()), window[0] if window else None)
    if match is None:
        return None, _question("previous_not_found", "", "I could not find that earlier order. Enter the items instead.", [])
    placed, order = match
    return {
        "orderRef": order.get("orderRef", ""),
        "placedOn": placed.isoformat(),
        "requestedDeliveryDate": order.get("requestedDeliveryDate", ""),
        "temperatureRequirement": order.get("temperatureRequirement", ""),
        "orderUnits": order.get("orderUnits", 0),
        "orderWeightKg": order.get("orderWeightKg", 0),
        "orderVolumeM3": order.get("orderVolumeM3", 0),
    }, None


def resolve_needed_by(value, today: date) -> tuple[str | None, dict | None]:
    if not value:
        return None, None
    try:
        needed = date.fromisoformat(str(value))
    except ValueError:
        return None, _question("date", "", "Which delivery date do you need? Choose it on the form.", [])
    if needed < today or (needed - today).days > 60:
        return None, _question("date", "", "That delivery date is not available. Choose it on the form.", [])
    return needed.isoformat(), None
