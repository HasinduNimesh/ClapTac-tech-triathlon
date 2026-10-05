package suggest

import (
	"strings"
	"testing"
	"time"
)

func day(s string) time.Time { t, _ := time.Parse("2006-01-02", s); return t }

var milk = Product{ID: "FR-MILK-1L", Name: "Milk", UnitsPerPack: 12, AvgDailySalesEach: 36, Temperature: "chilled", Brand: "Fresh"}
var curd = Product{ID: "FR-CURD", Name: "Curd", UnitsPerPack: 8, AvgDailySalesEach: 16, Temperature: "chilled", Brand: "Fresh"}
var rice = Product{ID: "FR-RICE", Name: "Rice", UnitsPerPack: 1, AvgDailySalesEach: 4, Temperature: "ambient", Brand: "Fresh"}
var shirt = Product{ID: "ST-SHIRT", Name: "Shirt", UnitsPerPack: 10, AvgDailySalesEach: 20, Temperature: "ambient", Brand: "Style", Season: "avurudu"}

// closed makes the listed dates non-operating.
func closed(dates ...string) func(string) bool {
	return func(d string) bool {
		for _, c := range dates {
			if c == d {
				return false
			}
		}
		return true
	}
}

func lineFor(r Result, id string) *Line {
	for i := range r.Lines {
		if r.Lines[i].ProductID == id {
			return &r.Lines[i]
		}
	}
	return nil
}

func TestColdStartUsesTheStoresSalesRateAndOnlyTheChosenGoodsType(t *testing.T) {
	r := Suggest(Input{Date: day("2026-10-07"), Temperature: "chilled", Products: []Product{milk, curd, rice}})
	if r.Basis != "sales" || r.CoverDays != 1 || len(r.Lines) != 2 {
		t.Fatalf("%+v", r)
	}
	if l := lineFor(r, "FR-MILK-1L"); l == nil || l.Packs != 3 || l.Basis != "sales" {
		t.Fatalf("36 eaches a day in crates of 12 is 3 crates: %+v", l)
	}
	if l := lineFor(r, "FR-CURD"); l == nil || l.Packs != 2 {
		t.Fatalf("%+v", l)
	}
	if lineFor(r, "FR-RICE") != nil {
		t.Fatal("ambient rice must not be suggested on a chilled order")
	}
}

func TestACloseDayLengthensTheCoverAndTheOrder(t *testing.T) {
	// Friday delivery, depot closed Saturday, Sunday and Monday (a long weekend): four days to last.
	op := closed("2026-10-10", "2026-10-11", "2026-10-12")
	if got := CoverDays(day("2026-10-09"), op); got != 4 {
		t.Fatalf("cover = %d", got)
	}
	r := Suggest(Input{Date: day("2026-10-09"), Temperature: "chilled", Products: []Product{milk}, Operating: op})
	if l := lineFor(r, "FR-MILK-1L"); l == nil || l.Packs != 12 || !strings.Contains(l.Reason, "for 4 days") {
		t.Fatalf("3 crates a day for 4 days: %+v", l)
	}
	if len(r.Notes) == 0 || !strings.Contains(r.Notes[0], "4 days") {
		t.Fatalf("the manager is told why: %v", r.Notes)
	}
	if CoverDays(day("2026-10-09"), nil) != 1 {
		t.Fatal("no calendar means a single day")
	}
	never := func(string) bool { return false }
	if got := CoverDays(day("2026-10-09"), never); got != 7 {
		t.Fatalf("cover is capped at a week, got %d", got)
	}
}

func orders(packs ...int) []HistoricOrder {
	// Wednesdays 2026-09-09 ... one order per entry, going back a week each time from 2026-09-30.
	var out []HistoricOrder
	d := day("2026-09-30")
	for _, p := range packs {
		out = append(out, HistoricOrder{DeliveryDate: d.Format("2006-01-02"), Packs: map[string]int{"FR-MILK-1L": p}})
		d = d.AddDate(0, 0, -7)
	}
	return out
}

func TestHistoryBeatsSalesOnceThereAreEnoughOrders(t *testing.T) {
	r := Suggest(Input{Date: day("2026-10-07"), Temperature: "chilled", Products: []Product{milk}, History: orders(6, 6, 6, 6)})
	l := lineFor(r, "FR-MILK-1L")
	if r.Basis != "history" || l == nil || l.Packs != 6 || l.Basis != "history" {
		t.Fatalf("the store really orders 6 crates, not the 3 its sales suggest: %+v %+v", r, l)
	}
	few := Suggest(Input{Date: day("2026-10-07"), Temperature: "chilled", Products: []Product{milk}, History: orders(6, 6)})
	if !few.LimitedHistory || few.Basis != "sales" || lineFor(few, "FR-MILK-1L").Packs != 3 || len(few.Notes) == 0 {
		t.Fatalf("two orders are not enough: %+v", few)
	}
}

func TestAPreHolidayOrderDoesNotInflateOrdinaryDays(t *testing.T) {
	// One past order of 12 crates had to last 4 days (3 a day); the others were 3 crates for one day.
	op := closed("2026-09-26", "2026-09-27", "2026-09-28")
	h := []HistoricOrder{
		{DeliveryDate: "2026-09-25", Packs: map[string]int{"FR-MILK-1L": 12}},
		{DeliveryDate: "2026-09-30", Packs: map[string]int{"FR-MILK-1L": 3}},
		{DeliveryDate: "2026-10-02", Packs: map[string]int{"FR-MILK-1L": 3}},
	}
	r := Suggest(Input{Date: day("2026-10-06"), Temperature: "chilled", Products: []Product{milk}, History: h, Operating: op})
	if l := lineFor(r, "FR-MILK-1L"); l == nil || l.Packs != 3 {
		t.Fatalf("an ordinary day is still 3 crates: %+v", l)
	}
}

func TestSameWeekdayCountsDouble(t *testing.T) {
	// Target is a Wednesday. Wednesday orders of 8 outweigh Friday orders of 2.
	h := []HistoricOrder{
		{DeliveryDate: "2026-09-30", Packs: map[string]int{"FR-MILK-1L": 8}}, // Wed
		{DeliveryDate: "2026-09-23", Packs: map[string]int{"FR-MILK-1L": 8}}, // Wed
		{DeliveryDate: "2026-09-25", Packs: map[string]int{"FR-MILK-1L": 2}}, // Fri
		{DeliveryDate: "2026-09-18", Packs: map[string]int{"FR-MILK-1L": 2}}, // Fri
	}
	r := Suggest(Input{Date: day("2026-10-07"), Temperature: "chilled", Products: []Product{milk}, History: h})
	// Weights 2,2,1,1: (2*8+2*8+2+2)/6 = 6. Equal weights would give 5.
	if l := lineFor(r, "FR-MILK-1L"); l == nil || l.Packs != 6 {
		t.Fatalf("%+v", l)
	}
}

func TestAOneOffProductIsNotSuggestedAgain(t *testing.T) {
	h := orders(6, 6, 6, 6)
	h[0].Packs["FR-CURD"] = 5 // bought once in four orders
	r := Suggest(Input{Date: day("2026-10-07"), Temperature: "chilled", Products: []Product{milk, curd}, History: h})
	if lineFor(r, "FR-CURD") != nil {
		t.Fatalf("a product on a quarter of the orders is a one-off: %+v", r.Lines)
	}
}

func TestOldAndFutureOrdersAreIgnored(t *testing.T) {
	h := []HistoricOrder{
		{DeliveryDate: "2026-05-01", Packs: map[string]int{"FR-MILK-1L": 30}},
		{DeliveryDate: "2026-05-08", Packs: map[string]int{"FR-MILK-1L": 30}},
		{DeliveryDate: "2026-05-15", Packs: map[string]int{"FR-MILK-1L": 30}},
		{DeliveryDate: "2026-10-09", Packs: map[string]int{"FR-MILK-1L": 30}},
	}
	r := Suggest(Input{Date: day("2026-10-07"), Temperature: "chilled", Products: []Product{milk}, History: h})
	if r.Basis != "sales" || lineFor(r, "FR-MILK-1L").Packs != 3 {
		t.Fatalf("orders older than %d weeks or after the delivery date say nothing: %+v", HistoryWeeks, r)
	}
}

func TestSeasonalUpliftIsVisibleAndDoesNotStack(t *testing.T) {
	r := Suggest(Input{Date: day("2026-04-10"), Temperature: "ambient", Products: []Product{rice, shirt}})
	// rice 4/day -> 4 packs, +30% Avurudu food -> 5.2 -> 5. shirt 20/10 = 2 packs, +40% -> 2.8 -> 3.
	if l := lineFor(r, "FR-RICE"); l == nil || l.Packs != 5 || !strings.Contains(l.Reason, "+30%") || !strings.Contains(l.Reason, "New Year") || l.UpliftPercent != 30 || l.Season != "avurudu" || l.PerDay != 4 {
		t.Fatalf("%+v", l)
	}
	if l := lineFor(r, "ST-SHIRT"); l == nil || l.Packs != 3 || !strings.Contains(l.Reason, "+40%") {
		t.Fatalf("%+v", l)
	}
	plain := Suggest(Input{Date: day("2026-07-10"), Temperature: "ambient", Products: []Product{rice, shirt}})
	if lineFor(plain, "FR-RICE").Packs != 4 || strings.Contains(lineFor(plain, "FR-RICE").Reason, "%") {
		t.Fatalf("no season in July: %+v", plain.Lines)
	}
	dec := Suggest(Input{Date: day("2026-12-20"), Temperature: "ambient", Products: []Product{shirt}})
	if l := lineFor(dec, "ST-SHIRT"); l == nil || strings.Contains(l.Reason, "%") {
		t.Fatalf("a shirt tagged for Avurudu gets no December festive uplift: %+v", l)
	}
	festive := shirt
	festive.Season = "festive"
	if l := lineFor(Suggest(Input{Date: day("2026-12-20"), Temperature: "ambient", Products: []Product{festive}}), "ST-SHIRT"); l == nil || !strings.Contains(l.Reason, "+30%") {
		t.Fatalf("festive-tagged wear does: %+v", l)
	}
}

func TestSeasonWindowsWrapAndAreInclusive(t *testing.T) {
	s := season{Start: [2]int{12, 15}, End: [2]int{1, 5}}
	for d, want := range map[string]bool{"2026-12-14": false, "2026-12-15": true, "2027-01-05": true, "2027-01-06": false, "2026-07-01": false} {
		if s.covers(day(d)) != want {
			t.Errorf("%s covered = %v, want %v", d, !want, want)
		}
	}
}

func TestPacksAreAtLeastOneAndCapped(t *testing.T) {
	slow := Product{ID: "FR-SLOW", UnitsPerPack: 10, AvgDailySalesEach: 0.5, Temperature: "ambient", Brand: "Fresh"}
	huge := Product{ID: "FR-HUGE", UnitsPerPack: 1, AvgDailySalesEach: 100000, Temperature: "ambient", Brand: "Fresh"}
	none := Product{ID: "FR-NONE", UnitsPerPack: 1, AvgDailySalesEach: 0, Temperature: "ambient", Brand: "Fresh"}
	r := Suggest(Input{Date: day("2026-07-10"), Temperature: "ambient", Products: []Product{slow, huge, none}})
	if lineFor(r, "FR-SLOW").Packs != 1 || lineFor(r, "FR-HUGE").Packs != maxPacks || lineFor(r, "FR-NONE") != nil {
		t.Fatalf("%+v", r.Lines)
	}
	if empty := Suggest(Input{Date: day("2026-07-10"), Temperature: "ambient"}); empty.Basis != "none" || len(empty.Lines) != 0 {
		t.Fatalf("%+v", empty)
	}
}
