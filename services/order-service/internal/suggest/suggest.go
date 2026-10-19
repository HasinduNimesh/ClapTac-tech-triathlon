// Package suggest proposes the item lines for a store's next order from what the store has ordered before, how
// long the delivery has to last (long weekends and holidays from the operating calendar) and a few well-known
// seasons. It is deterministic and explains every line, so a manager can see why a quantity was chosen and change
// it. It never places an order.
package suggest

import (
	"fmt"
	"math"
	"sort"
	"time"
)

const (
	// HistoryWeeks is how far back past orders are read.
	HistoryWeeks = 8
	// MinHistoryOrders is how many past orders with item lines are needed before history is trusted over the
	// store's sales rate.
	MinHistoryOrders = 3
	maxCoverDays     = 7
	maxPacks         = 999
)

// Product is one product the outlet lists, with its sales rate (in single items, "eaches", per day).
type Product struct {
	ID                string
	Name              string
	UnitsPerPack      int
	AvgDailySalesEach float64
	Temperature       string
	Season            string // all_year, avurudu, school_term, festive, monsoon, or empty
	Brand             string
}

// HistoricOrder is one past order of this outlet and goods type, with its item lines.
type HistoricOrder struct {
	DeliveryDate string // YYYY-MM-DD
	Packs        map[string]int
}

type Input struct {
	Date        time.Time // delivery date, in Colombo
	Temperature string
	Products    []Product
	History     []HistoricOrder
	// Operating says whether deliveries run on a date (YYYY-MM-DD). Nil means every day.
	Operating func(date string) bool
}

// Line is one suggested product. Reason is a ready English sentence; the structured fields let a screen say the
// same thing in the user's own language.
type Line struct {
	ProductID     string  `json:"productId"`
	Packs         int     `json:"packs"`
	Basis         string  `json:"basis"`         // history or sales
	PerDay        float64 `json:"perDay"`        // packs per day the quantity is built from
	UpliftPercent int     `json:"upliftPercent"` // 0 when no season applies
	Season        string  `json:"season,omitempty"`
	Reason        string  `json:"reason"`
}

type Result struct {
	Lines     []Line `json:"lines"`
	CoverDays int    `json:"coverDays"`
	Basis     string `json:"basis"` // history, sales or none
	// LimitedHistory is true when earlier orders exist but too few to rely on, so sales were used.
	LimitedHistory bool     `json:"limitedHistory"`
	Notes          []string `json:"notes"`
}

// CoverDays is how many days a delivery on date has to last: the day itself plus every following day the depot does
// not deliver, so an order before a long weekend is larger.
func CoverDays(date time.Time, operating func(string) bool) int {
	if operating == nil {
		return 1
	}
	days := 1
	for d := date.AddDate(0, 0, 1); days < maxCoverDays && !operating(d.Format("2006-01-02")); d = d.AddDate(0, 0, 1) {
		days++
	}
	return days
}

type season struct {
	Key        string
	Name       string
	Reason     string
	Start, End [2]int // month, day (inclusive); End before Start wraps the year
	Uplift     float64
	Brand      string // limit to a brand, empty for all
	Tag        string // limit to products with this season tag, empty for all
}

// Seasons are fixed-date festivals. They are deliberately few and visible: each applied uplift is shown to the
// manager with its reason. Moveable holidays (Poya, Deepavali, Vesak) are covered by the operating calendar
// instead, because a closure lengthens the cover days.
var seasons = []season{
	{Key: "avurudu", Name: "avurudu-food", Reason: "Sinhala and Tamil New Year week: food sells faster", Start: [2]int{4, 1}, End: [2]int{4, 13}, Uplift: 1.3, Brand: "Fresh"},
	{Key: "avurudu", Name: "avurudu-style", Reason: "Sinhala and Tamil New Year: clothing is bought for the festival", Start: [2]int{3, 25}, End: [2]int{4, 12}, Uplift: 1.4, Tag: "avurudu"},
	{Key: "christmas", Name: "christmas-food", Reason: "Christmas week: food sells faster", Start: [2]int{12, 15}, End: [2]int{12, 24}, Uplift: 1.2, Brand: "Fresh"},
	{Key: "christmas", Name: "festive-style", Reason: "Festive season: gifts and festive wear", Start: [2]int{12, 1}, End: [2]int{12, 24}, Uplift: 1.3, Tag: "festive"},
}

func (s season) covers(date time.Time) bool {
	m, d := int(date.Month()), date.Day()
	after := func(a [2]int) bool { return m > a[0] || (m == a[0] && d >= a[1]) }
	before := func(a [2]int) bool { return m < a[0] || (m == a[0] && d <= a[1]) }
	if s.Start[0] < s.End[0] || (s.Start[0] == s.End[0] && s.Start[1] <= s.End[1]) {
		return after(s.Start) && before(s.End)
	}
	return after(s.Start) || before(s.End)
}

// uplift returns the multiplier for one product on the date and the reason, 1 when none applies. The largest
// applicable uplift wins; they do not stack.
func uplift(p Product, date time.Time) (float64, string, string) {
	best, why, key := 1.0, "", ""
	for _, s := range seasons {
		if !s.covers(date) || (s.Brand != "" && s.Brand != p.Brand) || (s.Tag != "" && s.Tag != p.Season) {
			continue
		}
		if s.Uplift > best {
			best, why, key = s.Uplift, s.Reason, s.Key
		}
	}
	return best, why, key
}

func roundHalfUp(v float64) int { return int(math.Floor(v + 0.5)) }

func Suggest(in Input) Result {
	cover := CoverDays(in.Date, in.Operating)
	out := Result{Lines: []Line{}, CoverDays: cover, Basis: "none", Notes: []string{}}
	if cover > 1 {
		out.Notes = append(out.Notes, fmt.Sprintf("This delivery has to last %d days because the depot does not deliver on the days after it.", cover))
	}
	products := make([]Product, 0, len(in.Products))
	for _, p := range in.Products {
		if p.Temperature == in.Temperature && p.UnitsPerPack > 0 {
			products = append(products, p)
		}
	}
	sort.Slice(products, func(i, j int) bool { return products[i].ID < products[j].ID })

	usable := recentOrders(in)
	rates := map[string]float64{} // packs per day
	basis := "sales"
	if len(usable) >= MinHistoryOrders {
		basis = "history"
		rates = historyRates(in, usable)
	} else {
		for _, p := range products {
			rates[p.ID] = p.AvgDailySalesEach / float64(p.UnitsPerPack)
		}
		if len(in.History) > 0 {
			out.LimitedHistory = true
			out.Notes = append(out.Notes, "There are not enough earlier orders with items yet, so quantities come from the store's usual daily sales.")
		}
	}
	for _, p := range products {
		rate, ok := rates[p.ID]
		if !ok || rate <= 0 {
			continue
		}
		factor, why, seasonKey := uplift(p, in.Date)
		packs := roundHalfUp(rate * float64(cover) * factor)
		if packs < 1 {
			packs = 1
		}
		if packs > maxPacks {
			packs = maxPacks
		}
		reason := fmt.Sprintf("About %s per day from your last orders", trim(rate))
		if basis == "sales" {
			reason = fmt.Sprintf("About %s per day from the store's usual sales", trim(rate))
		}
		if cover > 1 {
			reason += fmt.Sprintf(", for %d days", cover)
		}
		if factor > 1 {
			reason += fmt.Sprintf(", +%d%%: %s", int(math.Round((factor-1)*100)), why)
		}
		percent := 0
		if factor > 1 {
			percent = int(math.Round((factor - 1) * 100))
		}
		out.Lines = append(out.Lines, Line{ProductID: p.ID, Packs: packs, Basis: basis, PerDay: math.Round(rate*10) / 10, UpliftPercent: percent, Season: seasonKey, Reason: reason})
	}
	if len(out.Lines) > 0 {
		out.Basis = basis
	}
	return out
}

func trim(v float64) string {
	if v >= 10 {
		return fmt.Sprintf("%.0f", v)
	}
	return fmt.Sprintf("%.1f", v)
}

func recentOrders(in Input) []HistoricOrder {
	cutoff := in.Date.AddDate(0, 0, -7*HistoryWeeks).Format("2006-01-02")
	before := in.Date.Format("2006-01-02")
	var out []HistoricOrder
	for _, h := range in.History {
		if len(h.Packs) > 0 && h.DeliveryDate >= cutoff && h.DeliveryDate < before {
			out = append(out, h)
		}
	}
	return out
}

// historyRates turns past orders into packs per day for each product. An order is divided by the days it had to
// last, so a big pre-holiday order does not inflate an ordinary day. Orders on the same weekday as the target count
// double, because stores tend to order the same on the same day. A product must be on at least half of the weighted
// orders, so a one-off purchase is not suggested again.
func historyRates(in Input, orders []HistoricOrder) map[string]float64 {
	var totalWeight float64
	weights := make([]float64, len(orders))
	for i, o := range orders {
		weights[i] = 1
		if d, err := time.Parse("2006-01-02", o.DeliveryDate); err == nil && d.Weekday() == in.Date.Weekday() {
			weights[i] = 2
		}
		totalWeight += weights[i]
	}
	present := map[string]float64{}
	sum := map[string]float64{}
	for i, o := range orders {
		d, _ := time.Parse("2006-01-02", o.DeliveryDate)
		days := float64(CoverDays(d, in.Operating))
		for id, packs := range o.Packs {
			present[id] += weights[i]
			sum[id] += weights[i] * float64(packs) / days
		}
	}
	out := map[string]float64{}
	for id, w := range present {
		if w*2 >= totalWeight {
			out[id] = sum[id] / w
		}
	}
	return out
}
