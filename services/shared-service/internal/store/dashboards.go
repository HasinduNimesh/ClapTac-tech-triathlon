package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/audit"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/shared-service/internal/dashboards"
)

// MaxDashboardsPerUser keeps the Dashboard menu short and bounds storage.
const MaxDashboardsPerUser = 12

type Dashboard struct {
	ID        string          `json:"id"`
	Spec      dashboards.Spec `json:"spec"`
	Version   int             `json:"version"`
	CreatedAt time.Time       `json:"createdAt"`
	UpdatedAt time.Time       `json:"updatedAt"`
}

func (s Store) ListDashboards(ctx context.Context, ownerID string) ([]Dashboard, error) {
	rows, err := s.Pool.Query(ctx, `SELECT id::text,spec,version,created_at,updated_at FROM shared.user_dashboards WHERE owner_user_id=$1 ORDER BY created_at, id`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Dashboard{}
	for rows.Next() {
		d, err := scanDashboard(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s Store) CreateDashboard(ctx context.Context, ownerID string, spec dashboards.Spec, ev audit.Event) (Dashboard, error) {
	raw, err := json.Marshal(spec)
	if err != nil {
		return Dashboard{}, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Dashboard{}, err
	}
	defer tx.Rollback(ctx)
	// Serialise a user's creates so the per-user cap cannot be raced past.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('user_dashboards:' || $1))`, ownerID); err != nil {
		return Dashboard{}, err
	}
	var count int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM shared.user_dashboards WHERE owner_user_id=$1`, ownerID).Scan(&count); err != nil {
		return Dashboard{}, err
	}
	if count >= MaxDashboardsPerUser {
		return Dashboard{}, fmt.Errorf("conflict: dashboard limit reached")
	}
	d, err := scanDashboard(tx.QueryRow(ctx, `INSERT INTO shared.user_dashboards(owner_user_id,name,spec) VALUES($1,$2,$3) RETURNING id::text,spec,version,created_at,updated_at`, ownerID, spec.Name, raw))
	if err != nil {
		return Dashboard{}, err
	}
	if err = s.insertAuditTx(ctx, tx, dashboardEvent(ev, ownerID, d)); err != nil {
		return Dashboard{}, err
	}
	return d, tx.Commit(ctx)
}

func (s Store) UpdateDashboard(ctx context.Context, ownerID, id string, spec dashboards.Spec, expected int, ev audit.Event) (Dashboard, error) {
	raw, err := json.Marshal(spec)
	if err != nil {
		return Dashboard{}, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Dashboard{}, err
	}
	defer tx.Rollback(ctx)
	var current int
	if err = tx.QueryRow(ctx, `SELECT version FROM shared.user_dashboards WHERE id::text=$1 AND owner_user_id=$2 FOR UPDATE`, id, ownerID).Scan(&current); err != nil {
		return Dashboard{}, notFound(err)
	}
	if current != expected {
		return Dashboard{}, fmt.Errorf("conflict: dashboard changed")
	}
	d, err := scanDashboard(tx.QueryRow(ctx, `UPDATE shared.user_dashboards SET name=$3,spec=$4,version=version+1,updated_at=now() WHERE id::text=$1 AND owner_user_id=$2 RETURNING id::text,spec,version,created_at,updated_at`, id, ownerID, spec.Name, raw))
	if err != nil {
		return Dashboard{}, err
	}
	if err = s.insertAuditTx(ctx, tx, dashboardEvent(ev, ownerID, d)); err != nil {
		return Dashboard{}, err
	}
	return d, tx.Commit(ctx)
}

func (s Store) DeleteDashboard(ctx context.Context, ownerID, id string, ev audit.Event) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `DELETE FROM shared.user_dashboards WHERE id::text=$1 AND owner_user_id=$2`, id, ownerID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("not found")
	}
	ev.ActorID, ev.ActorType, ev.ResourceType, ev.ResourceID, ev.Source = ownerID, "human", "USER_DASHBOARD", id, "shared-service"
	if err = s.insertAuditTx(ctx, tx, ev); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func dashboardEvent(ev audit.Event, ownerID string, d Dashboard) audit.Event {
	ev.ActorID, ev.ActorType, ev.ResourceType, ev.ResourceID, ev.Source = ownerID, "human", "USER_DASHBOARD", d.ID, "shared-service"
	ev.NewState = map[string]any{"name": d.Spec.Name, "cards": d.Spec.Cards, "filter": d.Spec.Filter, "version": d.Version}
	return ev
}

func scanDashboard(row scanner) (Dashboard, error) {
	var d Dashboard
	var raw []byte
	if err := row.Scan(&d.ID, &raw, &d.Version, &d.CreatedAt, &d.UpdatedAt); err != nil {
		return d, err
	}
	return d, json.Unmarshal(raw, &d.Spec)
}

func notFound(err error) error {
	if err == pgx.ErrNoRows {
		return fmt.Errorf("not found")
	}
	return err
}
