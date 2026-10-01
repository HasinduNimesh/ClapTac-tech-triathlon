package store

import (
	"testing"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/order-service/internal/domain"
)

func TestCustodySameStageReplayWithNewOperationKeyIsIdempotent(t *testing.T) {
	repo := NewMemory()
	order := domain.Order{ID: "tech-order", Brand: "Tech"}
	if _, err := repo.Create(order); err != nil {
		t.Fatal(err)
	}
	loaded := domain.CustodyEvent{Stage: "LOADED", SealID: "SEAL-1", SerialNumbers: []string{"SN-1"}, Condition: "intact", RecordedBy: "USR004", IdempotencyKey: "load-1"}
	first, created, err := repo.AddCustodyEvent(order, loaded)
	if err != nil || !created {
		t.Fatalf("first event created=%v err=%v", created, err)
	}

	replay := loaded
	replay.IdempotencyKey = "load-rapid-retry"
	previous, created, err := repo.AddCustodyEvent(order, replay)
	if err != nil || created || previous.ID != first.ID {
		t.Fatalf("same-stage replay result=%+v created=%v err=%v", previous, created, err)
	}

	changed := replay
	changed.IdempotencyKey = "load-changed-payload"
	changed.Condition = "box damaged"
	if _, _, err := repo.AddCustodyEvent(order, changed); err == nil {
		t.Fatal("same-stage replay with changed condition was accepted")
	}
	events, err := repo.ListCustodyEvents(order.ID)
	if err != nil || len(events) != 1 {
		t.Fatalf("ledger events=%d err=%v, want one immutable stage", len(events), err)
	}
}
