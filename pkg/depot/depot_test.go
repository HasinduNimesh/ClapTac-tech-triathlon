package depot

import "testing"

func TestSpellingsOfTheSameDepotMatch(t *testing.T) {
	cases := []struct {
		a, b string
		same bool
	}{
		{"Peliyagoda", "DEPOT_NORTH", true},
		{" peliyagoda ", "depot_north", true},
		{"Kandy", "DEPOT_SOUTH", true},
		{"Peliyagoda", "DEPOT_SOUTH", false},
		{"Kandy", "Peliyagoda", false},
		{"Galle", "Galle", true},
		{"", "DEPOT_NORTH", false},
	}
	for _, c := range cases {
		if got := Same(c.a, c.b); got != c.same {
			t.Errorf("Same(%q, %q) = %v, want %v", c.a, c.b, got, c.same)
		}
	}
}
