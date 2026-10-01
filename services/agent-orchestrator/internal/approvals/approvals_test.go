package approvals

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestApprovalOwnerExpiryAndOneDecision(t *testing.T) {
	m := NewMemory()
	now := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	m.now = func() time.Time { return now }
	req, err := m.Propose(context.Background(), Request{Tool: "ConfirmPlan", Args: map[string]any{"planId": "p1"}, ActorID: "user-a"})
	if err != nil {
		t.Fatal(err)
	}
	if req.ArgsHash == "" || !req.ExpiresAt.Equal(now.Add(5*time.Minute)) {
		t.Fatalf("proposal metadata missing: %+v", req)
	}
	if _, err = m.Decide(context.Background(), req.ID, "user-b", true, ""); !errors.Is(err, ErrNotOwner) {
		t.Fatalf("non-owner decision error: %v", err)
	}
	decided, err := m.Decide(context.Background(), req.ID, "user-a", false, "no")
	if err != nil || decided.Status != StatusRejected {
		t.Fatalf("reject failed: %+v %v", decided, err)
	}
	if _, err = m.Decide(context.Background(), req.ID, "user-a", true, ""); !errors.Is(err, ErrAlreadyDecided) {
		t.Fatalf("second decision error: %v", err)
	}
	expiring, err := m.Propose(context.Background(), Request{Tool: "ConfirmPlan", Args: map[string]any{"planId": "p2"}, ActorID: "user-a"})
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(6 * time.Minute)
	if _, err = m.Decide(context.Background(), expiring.ID, "user-a", true, ""); !errors.Is(err, ErrExpired) {
		t.Fatalf("expired decision error: %v", err)
	}
}
