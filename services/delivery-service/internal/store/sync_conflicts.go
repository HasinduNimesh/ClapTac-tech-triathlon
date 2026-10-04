package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/delivery-service/internal/domain"
)

const conflictSelect = `SELECT c.id::text, c.run_id, COALESCE(r.trip_id,''), COALESCE(r.vehicle_id,''), COALESCE(r.delivery_date::text,''),
	COALESCE(c.stop_id,''), c.operation_id, c.recorded_plan_version, c.current_plan_version, c.detail, c.created_at, COALESCE(c.settled_by,''), c.settled_at
	FROM delivery.sync_conflicts c LEFT JOIN delivery.runs r ON r.id::text = c.run_id`

func scanConflict(row pgx.Row) (domain.SyncConflict, error) {
	var c domain.SyncConflict
	err := row.Scan(&c.ID, &c.RunID, &c.TripID, &c.VehicleID, &c.DeliveryDate, &c.StopID, &c.OperationID, &c.RecordedPlanVersion, &c.CurrentPlanVersion, &c.Detail, &c.CreatedAt, &c.SettledBy, &c.SettledAt)
	return c, err
}

// InsertSyncConflict records a conflict once per operation id. It reports
// whether a new row was written; a repeat for the same operation is a no-op.
func (p Postgres) InsertSyncConflict(ctx context.Context, c domain.SyncConflict) (bool, error) {
	tag, err := p.Pool.Exec(ctx, `INSERT INTO delivery.sync_conflicts (run_id, stop_id, operation_id, recorded_plan_version, current_plan_version, detail)
		VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT (operation_id) DO NOTHING`,
		c.RunID, nullIfEmpty(c.StopID), c.OperationID, c.RecordedPlanVersion, c.CurrentPlanVersion, c.Detail)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// ListSyncConflicts returns conflicts newest first, optionally for one delivery date.
func (p Postgres) ListSyncConflicts(ctx context.Context, date string, openOnly bool) ([]domain.SyncConflict, error) {
	q := conflictSelect + ` WHERE ($1 = '' OR r.delivery_date::text = $1) AND (NOT $2 OR c.settled_at IS NULL) ORDER BY c.created_at DESC LIMIT 200`
	rows, err := p.Pool.Query(ctx, q, date, openOnly)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.SyncConflict{}
	for rows.Next() {
		c, err := scanConflict(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// SettleSyncConflict marks a conflict settled once; settling again is a no-op
// that returns the stored row.
func (p Postgres) SettleSyncConflict(ctx context.Context, id, actor string) (domain.SyncConflict, error) {
	tag, err := p.Pool.Exec(ctx, `UPDATE delivery.sync_conflicts SET settled_by=$2, settled_at=now() WHERE id::text=$1 AND settled_at IS NULL`, id, actor)
	if err != nil {
		return domain.SyncConflict{}, err
	}
	c, err := scanConflict(p.Pool.QueryRow(ctx, conflictSelect+` WHERE c.id::text=$1`, id))
	if err == pgx.ErrNoRows {
		return domain.SyncConflict{}, fmt.Errorf("not found")
	}
	_ = tag
	return c, err
}
