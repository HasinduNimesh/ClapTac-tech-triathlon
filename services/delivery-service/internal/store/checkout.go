package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/delivery-service/internal/domain"
)

func (p Postgres) Checkout(ctx context.Context, runID string) (*domain.Checkout, error) {
	var c domain.Checkout
	err := p.Pool.QueryRow(ctx, `SELECT plan_version,status,confirmed_order_ids,missing_order_ids,checked_by,checked_at
		FROM delivery.run_checkouts WHERE run_id=$1::uuid`, runID).Scan(
		&c.PlanVersion, &c.Status, &c.ConfirmedOrderIDs, &c.MissingOrderIDs, &c.CheckedBy, &c.CheckedAt)
	if err == pgx.ErrNoRows { return nil, nil }
	if err != nil { return nil, err }
	return &c, nil
}

func (p Postgres) RecordCheckout(ctx context.Context, runID string, planVersion int, confirmed, missing []string, actor string) (domain.Checkout, bool, error) {
	tx, err := p.Pool.Begin(ctx)
	if err != nil { return domain.Checkout{}, false, err }
	defer func() { _ = tx.Rollback(ctx) }()
	var status string
	var version int
	err = tx.QueryRow(ctx, `SELECT status,plan_version FROM delivery.runs WHERE id=$1::uuid FOR UPDATE`, runID).Scan(&status, &version)
	if err != nil { return domain.Checkout{}, false, err }
	if status != domain.RunPrepared || version != planVersion {
		return domain.Checkout{}, false, fmt.Errorf("conflict: prepared trip or plan version changed")
	}
	state := "confirmed"
	if len(missing) > 0 { state = "blocked" }
	var c domain.Checkout
	err = tx.QueryRow(ctx, `INSERT INTO delivery.run_checkouts
		(run_id,plan_version,status,confirmed_order_ids,missing_order_ids,checked_by)
		VALUES($1::uuid,$2,$3,$4,$5,$6)
		ON CONFLICT(run_id) DO UPDATE SET
		plan_version=EXCLUDED.plan_version,status=EXCLUDED.status,
		confirmed_order_ids=EXCLUDED.confirmed_order_ids,missing_order_ids=EXCLUDED.missing_order_ids,
		checked_by=EXCLUDED.checked_by,checked_at=now()
		WHERE (delivery.run_checkouts.plan_version,delivery.run_checkouts.status,
		       delivery.run_checkouts.confirmed_order_ids,delivery.run_checkouts.missing_order_ids)
		      IS DISTINCT FROM (EXCLUDED.plan_version,EXCLUDED.status,
		       EXCLUDED.confirmed_order_ids,EXCLUDED.missing_order_ids)
		RETURNING plan_version,status,confirmed_order_ids,missing_order_ids,checked_by,checked_at`,
		runID, planVersion, state, confirmed, missing, actor).Scan(
		&c.PlanVersion, &c.Status, &c.ConfirmedOrderIDs, &c.MissingOrderIDs, &c.CheckedBy, &c.CheckedAt)
	changed := err == nil
	if err == pgx.ErrNoRows {
		err = tx.QueryRow(ctx, `SELECT plan_version,status,confirmed_order_ids,missing_order_ids,checked_by,checked_at
			FROM delivery.run_checkouts WHERE run_id=$1::uuid`, runID).Scan(
			&c.PlanVersion, &c.Status, &c.ConfirmedOrderIDs, &c.MissingOrderIDs, &c.CheckedBy, &c.CheckedAt)
	}
	if err != nil { return domain.Checkout{}, false, err }
	if err := tx.Commit(ctx); err != nil { return domain.Checkout{}, false, err }
	return c, changed, nil
}
