package store

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/order-service/internal/domain"
)

type Postgres struct {
	Pool *pgxpool.Pool
}

func (p Postgres) Create(order domain.Order) (domain.Order, error) {
	ctx := context.Background()
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return domain.Order{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	row := tx.QueryRow(ctx, `
		INSERT INTO orders (
			order_ref, outlet_id, brand, requested_delivery_date,
			order_units, order_weight_kg, order_volume_m3,
			temperature_requirement, status, created_by, source_system, external_order_id
		) VALUES (
			'ORD' || lpad(nextval('orders.order_ref_seq')::text, 6, '0'),
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NULLIF($11,'')
		)
		RETURNING id::text, order_ref, outlet_id, brand, requested_delivery_date::text,
			order_units, order_weight_kg, order_volume_m3, temperature_requirement,
			status, created_by, created_at, source_system, COALESCE(external_order_id,'')
	`, order.OutletID, order.Brand, order.RequestedDeliveryDate, order.OrderUnits,
		order.OrderWeightKg, order.OrderVolumeM3, order.TemperatureRequirement,
		domain.StatusConfirmed, order.CreatedBy, order.SourceSystem, order.ExternalOrderID)
	created, err := scanOrder(row)
	if err != nil {
		return domain.Order{}, err
	}
	for _, l := range order.Lines {
		if _, err = tx.Exec(ctx, `
			INSERT INTO orders.order_lines (order_id, line_no, product_id, product_name, pack_name, units_per_pack, pack_qty, weight_kg, volume_m3, source)
			VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
			created.ID, l.LineNo, l.ProductID, l.ProductName, l.Pack, l.UnitsPerPack, l.PackQty, l.WeightKg, l.VolumeM3, l.Source); err != nil {
			return domain.Order{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.Order{}, err
	}
	created.Lines = order.Lines
	return created, nil
}

// RecentLines returns the item lines of an outlet's orders of one goods type delivered on or after since,
// grouped per order.
func (p Postgres) RecentLines(outletID, temperature, since string) ([]domain.HistoricLines, error) {
	rows, err := p.Pool.Query(context.Background(), `
		SELECT o.id::text, o.requested_delivery_date::text, l.product_id, l.pack_qty
		FROM orders o JOIN orders.order_lines l ON l.order_id = o.id
		WHERE o.outlet_id = $1 AND o.temperature_requirement = $2 AND o.requested_delivery_date >= $3::date
		ORDER BY o.requested_delivery_date, o.id, l.line_no`, outletID, temperature, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.HistoricLines{}
	for rows.Next() {
		var id, date, product string
		var packs int
		if err := rows.Scan(&id, &date, &product, &packs); err != nil {
			return nil, err
		}
		if n := len(out); n == 0 || out[n-1].OrderID != id {
			out = append(out, domain.HistoricLines{OrderID: id, DeliveryDate: date, Packs: map[string]int{}})
		}
		out[len(out)-1].Packs[product] += packs
	}
	return out, rows.Err()
}

// attachLines loads the item lines for the given orders in one query. Orders without lines are left as they are.
func (p Postgres) attachLines(orders []domain.Order) error {
	if len(orders) == 0 {
		return nil
	}
	ids := make([]string, len(orders))
	index := make(map[string]int, len(orders))
	for i, o := range orders {
		ids[i] = o.ID
		index[o.ID] = i
	}
	rows, err := p.Pool.Query(context.Background(), `
		SELECT order_id::text, line_no, product_id, product_name, pack_name, units_per_pack, pack_qty, weight_kg::float8, volume_m3::float8, source
		FROM orders.order_lines WHERE order_id = ANY($1::uuid[]) ORDER BY order_id, line_no`, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var orderID string
		var l domain.OrderLine
		if err := rows.Scan(&orderID, &l.LineNo, &l.ProductID, &l.ProductName, &l.Pack, &l.UnitsPerPack, &l.PackQty, &l.WeightKg, &l.VolumeM3, &l.Source); err != nil {
			return err
		}
		if i, ok := index[orderID]; ok {
			orders[i].Lines = append(orders[i].Lines, l)
		}
	}
	return rows.Err()
}

func (p Postgres) Get(id string) (domain.Order, error) {
	row := p.Pool.QueryRow(context.Background(), `
		SELECT id::text, order_ref, outlet_id, brand, requested_delivery_date::text,
			order_units, order_weight_kg, order_volume_m3, temperature_requirement,
			status, created_by, created_at, source_system, COALESCE(external_order_id,'')
		FROM orders
		WHERE id::text = $1 OR order_ref = $1
	`, id)
	o, err := scanOrder(row)
	if err != nil {
		return o, err
	}
	one := []domain.Order{o}
	if err := p.attachLines(one); err != nil {
		return domain.Order{}, err
	}
	return one[0], nil
}

func (p Postgres) GetImported(source, externalID string) (domain.Order, error) {
    row := p.Pool.QueryRow(context.Background(), `SELECT id::text,order_ref,outlet_id,brand,requested_delivery_date::text,
        order_units,order_weight_kg,order_volume_m3,temperature_requirement,status,created_by,created_at,
        source_system,COALESCE(external_order_id,'') FROM orders.orders
        WHERE source_system=$1 AND external_order_id=$2`, source, externalID)
    return scanOrder(row)
}

func (p Postgres) List(filter domain.ListFilter) ([]domain.Order, error) {
	q := `
		SELECT id::text, order_ref, outlet_id, brand, requested_delivery_date::text,
			order_units, order_weight_kg, order_volume_m3, temperature_requirement,
			status, created_by, created_at, source_system, COALESCE(external_order_id,'')
		FROM orders WHERE 1=1
	`
	args := []any{}
	n := 1
	if filter.OutletID != "" {
		q += fmt.Sprintf(" AND outlet_id = $%d", n)
		args = append(args, filter.OutletID)
		n++
	}
	if filter.Status != "" {
		q += fmt.Sprintf(" AND status = $%d", n)
		args = append(args, filter.Status)
		n++
	}
	if filter.Brand != "" {
		q += fmt.Sprintf(" AND brand = $%d", n)
		args = append(args, filter.Brand)
		n++
	}
	if filter.RequestedDeliveryDate != "" {
		q += fmt.Sprintf(" AND requested_delivery_date = $%d", n)
		args = append(args, filter.RequestedDeliveryDate)
	}
	q += " ORDER BY created_at DESC"
	rows, err := p.Pool.Query(context.Background(), q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Order
	for rows.Next() {
		o, err := scanOrder(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if out == nil {
		out = []domain.Order{}
	}
	rows.Close()
	if err := p.attachLines(out); err != nil {
		return nil, err
	}
	return out, nil
}

func (p Postgres) ImportOrders(source string, orders []domain.Order) ([]domain.ImportResult, error) {
	ctx := context.Background()
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	results := make([]domain.ImportResult, 0, len(orders))
	for _, o := range orders {
		selectSQL := `id::text,order_ref,outlet_id,brand,requested_delivery_date::text,order_units,order_weight_kg,order_volume_m3,temperature_requirement,status,created_by,created_at,source_system,COALESCE(external_order_id,'')`
		row := tx.QueryRow(ctx, `INSERT INTO orders.orders(order_ref,outlet_id,brand,requested_delivery_date,order_units,order_weight_kg,order_volume_m3,temperature_requirement,status,created_by,source_system,external_order_id)
			VALUES('ORD'||lpad(nextval('orders.order_ref_seq')::text,6,'0'),$1,$2,$3,$4,$5,$6,$7,'confirmed','integration-import',$8,$9)
			ON CONFLICT(source_system,external_order_id) WHERE external_order_id IS NOT NULL DO NOTHING RETURNING `+selectSQL,
			o.OutletID, o.Brand, o.RequestedDeliveryDate, o.OrderUnits, o.OrderWeightKg, o.OrderVolumeM3, o.TemperatureRequirement, source, o.ExternalOrderID)
		created, scanErr := scanOrder(row)
		if scanErr == nil {
			results = append(results, domain.ImportResult{Order: created, Created: true})
			continue
		}
		if scanErr.Error() != "not found" {
			return nil, scanErr
		}
		existing, err := scanOrder(tx.QueryRow(ctx, `SELECT `+selectSQL+` FROM orders.orders WHERE source_system=$1 AND external_order_id=$2 FOR UPDATE`, source, o.ExternalOrderID))
		if err != nil {
			return nil, err
		}
		if !sameImportedOrder(existing, o) {
			return nil, fmt.Errorf("conflict: external order %s already exists with different data", o.ExternalOrderID)
		}
		results = append(results, domain.ImportResult{Order: existing, Created: false})
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return results, nil
}

func sameImportedOrder(a, b domain.Order) bool {
	return a.OutletID == b.OutletID && a.Brand == b.Brand && a.RequestedDeliveryDate == b.RequestedDeliveryDate && a.OrderUnits == b.OrderUnits && a.OrderWeightKg == b.OrderWeightKg && a.OrderVolumeM3 == b.OrderVolumeM3 && a.TemperatureRequirement == b.TemperatureRequirement
}

func (p Postgres) Forecast(now time.Time) (domain.Forecast, error) {
	ctx := context.Background()
	loc, locErr := time.LoadLocation("Asia/Colombo")
	if locErr != nil {
		loc = time.FixedZone("Asia/Colombo", 5*60*60+30*60)
	}
	start := now.In(loc)
	// Use complete prior calendar weeks only, avoiding partial-week bias.
	weekStart := start.AddDate(0, 0, -int((int(start.Weekday())+6)%7))
	historyStart := weekStart.AddDate(0, 0, -7*forecastHistoryWeeks)
	out := domain.Forecast{GeneratedAt: now.UTC(), ForecastVersion: forecastVersion, Method: forecastMethod, HistoryWeeks: forecastHistoryWeeks, HorizonWeeks: ForecastHorizonWeeks, DriftModelVersion: forecastDriftVersion, BacktestModelVersion: forecastBacktestVersion, InputDrift: []domain.ForecastInputDrift{}, Weekly: []domain.ForecastBucket{}, Capacity: []domain.ForecastCapacity{}}
	rows, err := p.Pool.Query(ctx, `
		SELECT COALESCE(NULLIF(o.depot,''),'UNASSIGNED'), x.brand,
			COUNT(*) FILTER (WHERE x.temperature_requirement='chilled')::int,
			COUNT(*) FILTER (WHERE x.temperature_requirement='ambient')::int,
			COALESCE(SUM(x.order_weight_kg),0), COALESCE(SUM(x.order_volume_m3),0)
		FROM orders.orders x LEFT JOIN shared.outlets o ON o.id=x.outlet_id
		WHERE x.status='confirmed' AND x.requested_delivery_date >= $1 AND x.requested_delivery_date < $2
		GROUP BY 1,2 ORDER BY 1,2`, historyStart.Format("2006-01-02"), weekStart.Format("2006-01-02"))
	if err != nil {
		return out, err
	}
	groups := []forecastGroup{}
	for rows.Next() {
		var g forecastGroup
		if err := rows.Scan(&g.depot, &g.brand, &g.chilled, &g.ambient, &g.weight, &g.volume); err != nil {
			rows.Close()
			return out, err
		}
		groups = append(groups, g)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return out, err
	}
	rows.Close()
	driftStart := historyStart.AddDate(0, 0, -7*forecastHistoryWeeks)
	driftRows, err := p.Pool.Query(ctx, `
		SELECT COALESCE(NULLIF(o.depot,''),'UNASSIGNED'), x.brand,
			COUNT(*) FILTER (WHERE x.requested_delivery_date >= $1 AND x.requested_delivery_date < $2)::int,
			COUNT(*) FILTER (WHERE x.requested_delivery_date >= $2 AND x.requested_delivery_date < $3)::int
		FROM orders.orders x LEFT JOIN shared.outlets o ON o.id=x.outlet_id
		WHERE x.status='confirmed' AND x.requested_delivery_date >= $1 AND x.requested_delivery_date < $3
		GROUP BY 1,2 ORDER BY 1,2`, driftStart.Format("2006-01-02"), historyStart.Format("2006-01-02"), weekStart.Format("2006-01-02"))
	if err != nil {
		return out, err
	}
	for driftRows.Next() {
		var d domain.ForecastInputDrift
		if err := driftRows.Scan(&d.Depot, &d.Brand, &d.PreviousOrderCount, &d.RecentOrderCount); err != nil {
			driftRows.Close()
			return out, err
		}
		d.Status, d.ChangePercent = classifyForecastDrift(d.PreviousOrderCount, d.RecentOrderCount)
		d.BacktestAPEPercent = forecastAPE(d.PreviousOrderCount, d.RecentOrderCount)
		out.InputDrift = append(out.InputDrift, d)
	}
	if err := driftRows.Err(); err != nil {
		driftRows.Close()
		return out, err
	}
	driftRows.Close()
	// SQL above groups full-window history; projectWeekly divides by the history
	// weeks for the stable weekly baseline and repeats it over the whole horizon.
	out.Weekly = projectWeekly(weekStart, groups)
	var minutes int
	if err := p.Pool.QueryRow(ctx, `SELECT COALESCE(ROUND(AVG(minutes)),0)::int FROM shared.service_allowance WHERE minutes>0`).Scan(&minutes); err != nil {
		return out, err
	}
	if minutes > 0 {
		out.ServiceMinutesPerStop = minutes
		out.ServiceEstimateVersion = "configured_allowance_mean_v1"
		out.ServiceEstimateSource = "mean of configured service allowances"
	} else {
		out.ServiceMinutesPerStop = 20
		out.ServiceEstimateVersion = "fixed_20m_v1"
		out.ServiceEstimateSource = "deterministic 20-minute fallback; no configured allowances"
	}
	out.ServiceTimeBacktestVersion = serviceTimeBacktestVersion
	out.ServiceTimeBacktestWindowStart = historyStart.Format("2006-01-02")
	out.ServiceTimeBacktestWindowEnd = weekStart.Format("2006-01-02")
	out.ServiceTimeEvaluation = []domain.ServiceTimeEvaluation{}
	serviceRows, err := p.Pool.Query(ctx, `
		SELECT COALESCE(NULLIF(r.depot,''),'UNASSIGNED'), COALESCE(NULLIF(s.brand,''),'UNKNOWN'),
			COUNT(*)::int,
			ROUND(AVG(EXTRACT(EPOCH FROM (s.outcome_at-s.arrived_at))/60.0)::numeric,1)::float8,
			ROUND(AVG(ABS(EXTRACT(EPOCH FROM (s.outcome_at-s.arrived_at))/60.0-$1))::numeric,1)::float8
		FROM delivery.stops s JOIN delivery.runs r ON r.id=s.run_id
		WHERE s.outcome_code IN ('DELIVERED','PARTIAL')
			AND s.arrived_at IS NOT NULL AND s.outcome_at IS NOT NULL
			AND s.outcome_at >= $2 AND s.outcome_at < $3
			AND EXTRACT(EPOCH FROM (s.outcome_at-s.arrived_at)) BETWEEN 0 AND 14400
		GROUP BY 1,2 ORDER BY 1,2`, float64(out.ServiceMinutesPerStop), historyStart, weekStart)
	if err != nil {
		return out, err
	}
	for serviceRows.Next() {
		var evaluation domain.ServiceTimeEvaluation
		var observed, absoluteError float64
		if err := serviceRows.Scan(&evaluation.Depot, &evaluation.Brand, &evaluation.ActualStopCount, &observed, &absoluteError); err != nil {
			serviceRows.Close()
			return out, err
		}
		evaluation.ConfiguredMinutes = out.ServiceMinutesPerStop
		evaluation.Status = "INSUFFICIENT_HISTORY"
		if evaluation.ActualStopCount >= serviceTimeEvaluationMinStops {
			evaluation.Status = "EVALUATED"
			evaluation.MeanObservedMinutes = &observed
			evaluation.MeanAbsoluteErrorMinutes = &absoluteError
		}
		out.ServiceTimeEvaluation = append(out.ServiceTimeEvaluation, evaluation)
	}
	if err := serviceRows.Err(); err != nil {
		serviceRows.Close()
		return out, err
	}
	serviceRows.Close()
	var trips int
	_ = p.Pool.QueryRow(ctx, `SELECT v.max_trips_per_vehicle FROM shared.planning_policy_current c JOIN shared.planning_policy_versions v ON v.version=c.version WHERE c.singleton=true`).Scan(&trips)
	if trips < 1 {
		trips = 1
	}
	capRows, err := p.Pool.Query(ctx, `SELECT COALESCE(NULLIF(home_depot,''),'UNASSIGNED'),SUM(weight_cap_kg),SUM(volume_cap_m3) FROM fleet.vehicles GROUP BY 1 ORDER BY 1`)
	if err != nil {
		return out, err
	}
	defer capRows.Close()
	type capRow struct {
		depot          string
		weight, volume float64
	}
	caps := []capRow{}
	for capRows.Next() {
		var c capRow
		if err := capRows.Scan(&c.depot, &c.weight, &c.volume); err != nil {
			return out, err
		}
		caps = append(caps, c)
	}
	if err := capRows.Err(); err != nil {
		return out, err
	}
	for _, c := range caps {
		weekly := map[string][2]float64{}
		for _, b := range out.Weekly {
			if b.Depot == c.depot {
				v := weekly[b.WeekStarting]
				v[0] += b.EstimatedWeightKg
				v[1] += b.EstimatedVolumeM3
				weekly[b.WeekStarting] = v
			}
		}
		projectedW, projectedV := 0.0, 0.0
		for _, v := range weekly {
			projectedW = math.Max(projectedW, v[0])
			projectedV = math.Max(projectedV, v[1])
		}
		weeklyW := c.weight * float64(trips) * 5
		weeklyV := c.volume * float64(trips) * 5
		pressure := "low"
		ratio := math.Max(projectedW/math.Max(weeklyW, 1), projectedV/math.Max(weeklyV, 0.001))
		if ratio >= 0.85 {
			pressure = "high"
		} else if ratio >= 0.65 {
			pressure = "medium"
		}
		out.Capacity = append(out.Capacity, domain.ForecastCapacity{Depot: c.depot, ProjectedWeightKg: projectedW, ProjectedVolumeM3: projectedV, EstimatedWeightCapacityKg: weeklyW, EstimatedVolumeCapacityM3: weeklyV, Pressure: pressure})
	}
	return out, nil
}

func (p Postgres) GetReceipt(orderID string) (domain.Receipt, []domain.ReceiptIssue, error) {
	row := p.Pool.QueryRow(context.Background(), receiptSelect+` WHERE order_id::text=$1`, orderID)
	r, err := scanReceipt(row)
	if err != nil {
		return domain.Receipt{}, nil, err
	}
	issues, err := p.listReceiptIssues(r.ID)
	return r, issues, err
}

func (p Postgres) AddCustodyEvent(order domain.Order, event domain.CustodyEvent) (domain.CustodyEvent, bool, error) {
	ctx := context.Background()
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return domain.CustodyEvent{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	event.OrderID = order.ID
	var previous domain.CustodyEvent
	err = tx.QueryRow(ctx, `SELECT id::text,order_id::text,stage,seal_id,serial_numbers,condition,evidence_ref,receiver_name,idempotency_key,recorded_by,recorded_at FROM orders.tech_custody_events WHERE idempotency_key=$1`, event.IdempotencyKey).Scan(&previous.ID, &previous.OrderID, &previous.Stage, &previous.SealID, &previous.SerialNumbers, &previous.Condition, &previous.EvidenceRef, &previous.ReceiverName, &previous.IdempotencyKey, &previous.RecordedBy, &previous.RecordedAt)
	if err == nil {
		if previous.OrderID != order.ID || previous.Stage != event.Stage || previous.SealID != event.SealID || previous.Condition != event.Condition || previous.EvidenceRef != event.EvidenceRef || previous.ReceiverName != event.ReceiverName || !sameStrings(previous.SerialNumbers, event.SerialNumbers) {
			return domain.CustodyEvent{}, false, fmt.Errorf("conflict: idempotency key already used")
		}
		_ = tx.Commit(ctx)
		return previous, false, nil
	}
	if err != pgx.ErrNoRows {
		return domain.CustodyEvent{}, false, err
	}
	if _, err = tx.Exec(ctx, `SELECT id FROM orders.orders WHERE id=$1::uuid FOR UPDATE`, order.ID); err != nil {
		return domain.CustodyEvent{}, false, err
	}
	err = tx.QueryRow(ctx, `SELECT id::text,order_id::text,stage,seal_id,serial_numbers,condition,evidence_ref,receiver_name,idempotency_key,recorded_by,recorded_at FROM orders.tech_custody_events WHERE order_id=$1::uuid ORDER BY recorded_at DESC,id DESC LIMIT 1`, order.ID).Scan(&previous.ID, &previous.OrderID, &previous.Stage, &previous.SealID, &previous.SerialNumbers, &previous.Condition, &previous.EvidenceRef, &previous.ReceiverName, &previous.IdempotencyKey, &previous.RecordedBy, &previous.RecordedAt)
	if err != nil && err != pgx.ErrNoRows {
		return domain.CustodyEvent{}, false, err
	}
	if err == nil && sameCustodyContent(previous, event) {
		_ = tx.Commit(ctx)
		return previous, false, nil
	}
	if (err == pgx.ErrNoRows && event.Stage != "LOADED") || (err == nil && !custodyTransitionAllowed(previous.Stage, event.Stage)) {
		return domain.CustodyEvent{}, false, fmt.Errorf("conflict: custody stage out of sequence")
	}
	if err == nil && (previous.SealID != event.SealID || !sameStrings(previous.SerialNumbers, event.SerialNumbers)) {
		return domain.CustodyEvent{}, false, fmt.Errorf("conflict: seal and serials must match the loaded custody record")
	}
	err = tx.QueryRow(ctx, `INSERT INTO orders.tech_custody_events(order_id,stage,seal_id,serial_numbers,condition,evidence_ref,receiver_name,idempotency_key,recorded_by) VALUES($1::uuid,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id::text,recorded_at`, order.ID, event.Stage, event.SealID, event.SerialNumbers, event.Condition, event.EvidenceRef, event.ReceiverName, event.IdempotencyKey, event.RecordedBy).Scan(&event.ID, &event.RecordedAt)
	if err != nil {
		return domain.CustodyEvent{}, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.CustodyEvent{}, false, err
	}
	return event, true, nil
}

func (p Postgres) ListCustodyEvents(orderID string) ([]domain.CustodyEvent, error) {
	rows, err := p.Pool.Query(context.Background(), `SELECT id::text,order_id::text,stage,seal_id,serial_numbers,condition,evidence_ref,receiver_name,idempotency_key,recorded_by,recorded_at FROM orders.tech_custody_events WHERE order_id::text=$1 ORDER BY recorded_at,id`, orderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.CustodyEvent{}
	for rows.Next() {
		var e domain.CustodyEvent
		if err := rows.Scan(&e.ID, &e.OrderID, &e.Stage, &e.SealID, &e.SerialNumbers, &e.Condition, &e.EvidenceRef, &e.ReceiverName, &e.IdempotencyKey, &e.RecordedBy, &e.RecordedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (p Postgres) ConfirmReceipt(order domain.Order, c domain.ReceiptConfirmation) (domain.Receipt, []domain.ReceiptIssue, bool, error) {
	ctx := context.Background()
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return domain.Receipt{}, nil, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	row := tx.QueryRow(ctx, receiptSelect+` WHERE order_id=$1::uuid FOR UPDATE`, order.ID)
	existing, err := scanReceipt(row)
	if err == nil {
		if existing.ReceivedUnits != c.ReceivedUnits {
			return domain.Receipt{}, nil, false, fmt.Errorf("conflict: receipt already confirmed with different quantity")
		}
		issues, e := listReceiptIssuesTx(ctx, tx, existing.ID)
		if e != nil {
			return domain.Receipt{}, nil, false, e
		}
		return existing, issues, false, nil
	}
	if err.Error() != "not found" {
		return domain.Receipt{}, nil, false, err
	}
	status := "confirmed"
	if c.Issue != nil {
		status = "confirmed_with_issue"
	}
	r := domain.Receipt{OrderID: order.ID, DeliveryRunID: c.DeliveryRunID, DeliveryStopID: c.DeliveryStopID, DeliveryOutcome: c.DeliveryOutcome, ExpectedUnits: c.ExpectedUnits, ReceivedUnits: c.ReceivedUnits, Status: status, ConfirmedBy: c.ConfirmedBy, Version: 1}
	r, err = scanReceipt(tx.QueryRow(ctx, `INSERT INTO receipts(order_id,delivery_run_id,delivery_stop_id,delivery_outcome,expected_units,received_units,status,confirmed_by,received_temperature_c) VALUES($1::uuid,$2::uuid,$3::uuid,$4,$5,$6,$7,$8,$9) RETURNING `+receiptColumns, r.OrderID, r.DeliveryRunID, r.DeliveryStopID, r.DeliveryOutcome, r.ExpectedUnits, r.ReceivedUnits, r.Status, r.ConfirmedBy, c.ReceivedTemperatureC))
	if err != nil {
		return domain.Receipt{}, nil, false, err
	}
	var issues []domain.ReceiptIssue
	if c.Issue != nil {
		issue, e := insertReceiptIssue(ctx, tx, r.ID, *c.Issue, c.ConfirmedBy)
		if e != nil {
			return domain.Receipt{}, nil, false, e
		}
		issues = append(issues, issue)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Receipt{}, nil, false, err
	}
	return r, issues, true, nil
}

func (p Postgres) AddReceiptIssue(order domain.Order, req domain.ReceiptIssueRequest, actor string) (domain.Receipt, domain.ReceiptIssue, bool, error) {
	ctx := context.Background()
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return domain.Receipt{}, domain.ReceiptIssue{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	r, err := scanReceipt(tx.QueryRow(ctx, receiptSelect+` WHERE order_id=$1::uuid FOR UPDATE`, order.ID))
	if err != nil {
		return domain.Receipt{}, domain.ReceiptIssue{}, false, err
	}
	var existing domain.ReceiptIssue
	err = tx.QueryRow(ctx, `SELECT id::text,receipt_id::text,issue_type,affected_units,note,idempotency_key,created_by,created_at FROM receipt_issues WHERE idempotency_key=$1`, req.IdempotencyKey).Scan(&existing.ID, &existing.ReceiptID, &existing.IssueType, &existing.AffectedUnits, &existing.Note, &existing.IdempotencyKey, &existing.CreatedBy, &existing.CreatedAt)
	if err == nil {
		if existing.ReceiptID != r.ID {
			return domain.Receipt{}, domain.ReceiptIssue{}, false, fmt.Errorf("conflict: idempotency key already used")
		}
		if existing.IssueType != req.IssueType || existing.AffectedUnits != req.AffectedUnits || existing.Note != req.Note {
			return domain.Receipt{}, domain.ReceiptIssue{}, false, fmt.Errorf("conflict: idempotency key reused with different issue")
		}
		_ = tx.Commit(ctx)
		return r, existing, false, nil
	}
	if err != pgx.ErrNoRows {
		return domain.Receipt{}, domain.ReceiptIssue{}, false, err
	}
	issue, err := insertReceiptIssue(ctx, tx, r.ID, req, actor)
	if err != nil {
		return domain.Receipt{}, domain.ReceiptIssue{}, false, err
	}
	_, err = tx.Exec(ctx, `UPDATE receipts SET status='confirmed_with_issue',updated_at=now(),version=version+1 WHERE id=$1::uuid`, r.ID)
	if err != nil {
		return domain.Receipt{}, domain.ReceiptIssue{}, false, err
	}
	r.Status = "confirmed_with_issue"
	r.Version++
	if err := tx.Commit(ctx); err != nil {
		return domain.Receipt{}, domain.ReceiptIssue{}, false, err
	}
	return r, issue, true, nil
}

func (p Postgres) ListReceiptIssues() ([]domain.ReceiptIssueView, error) {
	rows, err := p.Pool.Query(context.Background(), `SELECT o.order_ref,o.outlet_id,`+receiptColumnsQualified+`, i.id::text,i.receipt_id::text,i.issue_type,i.affected_units,i.note,i.idempotency_key,i.created_by,i.created_at FROM receipt_issues i JOIN receipts r ON r.id=i.receipt_id JOIN orders o ON o.id=r.order_id ORDER BY i.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.ReceiptIssueView{}
	for rows.Next() {
		var v domain.ReceiptIssueView
		if err := rows.Scan(&v.OrderRef, &v.OutletID, &v.Receipt.ID, &v.Receipt.OrderID, &v.Receipt.DeliveryRunID, &v.Receipt.DeliveryStopID, &v.Receipt.DeliveryOutcome, &v.Receipt.ExpectedUnits, &v.Receipt.ReceivedUnits, &v.Receipt.Status, &v.Receipt.ConfirmedBy, &v.Receipt.ConfirmedAt, &v.Receipt.Version, &v.Receipt.ReceivedTemperatureC, &v.Issue.ID, &v.Issue.ReceiptID, &v.Issue.IssueType, &v.Issue.AffectedUnits, &v.Issue.Note, &v.Issue.IdempotencyKey, &v.Issue.CreatedBy, &v.Issue.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (p Postgres) listReceiptIssues(receiptID string) ([]domain.ReceiptIssue, error) {
	return listReceiptIssuesQuery(context.Background(), p.Pool, receiptID)
}

const receiptColumns = `id::text,order_id::text,delivery_run_id::text,delivery_stop_id::text,delivery_outcome,expected_units,received_units,status,confirmed_by,confirmed_at,version,received_temperature_c::float8`
const receiptSelect = `SELECT ` + receiptColumns + ` FROM receipts`
const receiptColumnsQualified = `r.id::text,r.order_id::text,r.delivery_run_id::text,r.delivery_stop_id::text,r.delivery_outcome,r.expected_units,r.received_units,r.status,r.confirmed_by,r.confirmed_at,r.version,r.received_temperature_c::float8`

func scanReceipt(row scanner) (domain.Receipt, error) {
	var r domain.Receipt
	err := row.Scan(&r.ID, &r.OrderID, &r.DeliveryRunID, &r.DeliveryStopID, &r.DeliveryOutcome, &r.ExpectedUnits, &r.ReceivedUnits, &r.Status, &r.ConfirmedBy, &r.ConfirmedAt, &r.Version, &r.ReceivedTemperatureC)
	if err == pgx.ErrNoRows {
		return r, fmt.Errorf("not found")
	}
	return r, err
}

func listReceiptIssuesTx(ctx context.Context, tx pgx.Tx, receiptID string) ([]domain.ReceiptIssue, error) {
	return listReceiptIssuesQuery(ctx, tx, receiptID)
}

type receiptQuerier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func listReceiptIssuesQuery(ctx context.Context, q receiptQuerier, receiptID string) ([]domain.ReceiptIssue, error) {
	rows, err := q.Query(ctx, `SELECT id::text,receipt_id::text,issue_type,affected_units,note,idempotency_key,created_by,created_at FROM receipt_issues WHERE receipt_id=$1::uuid ORDER BY created_at`, receiptID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.ReceiptIssue{}
	for rows.Next() {
		var i domain.ReceiptIssue
		if err := rows.Scan(&i.ID, &i.ReceiptID, &i.IssueType, &i.AffectedUnits, &i.Note, &i.IdempotencyKey, &i.CreatedBy, &i.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

func insertReceiptIssue(ctx context.Context, tx pgx.Tx, receiptID string, req domain.ReceiptIssueRequest, actor string) (domain.ReceiptIssue, error) {
	var i domain.ReceiptIssue
	err := tx.QueryRow(ctx, `INSERT INTO receipt_issues(receipt_id,issue_type,affected_units,note,idempotency_key,created_by) VALUES($1::uuid,$2,$3,$4,$5,$6) RETURNING id::text,receipt_id::text,issue_type,affected_units,note,idempotency_key,created_by,created_at`, receiptID, req.IssueType, req.AffectedUnits, req.Note, req.IdempotencyKey, actor).Scan(&i.ID, &i.ReceiptID, &i.IssueType, &i.AffectedUnits, &i.Note, &i.IdempotencyKey, &i.CreatedBy, &i.CreatedAt)
	return i, err
}

type scanner interface {
	Scan(dest ...any) error
}

func scanOrder(row scanner) (domain.Order, error) {
	var o domain.Order
	err := row.Scan(&o.ID, &o.OrderRef, &o.OutletID, &o.Brand, &o.RequestedDeliveryDate,
		&o.OrderUnits, &o.OrderWeightKg, &o.OrderVolumeM3, &o.TemperatureRequirement,
		&o.Status, &o.CreatedBy, &o.CreatedAt, &o.SourceSystem, &o.ExternalOrderID)
	if err == pgx.ErrNoRows {
		return domain.Order{}, fmt.Errorf("not found")
	}
	return o, err
}
