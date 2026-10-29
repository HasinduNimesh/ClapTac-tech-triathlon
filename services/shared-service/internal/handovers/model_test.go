package handovers

import (
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/authorization"
	"testing"
	"time"
)

func TestScopeRequiresApplicationRoleAndOutlet(t *testing.T) {
	store := &authorization.Profile{UserID: "s", Roles: []string{"STORE_MANAGER"}, OutletIDs: []string{"OUT001"}}
	dispatch := &authorization.Profile{UserID: "d", Roles: []string{"DISPATCHER"}}
	for _, tc := range []struct {
		p           *authorization.Profile
		role, scope string
		want        bool
	}{{store, "store", "OUT001", true}, {store, "store", "OUT002", false}, {store, "dispatcher", "fleet", false}, {dispatch, "dispatcher", "fleet", true}, {dispatch, "store", "OUT001", false}, {nil, "dispatcher", "fleet", false}, {dispatch, "dispatcher", "DEPOT_NORTH", false}} {
		if got := Authorized(tc.p, tc.role, tc.scope); got != tc.want {
			t.Errorf("scope %s/%s got %v", tc.role, tc.scope, got)
		}
	}
}
func TestSnapshotValidationAndFreshness(t *testing.T) {
	now := time.Now()
	s := Snapshot{Title: "Evening shift", AsOf: now, Tasks: []Task{{ID: "receipt:1", Priority: "High", Title: "Confirm receipt", Action: "Review", To: "/store-manager/receipts?order=1"}}}
	if err := Validate(s, "store"); err != nil {
		t.Fatal(err)
	}
	for _, link := range []string{"https://example.com", "//example.com", "/dispatcher/recovery", "/store-manager/../../admin", "javascript:alert(1)"} {
		copy := s
		copy.Tasks = append([]Task{}, s.Tasks...)
		copy.Tasks[0].To = link
		if Validate(copy, "store") == nil {
			t.Errorf("accepted %q", link)
		}
	}
	duplicate := s
	duplicate.Tasks = append(append([]Task{}, s.Tasks...), s.Tasks[0])
	if Validate(duplicate, "store") == nil {
		t.Fatal("accepted duplicate IDs")
	}
	if !Fresh(now, now) || Fresh(now.Add(-6*time.Minute), now) || Fresh(now.Add(time.Minute), now) {
		t.Fatal("freshness bounds")
	}
}
