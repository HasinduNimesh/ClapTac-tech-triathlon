package sequence

import (
	"testing"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/loading-service/internal/domain"
)

func TestReverseSuggestedKeepsDeliverySequence(t *testing.T) {
	stops := []domain.Stop{
		{OrderID: "OUT001", StopSequence: 1},
		{OrderID: "OUT002", StopSequence: 2},
		{OrderID: "OUT003", StopSequence: 3},
	}
	got := ReverseLastOut{}.Suggest(stops)
	if len(got) != 3 {
		t.Fatalf("len %d", len(got))
	}
	if got[0].StopSequence != 1 || got[1].StopSequence != 2 || got[2].StopSequence != 3 {
		t.Fatalf("delivery sequence changed %#v", got)
	}
	if got[0].SuggestedLoadSequence != 3 || got[1].SuggestedLoadSequence != 2 || got[2].SuggestedLoadSequence != 1 {
		t.Fatalf("suggested %#v", got)
	}
	if stops[0].StopSequence != 1 || stops[2].StopSequence != 3 {
		t.Fatal("input mutated")
	}
}
