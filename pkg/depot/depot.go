// Package depot is the one place that knows how a depot is spelled in each part
// of the system: the official dataset (and the vehicles, outlets and trips built
// from it) names depots "Peliyagoda" and "Kandy", while user profiles and the
// depot selector use DEPOT_NORTH and DEPOT_SOUTH. Comparing them directly makes
// a loader's own trips look like another depot's.
package depot

import "strings"

const (
	North = "DEPOT_NORTH"
	South = "DEPOT_SOUTH"
)

var aliases = map[string]string{
	"DEPOT_NORTH": North, "PELIYAGODA": North,
	"DEPOT_SOUTH": South, "KANDY": South,
}

// Key returns the canonical code for a depot written in any known spelling, or
// the trimmed, upper-cased input when it is not a known depot.
func Key(d string) string {
	k := strings.ToUpper(strings.TrimSpace(d))
	if v, ok := aliases[k]; ok {
		return v
	}
	return k
}

// Same reports whether two depot spellings name the same depot.
func Same(a, b string) bool { return Key(a) == Key(b) }
