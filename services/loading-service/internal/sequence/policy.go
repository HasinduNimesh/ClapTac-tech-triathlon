package sequence

import (
	"sort"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/loading-service/internal/domain"
)

type Policy interface {
	Suggest(stops []domain.Stop) []domain.LoadInstruction
}

// ReverseLastOut is a team policy: last delivery is loaded first.
// Guidance only — not a hard logistics constraint.
type ReverseLastOut struct{}

func (ReverseLastOut) Suggest(stops []domain.Stop) []domain.LoadInstruction {
	cp := append([]domain.Stop{}, stops...)
	sort.SliceStable(cp, func(i, j int) bool { return cp[i].StopSequence < cp[j].StopSequence })
	out := make([]domain.LoadInstruction, len(cp))
	n := len(cp)
	for i, s := range cp {
		out[i] = domain.LoadInstruction{
			OrderID: s.OrderID, StopSequence: s.StopSequence, SuggestedLoadSequence: n - i,
		}
	}
	return out
}
