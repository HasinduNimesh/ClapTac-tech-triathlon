// Package dashboards validates saved dashboard specs. The card ids match the
// store-manager cards in apps/web/src/store-manager/dashboards.ts and the
// dashboard assistant (services/agent-assistants); this service is
// authoritative because a client can call it without the assistant.
package dashboards

import (
	"errors"
	"fmt"
	"strings"
)

const MaxName = 60

var (
	cardIDs = []string{"deadlines", "receipts", "short", "ontime", "shortByWeek", "deferrals", "chilled", "arrivals"}
	filters = map[string]bool{"all": true, "chilled": true, "ambient": true}
)

type Spec struct {
	Name   string   `json:"name"`
	Cards  []string `json:"cards"`
	Filter string   `json:"filter"`
}

// Normalize trims the name, defaults the filter and rejects anything outside
// the fixed card list. Card order is kept; duplicates are rejected.
func Normalize(in Spec) (Spec, error) {
	out := Spec{Name: strings.Join(strings.Fields(in.Name), " "), Filter: strings.TrimSpace(in.Filter)}
	if out.Name == "" || len([]rune(out.Name)) > MaxName {
		return Spec{}, fmt.Errorf("name must contain 1-%d characters", MaxName)
	}
	if out.Filter == "" {
		out.Filter = "all"
	}
	if !filters[out.Filter] {
		return Spec{}, errors.New("filter must be all, chilled or ambient")
	}
	if len(in.Cards) == 0 || len(in.Cards) > len(cardIDs) {
		return Spec{}, fmt.Errorf("a dashboard needs 1-%d cards", len(cardIDs))
	}
	seen := map[string]bool{}
	for _, c := range in.Cards {
		if !known(c) {
			return Spec{}, errors.New("unsupported card")
		}
		if seen[c] {
			return Spec{}, errors.New("each card can appear once")
		}
		seen[c] = true
		out.Cards = append(out.Cards, c)
	}
	return out, nil
}

func known(id string) bool {
	for _, c := range cardIDs {
		if c == id {
			return true
		}
	}
	return false
}
