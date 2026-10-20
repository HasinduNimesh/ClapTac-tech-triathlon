package service

import (
	"testing"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/loading-service/internal/domain"
)

func TestOpenShortfallsIgnoreDecidedLines(t *testing.T) {
	loads := []domain.OrderLoad{
		{ID: "l1", OrderID: "o1", Status: domain.LoadShortfall},
		{ID: "l2", OrderID: "o2", Status: domain.LoadShortfall},
		{ID: "l3", OrderID: "o3", Status: domain.LoadShortfall},
		{ID: "l4", OrderID: "o4", Status: domain.LoadLoaded},
	}
	issues := map[string][]domain.Issue{
		"l1": {{ID: "i1"}}, // reported, not decided yet
		"l2": {{ID: "i2", Decision: domain.DecisionPartialLoad}},
		"l3": {{ID: "i3", Decision: domain.DecisionHold}},
	}
	if got, _ := countOpenShortfalls(loads, issues, map[string]bool{}); got != 2 {
		t.Fatalf("an undecided line and a held line are open, a partial load is not: got %d open", got)
	}
	issues["l1"][0].Decision = domain.DecisionPartialLoad
	issues["l3"][0].Decision = domain.DecisionPartialLoad
	if got, _ := countOpenShortfalls(loads, issues, map[string]bool{}); got != 0 {
		t.Fatalf("every shortfall is decided, got %d open", got)
	}
}

func TestMovedLineStaysOpenUntilItLeavesThePlan(t *testing.T) {
	loads := []domain.OrderLoad{{ID: "l1", OrderID: "o1", Status: domain.LoadShortfall}}
	issues := map[string][]domain.Issue{"l1": {{ID: "i1", Decision: domain.DecisionMoveToNextRun}}}
	if got, _ := countOpenShortfalls(loads, issues, map[string]bool{"o1": true}); got != 1 {
		t.Fatalf("still on the confirmed plan: want 1 open, got %d", got)
	}
	if got, _ := countOpenShortfalls(loads, issues, map[string]bool{}); got != 0 {
		t.Fatalf("off the plan: want 0 open, got %d", got)
	}
}

func TestOnlyAChilledLineMakesAShortfallCritical(t *testing.T) {
	loads := []domain.OrderLoad{
		{ID: "l1", OrderID: "o1", Status: domain.LoadShortfall, TemperatureRequirement: "ambient"},
		{ID: "l2", OrderID: "o2", Status: domain.LoadShortfall, TemperatureRequirement: "chilled"},
	}
	issues := map[string][]domain.Issue{"l1": {{ID: "i1"}}, "l2": {{ID: "i2", Decision: domain.DecisionPartialLoad}}}
	if n, chilledOpen := countOpenShortfalls(loads, issues, nil); n != 1 || chilledOpen {
		t.Fatalf("only the ambient line is open: got %d open, chilled=%v", n, chilledOpen)
	}
	issues["l2"][0].Decision = ""
	if n, chilledOpen := countOpenShortfalls(loads, issues, nil); n != 2 || !chilledOpen {
		t.Fatalf("the chilled line is open too: got %d open, chilled=%v", n, chilledOpen)
	}
}
