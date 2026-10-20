import asyncio
import unittest
from datetime import date

from assistants.order_assistant import graph as order_graph
from assistants.order_assistant.catalog import load_catalog, normalize_size

from .fakes import FakeBusiness, FakeLLM, PROFILES, extraction, item

TODAY = date(2026, 10, 3)  # Saturday


def run(args, orders=None, message="order text", products=None):
    llm = FakeLLM(args)
    business = FakeBusiness(PROFILES, orders)
    business.product_rows = products
    state = asyncio.run(order_graph.build_graph().ainvoke(
        {"message": message, "today": TODAY, "token": "store", "outlet_id": "OUT034"},
        config={"configurable": {"business": business, "llm": llm}},
    ))
    return order_graph.response(state), llm, business


class OrderAssistantTest(unittest.TestCase):
    def test_rice_oil_and_yoghurt_as_usual(self):
        body, llm, _ = run(extraction([
            item("rice 10 bags", "rice", 10),
            item("oil 24 bottles", "oil", 24, unit="item"),
            item("yoghurt as usual", "yoghurt", as_usual=True),
        ]), message="rice 10 bags, oil 24 bottles, and yoghurt as usual")
        lines = {line["productId"]: line["quantity"] for line in body["lines"]}
        self.assertEqual(lines, {"FR-RICE-5KG": 10, "FR-OIL-1L": 2})
        self.assertEqual([q["kind"] for q in body["questions"]], ["quantity"])
        self.assertEqual(body["questions"][0]["options"][0]["id"], "FR-YOGHURT-80G")
        self.assertEqual(len(body["drafts"]), 1)
        self.assertEqual(body["drafts"][0]["totals"], {"orderUnits": 12, "orderWeightKg": 74.2, "orderVolumeM3": 0.122})
        self.assertFalse(body["submitted"])
        # The model only ever sees the families of this outlet's brand.
        families = llm.calls[0]["function"]["parameters"]["properties"]["items"]["items"]["properties"]["family"]["enum"]
        self.assertIn("yoghurt", families)
        self.assertNotIn("laptops", families)

    def test_big_milk_asks_which_size(self):
        body, _, _ = run(extraction([item("2 crates of milk, the big ones", "milk", 2)]))
        self.assertEqual(body["lines"], [])
        question = body["questions"][0]
        self.assertEqual(question["kind"], "choose_product")
        self.assertEqual({o["id"] for o in question["options"]}, {"FR-MILK-1L", "FR-MILK-500ML"})
        self.assertEqual(question["suggestedQuantity"], 2)
        self.assertIn("Fresh milk 1 L or Fresh milk 500 ml", question["question"])

    def test_explicit_size_picks_one_product(self):
        body, _, _ = run(extraction([item("2 crates milk 500ml", "milk", 2, size="500ML")]))
        self.assertEqual(body["lines"][0]["productId"], "FR-MILK-500ML")
        self.assertEqual(body["drafts"][0]["temperatureRequirement"], "chilled")

    def test_same_as_last_tuesday_copies_that_order(self):
        orders = [
            {"orderRef": "FR-1", "outletId": "OUT034", "createdAt": "2026-09-29T03:10:00.123456789Z", "orderUnits": 14, "orderWeightKg": 80.5, "orderVolumeM3": 0.4, "temperatureRequirement": "ambient", "requestedDeliveryDate": "2026-09-30"},
            {"orderRef": "FR-2", "outletId": "OUT034", "createdAt": "2026-10-01T03:10:00Z", "orderUnits": 5, "orderWeightKg": 20, "orderVolumeM3": 0.1, "temperatureRequirement": "chilled", "requestedDeliveryDate": "2026-10-02"},
            {"orderRef": "OTHER", "outletId": "OUT999", "createdAt": "2026-09-29T05:00:00Z", "orderUnits": 99, "orderWeightKg": 1, "orderVolumeM3": 1, "temperatureRequirement": "ambient", "requestedDeliveryDate": "2026-09-30"},
        ]
        body, _, _ = run(extraction(kind="weekday", weekday="tuesday"), orders)
        self.assertEqual(body["previousOrder"]["orderRef"], "FR-1")
        self.assertEqual(body["previousOrder"]["orderUnits"], 14)
        self.assertEqual(body["questions"], [])

    def test_same_as_last_time_includes_an_order_from_earlier_today(self):
        orders = [
            {"orderRef": "FR-OLD", "outletId": "OUT034", "createdAt": "2026-09-30T03:00:00Z", "orderUnits": 3},
            {"orderRef": "FR-TODAY", "outletId": "OUT034", "createdAt": "2026-10-03T02:00:00Z", "orderUnits": 14},
        ]
        body, _, _ = run(extraction(kind="last_order"), orders)
        self.assertEqual(body["previousOrder"]["orderRef"], "FR-TODAY")
        body, _, _ = run(extraction(kind="weekday", weekday="saturday"), orders)
        self.assertEqual(body["questions"][0]["kind"], "previous_not_found")

    def test_missing_previous_order_is_a_question_not_a_guess(self):
        body, _, _ = run(extraction(kind="last_week"), [])
        self.assertIsNone(body["previousOrder"])
        self.assertEqual(body["questions"][0]["kind"], "previous_not_found")

    def test_unknown_or_invented_products_are_not_lines(self):
        body, _, _ = run(extraction([item("caviar", "caviar", 3), item("something", "unknown", 1)]))
        self.assertEqual(body["lines"], [])
        self.assertEqual([q["kind"] for q in body["questions"]], ["not_in_catalog", "not_in_catalog"])

    def test_bottles_that_do_not_fill_a_case_ask_with_a_suggestion(self):
        body, _, _ = run(extraction([item("oil 30 bottles", "oil", 30, unit="item")]))
        self.assertEqual(body["questions"][0]["kind"], "quantity")
        self.assertEqual(body["questions"][0]["suggestedQuantity"], 3)

    def test_ambient_and_chilled_become_separate_drafts(self):
        body, _, _ = run(extraction([item("rice", "rice", 2), item("yoghurt", "yoghurt", 1)]))
        self.assertEqual([d["temperatureRequirement"] for d in body["drafts"]], ["ambient", "chilled"])

    def test_needed_by_must_be_a_future_date(self):
        body, _, _ = run(extraction(needed_by="2026-10-06"))
        self.assertEqual(body["neededBy"], "2026-10-06")
        body, _, _ = run(extraction(needed_by="2026-09-01"))
        self.assertIsNone(body["neededBy"])
        self.assertEqual(body["questions"][0]["kind"], "date")

    def test_malformed_model_output_is_ignored(self):
        body, _, _ = run({"items": "not a list", "copyPrevious": None})
        self.assertEqual(body["lines"], [])
        self.assertIsNone(body["previousOrder"])

    def test_catalog_is_consistent(self):
        catalog = load_catalog()
        ids = [p.id for products in catalog.brands.values() for p in products]
        self.assertEqual(len(ids), len(set(ids)))
        for products in catalog.brands.values():
            for p in products:
                self.assertIn(p.temperature, ("ambient", "chilled"))
                self.assertGreater(p.unitsPerPack, 0)
                self.assertGreater(p.packWeightKg, 0)

    def test_normalize_size(self):
        self.assertEqual(normalize_size("1 L"), "1l")
        self.assertEqual(normalize_size("1 litre"), "1l")
        self.assertEqual(normalize_size("500ML"), "500ml")
        self.assertEqual(normalize_size("big"), "")


if __name__ == "__main__":
    unittest.main()


def live(id, family, name, size="", pack="bale", units=10, kg=10.0, m3=0.016, temp="ambient"):
    return {"id": id, "brand": "Fresh", "family": family, "name": name, "size": size, "pack": pack,
            "unitsPerPack": units, "packWeightKg": kg, "packVolumeM3": m3, "temperature": temp}


LIVE = [
    live("P-BASMATI", "basmati", "Basmati rice 1 kg", "1 kg"),
    live("P-SAMBA-5", "rice", "Samba rice 5 kg", "5 kg", pack="bag", units=1, kg=5.1),
    live("P-MARIE", "biscuits", "Marie biscuits 80 g", "80 g", pack="case", units=24, kg=2.1),
    live("P-CHOC", "biscuits", "Chocolate biscuits 100 g", "100 g", pack="case", units=24, kg=2.8),
]


class LiveCatalogTest(unittest.TestCase):
    """The assistant works from the product list in master data, not from a copy baked into the service."""

    def test_basmati_is_not_turned_into_samba(self):
        body, llm, _ = run(extraction([item("basmati rice 2 bales", "basmati", 2)]), products=LIVE)
        self.assertEqual([(l["productId"], l["quantity"]) for l in body["lines"]], [("P-BASMATI", 2)])
        families = llm.calls[0]["function"]["parameters"]["properties"]["items"]["items"]["properties"]["family"]["enum"]
        self.assertIn("basmati", families)
        self.assertIn("biscuits", families)

    def test_a_product_in_master_data_but_not_in_the_old_copy_can_be_ordered(self):
        body, _, _ = run(extraction([item("marie biscuits 80 g 3 cases", "biscuits", 3, size="80 g")]), products=LIVE)
        self.assertEqual([(l["productId"], l["quantity"]) for l in body["lines"]], [("P-MARIE", 3)])

    def test_two_products_in_one_family_ask_which_one(self):
        body, _, _ = run(extraction([item("biscuits 2 cases", "biscuits", 2)]), products=LIVE)
        self.assertEqual(body["lines"], [])
        self.assertEqual(body["questions"][0]["kind"], "choose_product")
        self.assertEqual({o["id"] for o in body["questions"][0]["options"]}, {"P-MARIE", "P-CHOC"})

    def test_a_product_taken_out_of_the_range_is_unknown(self):
        body, _, _ = run(extraction([item("basmati rice 2 bales", "basmati", 2)]), products=[p for p in LIVE if p["id"] != "P-BASMATI"])
        self.assertEqual(body["lines"], [])
        self.assertEqual(body["questions"][0]["kind"], "not_in_catalog")

    def test_incomplete_rows_are_skipped_not_guessed(self):
        broken = [{"id": "X", "family": "rice"}, {**LIVE[0], "unitsPerPack": 0}, {**LIVE[0], "temperature": "frozen"}, LIVE[1]]
        body, _, _ = run(extraction([item("samba rice 1 bag", "rice", 1)]), products=broken)
        self.assertEqual([l["productId"] for l in body["lines"]], ["P-SAMBA-5"])
        self.assertEqual([p["id"] for p in body["products"]], ["P-SAMBA-5"])
