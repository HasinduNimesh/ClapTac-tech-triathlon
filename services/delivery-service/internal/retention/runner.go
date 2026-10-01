package retention

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/objectstore"
)

const (
	policyVersion = "delivery-proof-retention-v1"
	batchSize     = 100
	maxPerSweep   = 1000
)

// Store performs each erasure while holding the proof row lock. The callback
// removes the object before its database reference and metadata are deleted.
type Store interface {
	ExpiredProofIDs(context.Context, time.Time, int) ([]string, error)
	EraseExpiredProof(context.Context, string, time.Time, time.Time, int, string, func(context.Context, string) error) (bool, error)
}

type Runner struct {
	Store Store
	Files objectstore.Store
	Days  int
	Now   func() time.Time
	Log   *slog.Logger
}

func (r Runner) Sweep(ctx context.Context) (int, error) {
	if r.Store == nil || r.Files == nil {
		return 0, fmt.Errorf("proof retention store and object storage are required")
	}
	if r.Days < 1 || r.Days > 3650 {
		return 0, fmt.Errorf("proof retention days must be between 1 and 3650")
	}
	now := time.Now().UTC()
	if r.Now != nil {
		now = r.Now().UTC()
	}
	cutoff := now.AddDate(0, 0, -r.Days)
	erased := 0
	for erased < maxPerSweep {
		limit := batchSize
		if remaining := maxPerSweep - erased; remaining < limit {
			limit = remaining
		}
		ids, err := r.Store.ExpiredProofIDs(ctx, cutoff, limit)
		if err != nil {
			return erased, err
		}
		if len(ids) == 0 {
			return erased, nil
		}
		for _, id := range ids {
			operationCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			didErase, err := r.Store.EraseExpiredProof(operationCtx, id, cutoff, now, r.Days, policyVersion, r.Files.Delete)
			cancel()
			if err != nil {
				return erased, fmt.Errorf("erase expired delivery proof: %w", err)
			}
			if didErase {
				erased++
			}
		}
		if len(ids) < limit {
			return erased, nil
		}
	}
	if r.Log != nil {
		r.Log.Warn("delivery_proof_retention_batch_limit_reached", "erased", erased, "limit", maxPerSweep)
	}
	return erased, nil
}

func (r Runner) Run(ctx context.Context) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		if count, err := r.Sweep(ctx); err != nil {
			if r.Log != nil {
				r.Log.Error("delivery_proof_retention_sweep_failed", "error", err)
			}
		} else if count > 0 && r.Log != nil {
			r.Log.Info("delivery_proof_retention_sweep_complete", "erased", count, "retention_days", r.Days)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
