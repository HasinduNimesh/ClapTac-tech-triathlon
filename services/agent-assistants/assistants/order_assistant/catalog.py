import json
import re
from dataclasses import asdict, dataclass
from functools import lru_cache
from pathlib import Path

CATALOG_PATH = Path(__file__).with_name("catalog.json")


@dataclass(frozen=True)
class Product:
    id: str
    family: str
    name: str
    size: str
    pack: str
    unitsPerPack: int
    packWeightKg: float
    packVolumeM3: float
    temperature: str
    aliases: tuple[str, ...]

    def public(self) -> dict:
        out = asdict(self)
        out.pop("aliases")
        return out


@dataclass(frozen=True)
class Catalog:
    version: str
    brands: dict[str, tuple[Product, ...]]

    def products(self, brand: str) -> tuple[Product, ...]:
        return self.brands.get(brand, ())

    def families(self, brand: str) -> dict[str, list[Product]]:
        out: dict[str, list[Product]] = {}
        for p in self.products(brand):
            out.setdefault(p.family, []).append(p)
        return out


@lru_cache(maxsize=1)
def load_catalog(path: str = str(CATALOG_PATH)) -> Catalog:
    raw = json.loads(Path(path).read_text(encoding="utf-8"))
    brands = {
        brand: tuple(Product(**{**item, "aliases": tuple(item.get("aliases", []))}) for item in items)
        for brand, items in raw["brands"].items()
    }
    return Catalog(version=raw["version"], brands=brands)


_UNIT_WORDS = [
    (r"millilitres?|milliliters?|mls?", "ml"),
    (r"litres?|liters?|ltrs?|l", "l"),
    (r"kilograms?|kilos?|kgs?", "kg"),
    (r"grams?|grammes?|g", "g"),
]


def normalize_size(value: str | None) -> str:
    """'1 L', '1l', '1 litre' -> '1l'; '500ML' -> '500ml'. Empty when no amount+unit."""
    if not value:
        return ""
    text = value.strip().lower().replace(" ", "")
    match = re.fullmatch(r"(\d+(?:\.\d+)?)([a-z]+)", text)
    if not match:
        return ""
    amount, unit = match.groups()
    for pattern, canonical in _UNIT_WORDS:
        if re.fullmatch(pattern, unit):
            amount = amount.rstrip("0").rstrip(".") if "." in amount else amount
            return amount + canonical
    return ""
