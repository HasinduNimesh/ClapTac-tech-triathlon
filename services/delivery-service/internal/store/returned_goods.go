package store

import (
    "context"
    "fmt"
    "time"

    "github.com/jackc/pgx/v5"
    "github.com/HasinduNimesh/ClapTac-tech-triathlon/services/delivery-service/internal/domain"
)

func (p Postgres) MarkRefusedOutcome(ctx context.Context, run domain.Run, stop domain.Stop, opID, driverID, reason, note string, details domain.ReturnDetails, occurredAt time.Time) error {
    tx, err := p.Pool.Begin(ctx)
    if err != nil { return err }
    defer func() { _ = tx.Rollback(ctx) }()
    tag, err := tx.Exec(ctx, `UPDATE delivery.stops SET status=$2,outcome_code='REFUSED',outcome_reason=$3,outcome_note=$4,
        outcome_at=$5,outcome_received_at=now(),completed_at=now(),updated_at=now(),version=version+1
        WHERE id=$1::uuid AND run_id=$6::uuid AND status=$7`,
        stop.ID, domain.StopCompleted, reason, note, occurredAt, run.ID, domain.StopArrived)
    if err != nil { return err }
    if tag.RowsAffected() != 1 { return fmt.Errorf("conflict: stop cannot record outcome") }
    _, err = tx.Exec(ctx, `INSERT INTO delivery.returned_goods
        (stop_id,run_id,order_id,order_ref,outlet_id,operation_id,goods,units,reason,note,resolution,driver_id,occurred_at)
        VALUES($1::uuid,$2::uuid,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
        stop.ID, run.ID, stop.OrderID, stop.OrderRef, stop.OutletID, opID,
        details.Goods, details.Units, reason, note, details.Resolution, driverID, occurredAt)
    if err != nil { return err }
    return tx.Commit(ctx)
}

const returnedGoodsSelect = `SELECT stop_id::text,run_id::text,order_id,order_ref,outlet_id,operation_id,goods,units,
    reason,note,resolution,driver_id,occurred_at,recorded_at,followup_order_id,followup_order_ref,
    COALESCE(followup_date::text,'') FROM delivery.returned_goods WHERE stop_id=$1::uuid`

func (p Postgres) ReturnedGoods(ctx context.Context, stopID string) (*domain.ReturnedGoods, error) {
    var item domain.ReturnedGoods
    err := p.Pool.QueryRow(ctx, returnedGoodsSelect, stopID).Scan(
        &item.StopID,&item.RunID,&item.OrderID,&item.OrderRef,&item.OutletID,&item.OperationID,
        &item.Goods,&item.Units,&item.Reason,&item.Note,&item.Resolution,&item.DriverID,
        &item.OccurredAt,&item.RecordedAt,&item.FollowupOrderID,&item.FollowupOrderRef,&item.FollowupDate)
    if err == pgx.ErrNoRows { return nil, nil }
    if err != nil { return nil, err }
    return &item, nil
}

func (p Postgres) LinkReturnFollowup(ctx context.Context, stopID, orderID, orderRef, date string) error {
    _, err := p.Pool.Exec(ctx, `UPDATE delivery.returned_goods SET followup_order_id=$2,followup_order_ref=$3,followup_date=$4::date
        WHERE stop_id=$1::uuid AND (followup_order_id='' OR followup_order_id=$2)`,
        stopID, orderID, orderRef, date)
    return err
}

// A row lock and dispatcher_notified_at make the existing trip-message alert
// exactly once, including when an offline operation is replayed.
func (p Postgres) EnsureReturnDispatcherMessage(ctx context.Context, stopID string) error {
    tx, err := p.Pool.Begin(ctx)
    if err != nil { return err }
    defer func() { _ = tx.Rollback(ctx) }()
    var runID, orderRef, goods, resolution, followupRef string
    var units int
    var notified *time.Time
    err = tx.QueryRow(ctx, `SELECT run_id::text,order_ref,goods,units,resolution,followup_order_ref,dispatcher_notified_at
        FROM delivery.returned_goods WHERE stop_id=$1::uuid FOR UPDATE`, stopID).
        Scan(&runID,&orderRef,&goods,&units,&resolution,&followupRef,&notified)
    if err != nil { return err }
    if notified != nil { return tx.Commit(ctx) }
    action := "Re-attempt on next run"
    if resolution == "REQUEST_DEFERRAL" { action = "Dispatcher deferral requested; use planning deferral on follow-up order" }
    body := fmt.Sprintf("Rejected delivery %s: %d unit(s) of %s returned. %s. Follow-up %s.", orderRef, units, goods, action, followupRef)
    if len(body) > 1000 { body = body[:1000] }
    var messageID string
    err = tx.QueryRow(ctx, `INSERT INTO delivery.trip_messages(run_id,stop_id,body,sent_by,acknowledged_by,acknowledged_at)
        VALUES($1::uuid,$2::uuid,$3,'system:returned-goods','system:returned-goods',now()) RETURNING id::text`,
        runID,stopID,body).Scan(&messageID)
    if err != nil { return err }
    _, err = tx.Exec(ctx, `INSERT INTO delivery.trip_message_events(message_id,event_type,actor_id)
        VALUES($1::uuid,'SENT','system:returned-goods')`,messageID)
    if err != nil { return err }
    _, err = tx.Exec(ctx, `UPDATE delivery.returned_goods SET dispatcher_notified_at=now() WHERE stop_id=$1::uuid`,stopID)
    if err != nil { return err }
    return tx.Commit(ctx)
}


func (p Postgres) ReturnTripDate(ctx context.Context, runID string) (string, error) {
    var date string
    err := p.Pool.QueryRow(ctx, `SELECT delivery_date::text FROM delivery.runs WHERE id=$1::uuid`, runID).Scan(&date)
    return date, err
}
