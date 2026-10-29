package service

import (
	"encoding/json"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/authorization"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/recovery"
	"strings"
	"testing"
)

func TestRecoveryStoreIsolation(t *testing.T) {
	c := recovery.Case{Incident: recovery.Incident{Description: "private incident"}, ApprovalNote: "private decision", ApprovedBy: "dispatcher", Candidates: []recovery.Candidate{{ID: "option", DistanceKm: 99, Moves: []recovery.Move{{OrderID: "own", OutletID: "store-a", VehicleID: "van", AllocationID: "internal-allocation", SourceTripID: "internal-trip"}, {OrderID: "secret-order", OutletID: "store-b", Deferred: true}}}}, Preferences: []recovery.Preference{{OutletID: "store-a", Actor: "private-actor"}, {OutletID: "store-b", Note: "secret-note"}}}
	store := &authorization.Profile{Roles: []string{authorization.RoleStoreManager}, OutletIDs: []string{"store-a"}}
	view, ok := scopeRecovery(c, store)
	if !ok {
		t.Fatal("own store should see case")
	}
	b, _ := json.Marshal(view)
	for _, secret := range []string{"secret-order", "store-b", "secret-note", "private", "internal-"} {
		if strings.Contains(string(b), secret) {
			t.Fatalf("leaked %q: %s", secret, b)
		}
	}
	if view.Candidates[0].Recovered != 1 || view.Candidates[0].Deferred != 0 || view.Candidates[0].DistanceKm != 0 {
		t.Fatal("fleet totals leaked")
	}
	if _, ok := scopeRecovery(c, &authorization.Profile{OutletIDs: []string{"elsewhere"}}); ok {
		t.Fatal("unaffected store sees case")
	}
	if _, ok := scopeRecovery(c, nil); ok {
		t.Fatal("missing profile sees case")
	}
	// Projection must not mutate the source used by subsequent readers.
	if len(c.Candidates[0].Moves) != 2 || c.Candidates[0].Moves[0].AllocationID == "" {
		t.Fatal("scope mutated original")
	}
}
