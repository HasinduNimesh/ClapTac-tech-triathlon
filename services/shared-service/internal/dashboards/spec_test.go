package dashboards

import (
	"reflect"
	"strings"
	"testing"
)

func TestNormalizeKeepsOrderAndDefaultsFilter(t *testing.T) {
	got, err := Normalize(Spec{Name: "  Receipts   and shortages ", Cards: []string{"deadlines", "short", "receipts"}})
	if err != nil {
		t.Fatal(err)
	}
	want := Spec{Name: "Receipts and shortages", Cards: []string{"deadlines", "short", "receipts"}, Filter: "all"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v want %+v", got, want)
	}
}

func TestNormalizeRejectsAnythingOutsideTheCardList(t *testing.T) {
	bad := []Spec{
		{Name: "", Cards: []string{"receipts"}},
		{Name: strings.Repeat("x", MaxName+1), Cards: []string{"receipts"}},
		{Name: "t"},
		{Name: "t", Cards: []string{"receipts", "receipts"}},
		{Name: "t", Cards: []string{"sql"}},
		{Name: "t", Cards: []string{"receipts"}, Filter: "frozen"},
		{Name: "t", Cards: []string{"deadlines", "receipts", "short", "ontime", "shortByWeek", "deferrals", "chilled", "arrivals", "receipts"}},
	}
	for i, spec := range bad {
		if _, err := Normalize(spec); err == nil {
			t.Fatalf("case %d should be rejected: %+v", i, spec)
		}
	}
}
