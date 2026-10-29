package store

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/domain"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/recovery"
	"github.com/jackc/pgx/v5"
	"time"
)

func (p Postgres) SaveRecovery(ctx context.Context, c recovery.Case) (recovery.Case, error) {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return c, err
	}
	defer tx.Rollback(ctx)
	var version int
	var status string
	if err = tx.QueryRow(ctx, `SELECT current_version,status FROM planning.plans WHERE id=$1::uuid FOR UPDATE`, c.PlanID).Scan(&version, &status); err != nil {
		return c, err
	}
	if version != c.PlanVersion || status != "confirmed" {
		return c, fmt.Errorf("conflict: plan changed; generate fresh options")
	}
	if _, err = tx.Exec(ctx, `UPDATE planning.recovery_cases SET payload=jsonb_set(payload,'{status}','"superseded"') WHERE plan_id=$1::uuid AND payload->'incident'->>'id'=$2 AND payload->>'status'='pending'`, c.PlanID, c.Incident.ID); err != nil {
		return c, err
	}
	b, _ := json.Marshal(c)
	if err = tx.QueryRow(ctx, `INSERT INTO planning.recovery_cases(plan_id,snapshot,allocation_snapshot,payload) VALUES($1::uuid,$2,$3,$4) RETURNING id::text`, c.PlanID, c.Snapshot, c.AllocationSnapshot, b).Scan(&c.ID); err != nil {
		return c, err
	}
	return c, tx.Commit(ctx)
}

type recoveryQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func readRecovery(ctx context.Context, q recoveryQuerier, id string) (recovery.Case, error) {
	var c recovery.Case
	var payload []byte
	err := q.QueryRow(ctx, `SELECT payload,snapshot,allocation_snapshot FROM planning.recovery_cases WHERE id::text=$1`, id).Scan(&payload, &c.Snapshot, &c.AllocationSnapshot)
	if err == pgx.ErrNoRows {
		return c, fmt.Errorf("not found")
	}
	if err != nil {
		return c, err
	}
	if err = json.Unmarshal(payload, &c); err != nil {
		return c, err
	}
	c.ID = id
	rows, err := q.Query(ctx, `SELECT outlet_id,candidate_id,note,actor,updated_at FROM planning.recovery_preferences WHERE case_id::text=$1 ORDER BY outlet_id`, id)
	if err != nil {
		return c, err
	}
	defer rows.Close()
	c.Preferences = []recovery.Preference{}
	for rows.Next() {
		var pref recovery.Preference
		if err = rows.Scan(&pref.OutletID, &pref.CandidateID, &pref.Note, &pref.Actor, &pref.UpdatedAt); err != nil {
			return c, err
		}
		c.Preferences = append(c.Preferences, pref)
	}
	return c, rows.Err()
}
func (p Postgres) RecoveryCase(ctx context.Context, id string) (recovery.Case, error) {
	return readRecovery(ctx, p.Pool, id)
}
func (p Postgres) RecoveryCases(ctx context.Context, date string) ([]recovery.Case, error) {
	rows, err := p.Pool.Query(ctx, `SELECT c.id::text FROM planning.recovery_cases c JOIN planning.plans p ON p.id=c.plan_id WHERE p.delivery_date=$1::date AND c.payload->>'status'<>'superseded' ORDER BY c.created_at DESC LIMIT 100`, date)
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	out := []recovery.Case{}
	for _, id := range ids {
		c, err := p.RecoveryCase(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

func (p Postgres) SaveRecoveryPreference(ctx context.Context, c recovery.Case, pref recovery.Preference) error {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var version int
	var status string
	if err = tx.QueryRow(ctx, `SELECT current_version,status FROM planning.plans WHERE id=$1::uuid FOR UPDATE`, c.PlanID).Scan(&version, &status); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `SELECT id FROM planning.recovery_cases WHERE id=$1::uuid FOR UPDATE`, c.ID); err != nil {
		return err
	}
	current, err := readRecovery(ctx, tx, c.ID)
	if err != nil {
		return err
	}
	if version != c.PlanVersion || status != "confirmed" || current.Status != "pending" || !time.Now().Before(current.ExpiresAt) {
		return fmt.Errorf("conflict: options expired or plan changed; refresh recovery")
	}
	_, err = tx.Exec(ctx, `INSERT INTO planning.recovery_preferences(case_id,outlet_id,candidate_id,note,actor) VALUES($1::uuid,$2,$3,$4,$5) ON CONFLICT(case_id,outlet_id) DO UPDATE SET candidate_id=EXCLUDED.candidate_id,note=EXCLUDED.note,actor=EXCLUDED.actor,updated_at=now()`, c.ID, pref.OutletID, pref.CandidateID, pref.Note, pref.Actor)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ApplyRecovery publishes assignments, deferrals, version and approval receipt
// in one transaction. Table locks also cover older planning writers that do not
// yet take a plan lock. They are held only for local SQL, never remote calls.
func (p Postgres) ApplyRecovery(ctx context.Context, c recovery.Case, option recovery.Candidate, actor, note, nextDate string) (recovery.Case, error) {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return c, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `LOCK TABLE planning.plans,planning.trips,planning.allocations,planning.deferrals IN SHARE ROW EXCLUSIVE MODE`); err != nil {
		return c, err
	}
	if _, err = tx.Exec(ctx, `SELECT id FROM planning.recovery_cases WHERE id=$1::uuid FOR UPDATE`, c.ID); err != nil {
		return c, err
	}
	current, err := readRecovery(ctx, tx, c.ID)
	if err != nil {
		return c, err
	}
	if current.Status == "approved" {
		if current.SelectedID == option.ID {
			return current, nil
		}
		return c, fmt.Errorf("conflict: another option was approved")
	}
	if current.Status != "pending" || !time.Now().Before(current.ExpiresAt) {
		return c, fmt.Errorf("conflict: options expired or superseded")
	}
	var version int
	var status string
	if err = tx.QueryRow(ctx, `SELECT current_version,status FROM planning.plans WHERE id=$1::uuid`, c.PlanID).Scan(&version, &status); err != nil {
		return c, err
	}
	if version != c.PlanVersion || status != "confirmed" {
		return c, fmt.Errorf("conflict: plan version changed")
	}
	rows, err := tx.Query(ctx, `SELECT id::text,plan_id::text,order_id,trip_id::text,vehicle_id,sequence,planned_arrival_at,planned_service_start_at,planned_departure_at FROM planning.allocations WHERE plan_id=$1::uuid ORDER BY id::text`, c.PlanID)
	if err != nil {
		return c, err
	}
	allocs := []domain.Allocation{}
	for rows.Next() {
		var a domain.Allocation
		if err = rows.Scan(&a.ID, &a.PlanID, &a.OrderID, &a.TripID, &a.VehicleID, &a.Sequence, &a.PlannedArrivalAt, &a.PlannedServiceStartAt, &a.PlannedDepartureAt); err != nil {
			rows.Close()
			return c, err
		}
		allocs = append(allocs, a)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return c, err
	}
	if recovery.Digest(allocs) != c.AllocationSnapshot {
		return c, fmt.Errorf("conflict: allocations changed; generate fresh options")
	}
	trips := map[string]string{}
	sequences := map[string]int{}
	for _, m := range option.Moves {
		if m.Deferred {
			if _, err = tx.Exec(ctx, `DELETE FROM planning.allocations WHERE id=$1::uuid AND plan_id=$2::uuid`, m.AllocationID, c.PlanID); err != nil {
				return c, err
			}
			if _, err = tx.Exec(ctx, `INSERT INTO planning.deferrals(plan_id,order_id,outlet_id,reason_code,reason_detail,comment,deferred_by,next_run_target) VALUES($1::uuid,$2,$3,'VEHICLE_UNAVAILABLE',$4,$5,$6,$7)`, c.PlanID, m.OrderID, m.OutletID, []byte(`{"recoveryCaseId":"`+c.ID+`"}`), note, actor, nextDate); err != nil {
				return c, err
			}
			continue
		}
		trip := trips[m.VehicleID]
		if trip == "" {
			var busy bool
			if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM planning.trips WHERE plan_id=$1::uuid AND vehicle_id=$2)`, c.PlanID, m.VehicleID).Scan(&busy); err != nil {
				return c, err
			}
			if busy {
				return c, fmt.Errorf("conflict: replacement vehicle is no longer idle")
			}
			if err = tx.QueryRow(ctx, `INSERT INTO planning.trips(plan_id,vehicle_id,trip_number,status) VALUES($1::uuid,$2,1,'confirmed') RETURNING id::text`, c.PlanID, m.VehicleID).Scan(&trip); err != nil {
				return c, err
			}
			trips[m.VehicleID] = trip
		}
		sequences[m.VehicleID]++
		if _, err = tx.Exec(ctx, `UPDATE planning.allocations SET trip_id=$1::uuid,vehicle_id=$2,sequence=$3,planned_arrival_at=$4,planned_service_start_at=$5,planned_departure_at=$6,updated_at=now() WHERE id=$7::uuid AND plan_id=$8::uuid`, trip, m.VehicleID, sequences[m.VehicleID], m.Arrival, m.ServiceStart, m.Departure, m.AllocationID, c.PlanID); err != nil {
			return c, err
		}
	}
	if _, err = tx.Exec(ctx, `DELETE FROM planning.trips t WHERE plan_id=$1::uuid AND vehicle_id=$2 AND NOT EXISTS(SELECT 1 FROM planning.allocations a WHERE a.trip_id=t.id)`, c.PlanID, c.Incident.VehicleID); err != nil {
		return c, err
	}
	version++
	// Hash the committed instruction set rather than a candidate identifier.
	var canonical []byte
	if err = tx.QueryRow(ctx, `SELECT jsonb_build_object('allocations',(SELECT COALESCE(jsonb_agg(to_jsonb(a) ORDER BY a.id),'[]') FROM planning.allocations a WHERE plan_id=$1::uuid),'deferrals',(SELECT COALESCE(jsonb_agg(to_jsonb(d) ORDER BY d.id),'[]') FROM planning.deferrals d WHERE plan_id=$1::uuid))`, c.PlanID).Scan(&canonical); err != nil {
		return c, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO planning.plan_publications(plan_id,version,content_hash,published_by) VALUES($1::uuid,$2,$3,$4)`, c.PlanID, version, recovery.Digest(json.RawMessage(canonical)), actor); err != nil {
		return c, err
	}
	if _, err = tx.Exec(ctx, `UPDATE planning.plans SET current_version=$2,published_at=now(),updated_at=now() WHERE id=$1::uuid`, c.PlanID, version); err != nil {
		return c, err
	}
	current.Status, current.SelectedID, current.ApprovedBy, current.ApprovalNote, current.PublishedVersion = "approved", option.ID, actor, note, version
	b, _ := json.Marshal(current)
	if _, err = tx.Exec(ctx, `UPDATE planning.recovery_cases SET payload=$2 WHERE id=$1::uuid`, c.ID, b); err != nil {
		return c, err
	}
	return current, tx.Commit(ctx)
}
