package store

import (
	"context"
	"fmt"
	"sort"
)

type RecoveryStop struct {
	TripID  string `json:"tripId"`
	OrderID string `json:"orderId"`
}

// SuspendRecovery closes the old route to new arrival/completion events. A
// database trigger takes the same lock, including for offline sync mutations.
// Suspension is deliberately durable if downstream publication fails: a broken
// truck must not resume deliveries merely because the planner is unavailable.
func (p Postgres) SuspendRecovery(ctx context.Context, caseID string, stops []RecoveryStop) error {
	if caseID == "" || len(stops) == 0 || len(stops) > 100 {
		return fmt.Errorf("invalid: recovery case and 1–100 stops required")
	}
	sort.Slice(stops, func(i, j int) bool {
		return stops[i].TripID+":"+stops[i].OrderID < stops[j].TripID+":"+stops[j].OrderID
	})
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	for _, s := range stops {
		if s.TripID == "" || s.OrderID == "" {
			return fmt.Errorf("invalid: trip and order required")
		}
		if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,79))`, s.TripID+":"+s.OrderID); err != nil {
			return err
		}
		var completed bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM delivery.stops s JOIN delivery.runs r ON r.id=s.run_id WHERE r.trip_id=$1 AND s.order_id=$2 AND (s.status='completed' OR COALESCE(s.outcome_code,'')<>''))`, s.TripID, s.OrderID).Scan(&completed); err != nil {
			return err
		}
		if completed {
			return fmt.Errorf("conflict: a delivery was completed; generate new recovery options")
		}
		if _, err = tx.Exec(ctx, `INSERT INTO delivery.recovery_suspensions(trip_id,order_id,case_id) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, s.TripID, s.OrderID, caseID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (p Postgres) RecoverySuspended(ctx context.Context, tripID, orderID string) (bool, error) {
	var held bool
	err := p.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM delivery.recovery_suspensions WHERE trip_id=$1 AND order_id=$2)`, tripID, orderID).Scan(&held)
	return held, err
}
