package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/delivery-service/internal/domain"
)

type Postgres struct {
	Pool *pgxpool.Pool
}

func (p Postgres) SendTripMessage(ctx context.Context, tripID, stopID, body, actor string) (domain.TripMessage, error) {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return domain.TripMessage{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var runID string
	if err = tx.QueryRow(ctx, `SELECT id::text FROM delivery.runs WHERE trip_id=$1`, tripID).Scan(&runID); err != nil {
		return domain.TripMessage{}, err
	}
	var stop any
	if stopID != "" {
		var exists bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM delivery.stops WHERE id=$1::uuid AND run_id=$2::uuid)`, stopID, runID).Scan(&exists); err != nil {
			return domain.TripMessage{}, err
		}
		if !exists {
			return domain.TripMessage{}, fmt.Errorf("stop does not belong to trip")
		}
		stop = stopID
	}
	var m domain.TripMessage
	err = tx.QueryRow(ctx, `INSERT INTO delivery.trip_messages(run_id,stop_id,body,sent_by) VALUES($1::uuid,$2::uuid,$3,$4) RETURNING id::text,COALESCE(stop_id::text,''),body,sent_by,created_at`, runID, stop, body, actor).Scan(&m.ID, &m.StopID, &m.Body, &m.SentBy, &m.CreatedAt)
	if err != nil {
		return m, err
	}
	m.TripID = tripID
	if _, err = tx.Exec(ctx, `INSERT INTO delivery.trip_message_events(message_id,event_type,actor_id) VALUES($1::uuid,'SENT',$2)`, m.ID, actor); err != nil {
		return m, err
	}
	if err = tx.Commit(ctx); err != nil {
		return m, err
	}
	return m, nil
}

func (p Postgres) ListTripMessages(ctx context.Context, tripID string) ([]domain.TripMessage, error) {
	rows, err := p.Pool.Query(ctx, `SELECT m.id::text,r.trip_id,COALESCE(m.stop_id::text,''),m.body,m.sent_by,m.created_at,COALESCE(m.acknowledged_by,''),m.acknowledged_at FROM delivery.trip_messages m JOIN delivery.runs r ON r.id=m.run_id WHERE r.trip_id=$1 ORDER BY m.created_at,m.id`, tripID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.TripMessage{}
	for rows.Next() {
		var m domain.TripMessage
		if err := rows.Scan(&m.ID, &m.TripID, &m.StopID, &m.Body, &m.SentBy, &m.CreatedAt, &m.AcknowledgedBy, &m.AcknowledgedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (p Postgres) AcknowledgeTripMessage(ctx context.Context, tripID, messageID, actor string) (domain.TripMessage, error) {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return domain.TripMessage{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var m domain.TripMessage
	err = tx.QueryRow(ctx, `UPDATE delivery.trip_messages SET acknowledged_by=COALESCE(acknowledged_by,$3),acknowledged_at=COALESCE(acknowledged_at,now()) WHERE id=$1::uuid AND run_id=(SELECT id FROM delivery.runs WHERE trip_id=$2) RETURNING id::text,$2,COALESCE(stop_id::text,''),body,sent_by,created_at,acknowledged_by,acknowledged_at`, messageID, tripID, actor).Scan(&m.ID, &m.TripID, &m.StopID, &m.Body, &m.SentBy, &m.CreatedAt, &m.AcknowledgedBy, &m.AcknowledgedAt)
	if err != nil {
		return m, err
	}
	if m.AcknowledgedBy == actor {
		if _, err = tx.Exec(ctx, `INSERT INTO delivery.trip_message_events(message_id,event_type,actor_id) SELECT $1::uuid,'ACKNOWLEDGED',$2 WHERE NOT EXISTS(SELECT 1 FROM delivery.trip_message_events WHERE message_id=$1::uuid AND event_type='ACKNOWLEDGED' AND actor_id=$2)`, messageID, actor); err != nil {
			return m, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return m, err
	}
	return m, nil
}

func (p Postgres) GetByTrip(ctx context.Context, tripID string) (domain.Run, error) {
	row := p.Pool.QueryRow(ctx, runSelect+" WHERE trip_id = $1", tripID)
	return scanRun(row)
}

// LatenessHistory reports a depot/brand/temperature prior from the 90 calendar days before
// the selected trip. Only driver-recorded arrivals with valid delivery windows
// are counted; order outcomes and planned ETAs are not substituted for arrival.
func (p Postgres) LatenessHistory(ctx context.Context, tripID string) ([]domain.LatenessProbability, error) {
	query := fmt.Sprintf(`
		WITH current_run AS (
			SELECT id, delivery_date, depot
			FROM delivery.runs
			WHERE trip_id = $1
		), current_brands AS (
			SELECT DISTINCT run_id, brand, temperature_requirement
			FROM delivery.stops
			WHERE run_id = (SELECT id FROM current_run)
		), observed AS (
			SELECT r.depot, s.brand, s.temperature_requirement, r.delivery_date,
				CASE WHEN s.arrived_at >
					((r.delivery_date + s.planned_window_close::time) AT TIME ZONE 'Asia/Colombo')
					THEN 1.0 ELSE 0.0 END AS late,
				EXTRACT(EPOCH FROM (s.arrived_at - s.planned_arrival_at)) / 60.0 AS arrival_offset_minutes
			FROM delivery.runs r
			JOIN delivery.stops s ON s.run_id = r.id
			JOIN current_run c ON r.depot = c.depot
			WHERE r.delivery_date >= c.delivery_date - $2::int
			  AND r.delivery_date < c.delivery_date
			  AND s.arrived_at IS NOT NULL
			  AND s.planned_window_close ~ '^([01][0-9]|2[0-3]):[0-5][0-9](:[0-5][0-9])?$'
			  AND (s.brand, s.temperature_requirement) IN (SELECT brand, temperature_requirement FROM current_brands)
		), history AS (
			SELECT brand, temperature_requirement, COUNT(*)::int AS sample_count, SUM(late)::int AS late_count
			FROM observed o
			JOIN current_run c ON TRUE
			WHERE o.delivery_date >= c.delivery_date - $3::int
			GROUP BY brand, temperature_requirement
		), scored AS (
			SELECT o.brand, o.temperature_requirement, o.delivery_date, o.late,
				COUNT(*) OVER (
					PARTITION BY o.depot, o.brand, o.temperature_requirement ORDER BY o.delivery_date::timestamp
					RANGE BETWEEN INTERVAL '%d days' PRECEDING AND INTERVAL '1 day' PRECEDING
				) AS prior_count,
				COALESCE(SUM(o.late) OVER (
					PARTITION BY o.depot, o.brand, o.temperature_requirement ORDER BY o.delivery_date::timestamp
					RANGE BETWEEN INTERVAL '%d days' PRECEDING AND INTERVAL '1 day' PRECEDING
				), 0) AS prior_late
			FROM observed o
		), calibration AS (
			SELECT brand, temperature_requirement,
				COUNT(*) FILTER (WHERE prior_count >= $4::int)::int AS sample_count,
				AVG(POWER((prior_late + 1)::float8 / (prior_count + 2) - late, 2))
					FILTER (WHERE prior_count >= $4::int) AS brier_score
			FROM scored
			GROUP BY brand, temperature_requirement
		), range_history AS (
			SELECT o.*
			FROM observed o
			WHERE o.arrival_offset_minutes IS NOT NULL
		), range_current AS (
			SELECT h.brand, h.temperature_requirement, COUNT(*)::int AS sample_count,
				percentile_cont(0.1) WITHIN GROUP (ORDER BY h.arrival_offset_minutes) AS p10,
				percentile_cont(0.9) WITHIN GROUP (ORDER BY h.arrival_offset_minutes) AS p90
			FROM range_history h
			JOIN current_run c ON TRUE
			WHERE h.delivery_date >= c.delivery_date - $3::int
			GROUP BY h.brand, h.temperature_requirement
		), range_holdouts AS (
			SELECT sample.brand, sample.temperature_requirement, sample.delivery_date, sample.arrival_offset_minutes,
				prior.sample_count, prior.p10, prior.p90
			FROM range_history sample
			CROSS JOIN LATERAL (
				SELECT COUNT(*)::int AS sample_count,
					percentile_cont(0.1) WITHIN GROUP (ORDER BY history.arrival_offset_minutes) AS p10,
					percentile_cont(0.9) WITHIN GROUP (ORDER BY history.arrival_offset_minutes) AS p90
				FROM range_history history
				WHERE history.depot = sample.depot
				  AND history.brand = sample.brand
				  AND history.temperature_requirement = sample.temperature_requirement
				  AND history.delivery_date >= sample.delivery_date - $3::int
				  AND history.delivery_date < sample.delivery_date
			) prior
		), range_calibration AS (
			SELECT brand, temperature_requirement,
				COUNT(*) FILTER (WHERE sample_count >= $5::int)::int AS holdout_count,
				AVG(CASE WHEN arrival_offset_minutes BETWEEN p10 AND p90 THEN 1.0 ELSE 0.0 END)
					FILTER (WHERE sample_count >= $5::int) AS coverage
			FROM range_holdouts
			GROUP BY brand, temperature_requirement
		)
		SELECT c.depot, b.brand, b.temperature_requirement, COALESCE(h.sample_count, 0), COALESCE(h.late_count, 0),
			COALESCE(cal.sample_count, 0), cal.brier_score,
			COALESCE(rc.sample_count, 0), rc.p10, rc.p90,
			COALESCE(rcl.holdout_count, 0), rcl.coverage
		FROM current_run c
		JOIN current_brands b ON b.run_id = c.id
		LEFT JOIN history h ON h.brand = b.brand AND h.temperature_requirement = b.temperature_requirement
		LEFT JOIN calibration cal ON cal.brand = b.brand AND cal.temperature_requirement = b.temperature_requirement
		LEFT JOIN range_current rc ON rc.brand = b.brand AND rc.temperature_requirement = b.temperature_requirement
		LEFT JOIN range_calibration rcl ON rcl.brand = b.brand AND rcl.temperature_requirement = b.temperature_requirement
		ORDER BY b.brand, b.temperature_requirement`, domain.LatenessHistoryDays, domain.LatenessHistoryDays)
	rows, err := p.Pool.Query(ctx, query, tripID, domain.LatenessBacktestDays, domain.LatenessHistoryDays, domain.LatenessMinSamples, domain.ArrivalRangeMinTraining)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []domain.LatenessProbability{}
	for rows.Next() {
		var depot, brand, temperatureRequirement string
		var samples, late, calibrationSamples int
		var brierScore *float64
		var rangeSamples, rangeHoldouts int
		var rangeP10, rangeP90, rangeCoverage *float64
		if err := rows.Scan(&depot, &brand, &temperatureRequirement, &samples, &late, &calibrationSamples, &brierScore,
			&rangeSamples, &rangeP10, &rangeP90, &rangeHoldouts, &rangeCoverage); err != nil {
			return nil, err
		}
		item := domain.EstimateLatenessProbability(depot, brand, samples, late)
		item.TemperatureRequirement = temperatureRequirement
		item = domain.ApplyLatenessCalibration(item, calibrationSamples, brierScore)
		items = append(items, domain.ApplyArrivalRange(item, rangeSamples, rangeHoldouts, rangeCoverage, rangeP10, rangeP90))
	}
	return items, rows.Err()
}

func (p Postgres) GetRun(ctx context.Context, id string) (domain.Run, error) {
	row := p.Pool.QueryRow(ctx, runSelect+" WHERE id::text = $1", id)
	return scanRun(row)
}

func (p Postgres) ListRuns(ctx context.Context, date, vehicleID string) ([]domain.Run, error) {
	q := runSelect + " WHERE delivery_date::text = $1"
	args := []any{date}
	if vehicleID != "" {
		q += " AND vehicle_id = $2"
		args = append(args, vehicleID)
	}
	q += " ORDER BY trip_number, trip_id"
	rows, err := p.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Run
	for rows.Next() {
		r, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if out == nil {
		out = []domain.Run{}
	}
	return out, rows.Err()
}

func (p Postgres) PrepareTx(ctx context.Context, run domain.Run, stops []domain.Stop) (domain.Run, error) {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return domain.Run{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	row := tx.QueryRow(ctx, `
		INSERT INTO runs (trip_id, plan_id, plan_ref, delivery_date, vehicle_id, depot, trip_number, vehicle_type, vehicle_temperature_capability, status,plan_version)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		RETURNING `+runReturning, run.TripID, run.PlanID, run.PlanRef, run.DeliveryDate, run.VehicleID, run.Depot, run.TripNumber, run.VehicleType, run.VehicleTemperatureCapability, domain.RunPrepared, run.PlanVersion)
	created, err := scanRun(row)
	if err != nil {
		return domain.Run{}, err
	}
	for _, st := range stops {
		short, _ := json.Marshal(st.LoadingShortfallSummary)
		if st.LoadingShortfallSummary == nil {
			short = []byte("[]")
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO stops (run_id, allocation_id, order_id, order_ref, expected_units, unit_label, outlet_id, brand, outlet_name, district, dock_type, parking_constraint,
				stop_sequence, planned_arrival_at, temperature_requirement, chilled_temperature_min_c, chilled_temperature_max_c, planned_window_open, planned_window_close, access_instructions, access_instructions_updated_at, loading_status, loading_shortfall_summary, status)
				VALUES ($1::uuid,$2,$3,$4,$5,COALESCE(NULLIF($6,''),'units'),$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23::jsonb,$24)
		`, created.ID, st.AllocationID, st.OrderID, st.OrderRef, st.ExpectedUnits, st.UnitLabel, st.OutletID, st.Brand, st.OutletName, st.District, st.DockType, st.ParkingConstraint,
			st.StopSequence, st.PlannedArrivalAt, st.TemperatureRequirement, st.ChilledTemperatureMinC, st.ChilledTemperatureMaxC, st.PlannedWindowOpen, st.PlannedWindowClose, st.AccessInstructions, st.AccessInstructionsUpdatedAt, st.LoadingStatus, string(short), domain.StopPending)
		if err != nil {
			return domain.Run{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Run{}, err
	}
	return created, nil
}

func (p Postgres) StartRun(ctx context.Context, runID, actor string) (domain.Run, error) {
	row := p.Pool.QueryRow(ctx, `
		UPDATE runs SET status = $2, started_by = $3, started_at = now(), updated_at = now(), version = version + 1
		WHERE id::text = $1 AND status = $4 AND EXISTS (
			SELECT 1 FROM delivery.run_checkouts c WHERE c.run_id=runs.id AND c.plan_version=runs.plan_version AND c.status='confirmed')
		RETURNING `+runReturning, runID, domain.RunInProgress, actor, domain.RunPrepared)
	return scanRun(row)
}

func (p Postgres) CompleteRun(ctx context.Context, runID, actor string) (domain.Run, error) {
	row := p.Pool.QueryRow(ctx, `
		UPDATE runs SET status = $2, completed_by = $3, completed_at = now(), updated_at = now(), version = version + 1
		WHERE id::text = $1 AND status = $4
		RETURNING `+runReturning, runID, domain.RunCompleted, actor, domain.RunInProgress)
	return scanRun(row)
}

func (p Postgres) ListStops(ctx context.Context, runID string) ([]domain.Stop, error) {
	rows, err := p.Pool.Query(ctx, stopSelect+" WHERE run_id::text = $1 ORDER BY stop_sequence", runID)
	if err != nil {
		return nil, err
	}
	var out []domain.Stop
	for rows.Next() {
		s, err := scanStop(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	readings, err := p.Pool.Query(ctx, `SELECT stop_id::text,operation_id,value_c,occurred_at,received_at,actor_id,source,evaluation,min_c_snapshot,max_c_snapshot,note FROM delivery.temperature_readings WHERE run_id=$1::uuid ORDER BY occurred_at,received_at`, runID)
	if err != nil {
		return nil, err
	}
	defer readings.Close()
	byID := make(map[string]int, len(out))
	for i := range out {
		byID[out[i].ID] = i
		out[i].TemperatureReadings = []domain.TemperatureReading{}
	}
	for readings.Next() {
		var stopID string
		var item domain.TemperatureReading
		var minC, maxC sql.NullFloat64
		if err := readings.Scan(&stopID, &item.OperationID, &item.ValueC, &item.OccurredAt, &item.ReceivedAt, &item.ActorID, &item.Source, &item.Evaluation, &minC, &maxC, &item.Note); err != nil {
			return nil, err
		}
		item.Unit = "C"
		if minC.Valid {
			v := minC.Float64
			item.MinC = &v
		}
		if maxC.Valid {
			v := maxC.Float64
			item.MaxC = &v
		}
		if i, ok := byID[stopID]; ok {
			out[i].TemperatureReadings = append(out[i].TemperatureReadings, item)
		}
	}
	if err := readings.Err(); err != nil {
		return nil, err
	}
	if out == nil {
		out = []domain.Stop{}
	}
	return out, rows.Err()
}

func (p Postgres) GetStop(ctx context.Context, runID, stopID string) (domain.Stop, error) {
	row := p.Pool.QueryRow(ctx, stopSelect+" WHERE run_id::text = $1 AND id::text = $2", runID, stopID)
	return scanStop(row)
}

// RecordTemperatureReading atomically stores immutable evidence and its sync receipt.
// The source column includes a future sensor adapter seam; the driver path is manual only.
func (p Postgres) RecordTemperatureReading(ctx context.Context, runID, stopID, operationID, actorID string, occurredAt time.Time, value float64, note string) (domain.TemperatureReading, error) {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return domain.TemperatureReading{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var requirement, status string
	var minC, maxC sql.NullFloat64
	err = tx.QueryRow(ctx, `SELECT temperature_requirement,status,chilled_temperature_min_c,chilled_temperature_max_c FROM delivery.stops WHERE id=$1::uuid AND run_id=$2::uuid FOR UPDATE`, stopID, runID).Scan(&requirement, &status, &minC, &maxC)
	if err != nil {
		return domain.TemperatureReading{}, err
	}
	if requirement != "chilled" {
		return domain.TemperatureReading{}, fmt.Errorf("rejected: temperature readings are only supported for chilled deliveries")
	}
	if status != domain.StopArrived {
		return domain.TemperatureReading{}, fmt.Errorf("rejected: arrive before recording a temperature reading")
	}
	item := domain.TemperatureReading{OperationID: operationID, ValueC: value, Unit: "C", OccurredAt: occurredAt, ActorID: actorID, Source: "manual", Note: note}
	if minC.Valid {
		v := minC.Float64
		item.MinC = &v
	}
	if maxC.Valid {
		v := maxC.Float64
		item.MaxC = &v
	}
	switch {
	case !minC.Valid || !maxC.Valid:
		item.Evaluation = "LIMITS_UNCONFIGURED"
	case value < minC.Float64 || value > maxC.Float64:
		item.Evaluation = "OUT_OF_RANGE"
	default:
		item.Evaluation = "IN_RANGE"
	}
	err = tx.QueryRow(ctx, `INSERT INTO delivery.temperature_readings(operation_id,run_id,stop_id,value_c,occurred_at,actor_id,source,min_c_snapshot,max_c_snapshot,evaluation,note) VALUES($1,$2::uuid,$3::uuid,$4,$5,$6,'manual',$7,$8,$9,$10) RETURNING received_at`, operationID, runID, stopID, value, occurredAt, actorID, minC, maxC, item.Evaluation, note).Scan(&item.ReceivedAt)
	if err != nil {
		return domain.TemperatureReading{}, err
	}
	payload, _ := json.Marshal(map[string]any{"reading": item})
	_, err = tx.Exec(ctx, `INSERT INTO delivery.sync_operations(operation_id,run_id,stop_id,operation_type,occurred_at,result_status,result_payload) VALUES($1,$2,$3,$4,$5,'APPLIED',$6::jsonb)`, operationID, runID, stopID, domain.OpTemperatureReading, occurredAt, payload)
	if err != nil {
		return domain.TemperatureReading{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.TemperatureReading{}, err
	}
	return item, nil
}

// InsertDriverIncident records a FR-22 categorised field report and its
// matching sync-operation ledger row in one transaction, mirroring how a
// temperature reading is recorded.
func (p Postgres) InsertDriverIncident(ctx context.Context, runID, stopID, operationID, category, description, reportedBy string, occurredAt time.Time) (domain.DriverIncident, error) {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return domain.DriverIncident{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	item := domain.DriverIncident{OperationID: operationID, RunID: runID, StopID: stopID, Category: category, Description: description, ReportedBy: reportedBy, OccurredAt: occurredAt}
	err = tx.QueryRow(ctx, `
		INSERT INTO delivery.driver_incidents(run_id, stop_id, operation_id, category, description, reported_by, occurred_at)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7)
		RETURNING id::text, created_at
	`, runID, nullIfEmpty(stopID), operationID, category, description, reportedBy, occurredAt).Scan(&item.ID, &item.CreatedAt)
	if err != nil {
		return domain.DriverIncident{}, err
	}
	payload, _ := json.Marshal(map[string]any{"incident": item})
	if _, err = tx.Exec(ctx, `INSERT INTO delivery.sync_operations(operation_id,run_id,stop_id,operation_type,occurred_at,result_status,result_payload) VALUES($1,$2,$3,$4,$5,'APPLIED',$6::jsonb)`, operationID, runID, nullIfEmpty(stopID), domain.OpIncidentReport, occurredAt, payload); err != nil {
		return domain.DriverIncident{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.DriverIncident{}, err
	}
	return item, nil
}

func (p Postgres) OrderTracking(ctx context.Context, orderID string) (domain.OrderTracking, error) {
	var t domain.OrderTracking
	var short []byte
	err := p.Pool.QueryRow(ctx, `SELECT r.id::text,r.trip_id,r.vehicle_id,r.depot,r.status,s.id::text,COALESCE(s.outcome_code,''),COALESCE(s.outcome_reason,''),s.outcome_at,s.completed_at,s.loading_shortfall_summary,s.delivered_units
		FROM stops s JOIN runs r ON r.id=s.run_id WHERE s.order_id=$1
		ORDER BY r.updated_at DESC,COALESCE(s.completed_at,s.outcome_received_at,s.arrived_at) DESC NULLS LAST LIMIT 1`, orderID).Scan(&t.RunID, &t.TripID, &t.VehicleID, &t.Depot, &t.RunStatus, &t.StopID, &t.Outcome, &t.Reason, &t.OccurredAt, &t.CompletedAt, &short, &t.DeliveredUnits)

	if err == pgx.ErrNoRows {
		return t, fmt.Errorf("not found")
	}
	if err != nil {
		return t, err
	}
	if t.RunStatus == domain.RunInProgress {
		point, err := p.ActiveLocation(ctx, t.TripID)
		if err == nil {
			t.Location = point
		}
		prediction, e := p.ArrivalPrediction(ctx, t.StopID)
		if e == nil {
			t.ArrivalPrediction = prediction
		}
	}
	if t.Outcome == domain.OutcomeRefused {
		returned, e := p.ReturnedGoods(ctx, t.StopID)
		if e == nil {
			t.ReturnedGoods = returned
		}

	}
	_ = json.Unmarshal(short, &t.LoadingShortfallSummary)
	if t.LoadingShortfallSummary == nil {
		t.LoadingShortfallSummary = []any{}
	}
	rows, err := p.Pool.Query(ctx, `SELECT idempotency_key,proof_type,mime_type,uploaded_at,pending,COALESCE(receiver_name,'') FROM proofs WHERE stop_id=$1::uuid ORDER BY created_at`, t.StopID)
	if err != nil {
		return t, err
	}
	defer rows.Close()
	t.Proofs = []domain.ProofSummary{}
	for rows.Next() {
		var pr domain.ProofSummary
		if err := rows.Scan(&pr.OperationID, &pr.Type, &pr.MimeType, &pr.UploadedAt, &pr.Pending, &pr.ReceiverName); err != nil {
			return t, err
		}
		t.Proofs = append(t.Proofs, pr)
	}
	return t, rows.Err()
}

func (p Postgres) OutletLastServed(ctx context.Context) ([]domain.OutletLastServed, error) {
	rows, err := p.Pool.Query(ctx, `
		SELECT outlet_id, MAX(COALESCE(outcome_at, outcome_received_at))
		FROM stops
		WHERE outlet_id IS NOT NULL AND outlet_id <> ''
		  AND outcome_code IN ('DELIVERED', 'PARTIAL')
		GROUP BY outlet_id
		ORDER BY outlet_id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]domain.OutletLastServed, 0)
	for rows.Next() {
		var item domain.OutletLastServed
		if err := rows.Scan(&item.OutletID, &item.LastServedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// OutletLastAttempted covers every terminal outcome (DELIVERED, PARTIAL,
// NOT_DELIVERED, FAILED, REFUSED) via "outcome_code is set" rather than an
// explicit list, so a future outcome code is still picked up automatically.
// beforeDate scopes it to attempts strictly before that date - required so
// reopening an older plan is judged by what had actually happened as of
// that plan's date, not contaminated by attempts recorded since.
//
// outcome_at/outcome_received_at are timestamptz; beforeDate is a plain
// calendar date in Asia/Colombo (NFR-20). Comparing a timestamptz directly
// against ::date lets Postgres cast using the session/DB timezone (UTC by
// default here), which is the wrong midnight - an outcome at 19:00Z the day
// before is already past midnight in Colombo (+5:30) and must count as
// "that day", not "before it". AT TIME ZONE 'Asia/Colombo' anchors the
// cutoff explicitly, matching the existing pattern in LatenessHistory below.
func (p Postgres) OutletLastAttempted(ctx context.Context, beforeDate string) ([]domain.OutletLastAttempted, error) {
	rows, err := p.Pool.Query(ctx, `
		SELECT outlet_id, MAX(COALESCE(outcome_at, outcome_received_at))
		FROM stops
		WHERE outlet_id IS NOT NULL AND outlet_id <> ''
		  AND outcome_code IS NOT NULL AND outcome_code <> ''
		  AND COALESCE(outcome_at, outcome_received_at) < ($1::date::timestamp AT TIME ZONE 'Asia/Colombo')
		GROUP BY outlet_id
		ORDER BY outlet_id
	`, beforeDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]domain.OutletLastAttempted, 0)
	for rows.Next() {
		var item domain.OutletLastAttempted
		if err := rows.Scan(&item.OutletID, &item.LastAttemptedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (p Postgres) MarkArrived(ctx context.Context, stopID string, occurredAt time.Time) error {
	tag, err := p.Pool.Exec(ctx, `
		UPDATE stops SET status = $2, arrived_at = $3, arrived_received_at = now(), updated_at = now(), version = version + 1
		WHERE id::text = $1 AND status = $4
	`, stopID, domain.StopArrived, occurredAt, domain.StopPending)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("conflict: stop cannot be marked arrived")
	}
	return nil
}

func (p Postgres) MarkOutcome(ctx context.Context, stopID, code, reason, note string, occurredAt time.Time, deliveredUnits *int) error {
	tag, err := p.Pool.Exec(ctx, `
		UPDATE stops SET status = $2, outcome_code = $3, outcome_reason = $4, outcome_note = $5,
			outcome_at = $6, delivered_units = $7, outcome_received_at = now(), completed_at = now(), updated_at = now(), version = version + 1
		WHERE id::text = $1 AND status = $8
	`, stopID, domain.StopCompleted, code, reason, note, occurredAt, deliveredUnits, domain.StopArrived)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("conflict: stop cannot record outcome")
	}
	return nil
}

func (p Postgres) IncompleteStopIDs(ctx context.Context, runID string) ([]string, error) {
	rows, err := p.Pool.Query(ctx, `SELECT id::text FROM stops WHERE run_id::text = $1 AND status <> $2 ORDER BY stop_sequence`, runID, domain.StopCompleted)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if ids == nil {
		ids = []string{}
	}
	return ids, rows.Err()
}

func (p Postgres) GetOp(ctx context.Context, operationID string) (domain.SyncOp, error) {
	row := p.Pool.QueryRow(ctx, `
		SELECT operation_id, run_id, COALESCE(stop_id,''), operation_type, occurred_at, received_at, result_status, result_payload
		FROM sync_operations WHERE operation_id = $1`, operationID)
	var op domain.SyncOp
	var occurred *time.Time
	var payload []byte
	err := row.Scan(&op.OperationID, &op.RunID, &op.StopID, &op.OperationType, &occurred, &op.ReceivedAt, &op.ResultStatus, &payload)
	if err == pgx.ErrNoRows {
		return op, fmt.Errorf("not found")
	}
	if err != nil {
		return op, err
	}
	op.OccurredAt = occurred
	_ = json.Unmarshal(payload, &op.ResultPayload)
	if op.ResultPayload == nil {
		op.ResultPayload = map[string]any{}
	}
	return op, nil
}

func (p Postgres) InsertOp(ctx context.Context, op domain.SyncOp) error {
	payload, _ := json.Marshal(op.ResultPayload)
	if op.ResultPayload == nil {
		payload = []byte("{}")
	}
	_, err := p.Pool.Exec(ctx, `
		INSERT INTO sync_operations (operation_id, run_id, stop_id, operation_type, occurred_at, result_status, result_payload)
		VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb)
	`, op.OperationID, op.RunID, nullIfEmpty(op.StopID), op.OperationType, op.OccurredAt, op.ResultStatus, string(payload))
	return err
}

func (p Postgres) InsertProof(ctx context.Context, pr domain.Proof) (domain.Proof, error) {
	row := p.Pool.QueryRow(ctx, `
		INSERT INTO proofs (stop_id, proof_type, object_key, mime_type, sha256, captured_at, created_by, idempotency_key, pending, receiver_name)
		VALUES ($1::uuid,$2,$3,$4,$5,$6,$7,$8,true,$9)
		RETURNING id::text, stop_id::text, proof_type, object_key, mime_type, COALESCE(sha256,''), captured_at, uploaded_at, created_by, idempotency_key, pending, COALESCE(receiver_name,'')
	`, pr.StopID, pr.ProofType, pr.ObjectKey, pr.MimeType, pr.SHA256, pr.CapturedAt, pr.CreatedBy, pr.IdempotencyKey, nullIfEmpty(pr.ReceiverName))
	return scanProof(row)
}

func (p Postgres) GetProofByKey(ctx context.Context, key string) (domain.Proof, error) {
	row := p.Pool.QueryRow(ctx, `
		SELECT id::text, stop_id::text, proof_type, object_key, mime_type, COALESCE(sha256,''), captured_at, uploaded_at, created_by, idempotency_key, pending, COALESCE(receiver_name,'')
		FROM proofs WHERE idempotency_key = $1`, key)
	return scanProof(row)
}

func (p Postgres) FinalizeProof(ctx context.Context, id, objectKey string) error {
	_, err := p.Pool.Exec(ctx, `UPDATE proofs SET pending = false, object_key = $2, uploaded_at = now() WHERE id::text = $1`, id, objectKey)
	return err
}

func (p Postgres) CountReadyProofs(ctx context.Context, stopID string) (int, error) {
	var n int
	err := p.Pool.QueryRow(ctx, `SELECT count(*) FROM proofs WHERE stop_id::text = $1 AND pending = false`, stopID).Scan(&n)
	return n, err
}

func (p Postgres) ExpiredProofIDs(ctx context.Context, cutoff time.Time, limit int) ([]string, error) {
	rows, err := p.Pool.Query(ctx, `
		SELECT p.id::text
		FROM delivery.proofs p
		JOIN delivery.stops s ON s.id=p.stop_id
		JOIN delivery.runs r ON r.id=s.run_id
		WHERE p.pending=FALSE AND p.retention_hold=FALSE AND p.object_key<>''
		  AND p.uploaded_at IS NOT NULL AND r.completed_at IS NOT NULL AND r.completed_at<=$1
		ORDER BY r.completed_at,p.id
		LIMIT $2`, cutoff, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]string, 0, limit)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// EraseExpiredProof holds the proof row lock while deleting the object. A
// concurrent legal hold therefore cannot be added between eligibility check
// and blob deletion. If storage deletion succeeds but the SQL transaction
// fails, a later sweep safely retries the idempotent object deletion.
func (p Postgres) EraseExpiredProof(ctx context.Context, id string, cutoff, erasedAt time.Time, retentionDays int, policyVersion string, deleteObject func(context.Context, string) error) (bool, error) {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var stopID, proofType, objectKey string
	var completedAt time.Time
	err = tx.QueryRow(ctx, `
		SELECT p.stop_id::text,p.proof_type,p.object_key,r.completed_at
		FROM delivery.proofs p
		JOIN delivery.stops s ON s.id=p.stop_id
		JOIN delivery.runs r ON r.id=s.run_id
		WHERE p.id=$1::uuid AND p.pending=FALSE AND p.retention_hold=FALSE
		  AND p.uploaded_at IS NOT NULL AND r.completed_at IS NOT NULL AND r.completed_at<=$2
		FOR UPDATE OF p`, id, cutoff).Scan(&stopID, &proofType, &objectKey, &completedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if objectKey != "" {
		if deleteObject == nil {
			return false, fmt.Errorf("proof object deletion is required")
		}
		if err := deleteObject(ctx, objectKey); err != nil {
			return false, err
		}
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO delivery.proof_retention_events(proof_id,stop_id,proof_type,trip_completed_at,erased_at,retention_days,policy_version)
		VALUES($1::uuid,$2::uuid,$3,$4,$5,$6,$7)
		ON CONFLICT(proof_id) DO NOTHING`, id, stopID, proofType, completedAt, erasedAt, retentionDays, policyVersion); err != nil {
		return false, err
	}
	command, err := tx.Exec(ctx, `DELETE FROM delivery.proofs WHERE id=$1::uuid`, id)
	if err != nil {
		return false, err
	}
	if command.RowsAffected() != 1 {
		return false, fmt.Errorf("expired proof disappeared while locked")
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

const runSelect = `
SELECT id::text, trip_id, plan_id, plan_ref, delivery_date::text, vehicle_id, depot, trip_number,
		COALESCE(vehicle_type,''), COALESCE(vehicle_temperature_capability,''), status,
		COALESCE(started_by,''), started_at, COALESCE(completed_by,''), completed_at, version,plan_version
	FROM runs`

const runReturning = `
	id::text, trip_id, plan_id, plan_ref, delivery_date::text, vehicle_id, depot, trip_number,
	COALESCE(vehicle_type,''), COALESCE(vehicle_temperature_capability,''), status,
	COALESCE(started_by,''), started_at, COALESCE(completed_by,''), completed_at, version,plan_version`

const stopSelect = `
	SELECT id::text, run_id::text, allocation_id, order_id, COALESCE(order_ref,''), expected_units, unit_label, COALESCE(outlet_id,''), COALESCE(brand,''),
		COALESCE(outlet_name,''), COALESCE(district,''), COALESCE(dock_type,''), COALESCE(parking_constraint,''),
		stop_sequence, planned_arrival_at, COALESCE(temperature_requirement,''), chilled_temperature_min_c, chilled_temperature_max_c, COALESCE(planned_window_open,''), COALESCE(planned_window_close,''), COALESCE(access_instructions,''), access_instructions_updated_at,
		COALESCE(loading_status,''), loading_shortfall_summary, status,
		arrived_at, arrived_received_at, COALESCE(outcome_code,''), COALESCE(outcome_reason,''), COALESCE(outcome_note,''), delivered_units, shortfall_units,
		outcome_at, outcome_received_at, completed_at, version
	FROM stops`

type scanner interface{ Scan(dest ...any) error }

func scanRun(row scanner) (domain.Run, error) {
	var r domain.Run
	var started, completed *time.Time
	err := row.Scan(&r.ID, &r.TripID, &r.PlanID, &r.PlanRef, &r.DeliveryDate, &r.VehicleID, &r.Depot, &r.TripNumber,
		&r.VehicleType, &r.VehicleTemperatureCapability, &r.Status, &r.StartedBy, &started, &r.CompletedBy, &completed, &r.Version, &r.PlanVersion)
	if err == pgx.ErrNoRows {
		return r, fmt.Errorf("not found")
	}
	r.StartedAt = started
	r.CompletedAt = completed
	return r, err
}

func scanStop(row scanner) (domain.Stop, error) {
	var s domain.Stop
	var short []byte
	err := row.Scan(&s.ID, &s.RunID, &s.AllocationID, &s.OrderID, &s.OrderRef, &s.ExpectedUnits, &s.UnitLabel, &s.OutletID, &s.Brand,
		&s.OutletName, &s.District, &s.DockType, &s.ParkingConstraint, &s.StopSequence, &s.PlannedArrivalAt, &s.TemperatureRequirement, &s.ChilledTemperatureMinC, &s.ChilledTemperatureMaxC,
		&s.PlannedWindowOpen, &s.PlannedWindowClose, &s.AccessInstructions, &s.AccessInstructionsUpdatedAt, &s.LoadingStatus, &short, &s.Status,
		&s.ArrivedAt, &s.ArrivedReceivedAt, &s.OutcomeCode, &s.OutcomeReason, &s.OutcomeNote, &s.DeliveredUnits, &s.ShortfallUnits,
		&s.OutcomeAt, &s.OutcomeReceivedAt, &s.CompletedAt, &s.Version)
	if err == pgx.ErrNoRows {
		return s, fmt.Errorf("not found")
	}
	if err != nil {
		return s, err
	}
	_ = json.Unmarshal(short, &s.LoadingShortfallSummary)
	if s.LoadingShortfallSummary == nil {
		s.LoadingShortfallSummary = []any{}
	}
	return s, nil
}

func scanProof(row scanner) (domain.Proof, error) {
	var p domain.Proof
	err := row.Scan(&p.ID, &p.StopID, &p.ProofType, &p.ObjectKey, &p.MimeType, &p.SHA256, &p.CapturedAt, &p.UploadedAt, &p.CreatedBy, &p.IdempotencyKey, &p.Pending, &p.ReceiverName)
	if err == pgx.ErrNoRows {
		return p, fmt.Errorf("not found")
	}
	return p, err
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
