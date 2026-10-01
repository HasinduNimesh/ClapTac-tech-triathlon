package retention

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/objectstore"
)

type fakeStore struct {
	ids          []string
	queryErr     error
	eraseErr     error
	erased       []string
	cutoff       time.Time
	erasureClock time.Time
	days         int
	policy       string
}

func (f *fakeStore) ExpiredProofIDs(_ context.Context, cutoff time.Time, limit int) ([]string, error) {
	f.cutoff = cutoff
	if f.queryErr != nil {
		return nil, f.queryErr
	}
	if len(f.ids) > limit {
		return append([]string(nil), f.ids[:limit]...), nil
	}
	return append([]string(nil), f.ids...), nil
}

func (f *fakeStore) EraseExpiredProof(_ context.Context, id string, cutoff, now time.Time, days int, policy string, deleteObject func(context.Context, string) error) (bool, error) {
	if f.eraseErr != nil {
		return false, f.eraseErr
	}
	if err := deleteObject(context.Background(), "proof/"+id); err != nil {
		return false, err
	}
	f.erased = append(f.erased, id)
	for i, candidate := range f.ids {
		if candidate == id {
			f.ids = append(f.ids[:i], f.ids[i+1:]...)
			break
		}
	}
	f.cutoff, f.erasureClock, f.days, f.policy = cutoff, now, days, policy
	return true, nil
}

func TestSweepUsesCompletionRetentionCutoffAndErasesObjects(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	files := &objectstore.Memory{Objects: map[string][]byte{"proof/old-1": []byte("1"), "proof/old-2": []byte("2")}}
	store := &fakeStore{ids: []string{"old-1", "old-2"}}
	runner := Runner{Store: store, Files: files, Days: 180, Now: func() time.Time { return now }}

	count, err := runner.Sweep(context.Background())
	if err != nil || count != 2 {
		t.Fatalf("Sweep()=(%d,%v), want (2,nil)", count, err)
	}
	if len(files.Objects) != 0 || len(store.erased) != 2 {
		t.Fatalf("erasure incomplete: objects=%v erased=%v", files.Objects, store.erased)
	}
	if want := now.AddDate(0, 0, -180); !store.cutoff.Equal(want) {
		t.Fatalf("cutoff=%s want=%s", store.cutoff, want)
	}
	if store.days != 180 || store.policy != policyVersion || !store.erasureClock.Equal(now) {
		t.Fatalf("audit policy not propagated: days=%d policy=%s at=%s", store.days, store.policy, store.erasureClock)
	}
}

func TestSweepRejectsInvalidConfigurationAndStopsOnStorageFailure(t *testing.T) {
	for _, days := range []int{0, -1, 3651} {
		if _, err := (Runner{Store: &fakeStore{}, Files: &objectstore.Memory{}, Days: days}).Sweep(context.Background()); err == nil {
			t.Fatalf("days=%d accepted", days)
		}
	}
	queryFailure := &fakeStore{queryErr: errors.New("db unavailable")}
	if _, err := (Runner{Store: queryFailure, Files: &objectstore.Memory{}, Days: 180}).Sweep(context.Background()); err == nil {
		t.Fatal("expected query failure")
	}

	files := &failingDeleteStore{}
	store := &fakeStore{ids: []string{"proof-1"}}
	count, err := (Runner{Store: store, Files: files, Days: 180, Log: slog.New(slog.NewTextHandler(discardWriter{}, nil))}).Sweep(context.Background())
	if err == nil || count != 0 || len(store.erased) != 0 {
		t.Fatalf("failed object deletion must not record erasure: count=%d err=%v erased=%v", count, err, store.erased)
	}
}

type failingDeleteStore struct{}

func (failingDeleteStore) Put(context.Context, string, string, []byte) error { return nil }
func (failingDeleteStore) Delete(context.Context, string) error {
	return errors.New("object store unavailable")
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }
