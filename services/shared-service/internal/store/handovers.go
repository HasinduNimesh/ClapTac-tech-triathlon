package store

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/audit"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/shared-service/internal/handovers"
	"github.com/jackc/pgx/v5"
)

const handoverColumns = `id::text,role,scope,author_id,author_name,snapshot,published_at,COALESCE(acknowledged_by,''),COALESCE(acknowledged_name,''),acknowledged_at`

func scanHandover(row scanner) (handovers.Record, error) {
	var h handovers.Record
	var raw []byte
	err := row.Scan(&h.ID, &h.Role, &h.Scope, &h.AuthorID, &h.AuthorName, &raw, &h.PublishedAt, &h.AcknowledgedBy, &h.AcknowledgedName, &h.AcknowledgedAt)
	if err != nil {
		return h, err
	}
	err = json.Unmarshal(raw, &h.Snapshot)
	return h, err
}
func (s Store) ListHandovers(ctx context.Context, role, scope string, offset int) ([]handovers.Record, bool, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+handoverColumns+` FROM shared.shift_handovers WHERE role=$1 AND scope=$2 ORDER BY published_at DESC,id DESC LIMIT 21 OFFSET $3`, role, scope, offset)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	items := []handovers.Record{}
	for rows.Next() {
		h, err := scanHandover(rows)
		if err != nil {
			return nil, false, err
		}
		items = append(items, h)
	}
	more := len(items) > 20
	if more {
		items = items[:20]
	}
	return items, more, rows.Err()
}
func (s Store) PublishHandover(ctx context.Context, h handovers.Record, ev audit.Event) (handovers.Record, error) {
	raw, err := json.Marshal(h.Snapshot)
	if err != nil {
		return h, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return h, err
	}
	defer tx.Rollback(ctx)
	// Serialize repeated requests before checking freshness: retries of an already
	// committed report stay idempotent even when its source snapshot has aged.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('shift_handover:' || $1))`, h.ID); err != nil {
		return h, err
	}
	existing, err := scanHandover(tx.QueryRow(ctx, `SELECT `+handoverColumns+` FROM shared.shift_handovers WHERE id=$1::uuid`, h.ID))
	if err == nil {
		previous, _ := json.Marshal(existing.Snapshot)
		if existing.AuthorID != h.AuthorID || existing.Role != h.Role || existing.Scope != h.Scope || !bytes.Equal(previous, raw) {
			return h, fmt.Errorf("conflict: publication ID already used")
		}
		return existing, tx.Commit(ctx)
	}
	if err != pgx.ErrNoRows {
		return h, err
	}
	if !handovers.Fresh(h.Snapshot.AsOf, time.Now()) {
		return h, fmt.Errorf("conflict: refresh the snapshot before publishing")
	}
	saved, err := scanHandover(tx.QueryRow(ctx, `INSERT INTO shared.shift_handovers(id,role,scope,author_id,author_name,snapshot) VALUES($1::uuid,$2,$3,$4,$5,$6) RETURNING `+handoverColumns, h.ID, h.Role, h.Scope, h.AuthorID, h.AuthorName, raw))
	if err != nil {
		return h, err
	}
	ev.ActorID, ev.ActorType, ev.Action, ev.ResourceType, ev.ResourceID, ev.Source = h.AuthorID, "human", "SHIFT_HANDOVER_PUBLISHED", "SHIFT_HANDOVER", h.ID, "shared-service"
	// Do not copy store notes/tasks into the broadly searchable audit stream.
	ev.NewState = map[string]any{"role": h.Role, "scope": h.Scope, "taskCount": len(h.Snapshot.Tasks)}
	if err = s.insertAuditTx(ctx, tx, ev); err != nil {
		return h, err
	}
	return saved, tx.Commit(ctx)
}
func (s Store) AcknowledgeHandover(ctx context.Context, id, role, scope, actor, name string, ev audit.Event) (handovers.Record, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return handovers.Record{}, err
	}
	defer tx.Rollback(ctx)
	h, err := scanHandover(tx.QueryRow(ctx, `SELECT `+handoverColumns+` FROM shared.shift_handovers WHERE id=$1::uuid AND role=$2 AND scope=$3 FOR UPDATE`, id, role, scope))
	if err != nil {
		return h, notFound(err)
	}
	if h.AuthorID == actor {
		return h, fmt.Errorf("conflict: another person must acknowledge the handover")
	}
	if h.AcknowledgedBy != "" {
		if h.AcknowledgedBy != actor {
			return h, fmt.Errorf("conflict: handover already acknowledged")
		}
		return h, tx.Commit(ctx)
	}
	h, err = scanHandover(tx.QueryRow(ctx, `UPDATE shared.shift_handovers SET acknowledged_by=$2,acknowledged_name=$3,acknowledged_at=now() WHERE id=$1::uuid RETURNING `+handoverColumns, id, actor, name))
	if err != nil {
		return h, err
	}
	ev.ActorID, ev.ActorType, ev.Action, ev.ResourceType, ev.ResourceID, ev.Source = actor, "human", "SHIFT_HANDOVER_ACKNOWLEDGED", "SHIFT_HANDOVER", id, "shared-service"
	ev.NewState = map[string]any{"role": role, "scope": scope}
	if err = s.insertAuditTx(ctx, tx, ev); err != nil {
		return h, err
	}
	return h, tx.Commit(ctx)
}
