package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/delivery-service/internal/domain"
)

const arrivalChangeThreshold = 30 * time.Minute

type PendingArrivalNotice struct {
	EventKey string
	OldETA time.Time
	NewETA time.Time
}

func arrivalChange(oldETA, newETA time.Time) bool {
	delta := newETA.Sub(oldETA)
	return delta >= arrivalChangeThreshold || delta <= -arrivalChangeThreshold
}

func (p Postgres) SaveArrivalPrediction(ctx context.Context, stopID string, planVersion int, sourceAt *time.Time, eta time.Time, lower, upper *time.Time, risk string) (domain.ArrivalPrediction, *PendingArrivalNotice, error) {
	tx, err := p.Pool.Begin(ctx)
	if err != nil { return domain.ArrivalPrediction{}, nil, err }
	defer func() { _ = tx.Rollback(ctx) }()
	var planned *time.Time
	err = tx.QueryRow(ctx, `SELECT planned_arrival_at FROM delivery.stops WHERE id=$1::uuid`, stopID).Scan(&planned)
	if err != nil { return domain.ArrivalPrediction{}, nil, err }
	baseline := eta
	if planned != nil { baseline = *planned }
	_, err = tx.Exec(ctx, `INSERT INTO delivery.arrival_predictions(stop_id,plan_version,source_event_at,current_eta,communicated_eta,range_lower,range_upper,risk)
		VALUES($1::uuid,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(stop_id) DO NOTHING`,
		stopID, planVersion, sourceAt, baseline, baseline, lower, upper, risk)
	if err != nil { return domain.ArrivalPrediction{}, nil, err }
	var storedVersion, noticeSequence int
	var storedSource *time.Time
	var communicated time.Time
	var pendingKey string
	var pendingOld, pendingNew *time.Time
	err = tx.QueryRow(ctx, `SELECT plan_version,notice_sequence,source_event_at,communicated_eta,pending_event_key,pending_old_eta,pending_new_eta
		FROM delivery.arrival_predictions WHERE stop_id=$1::uuid FOR UPDATE`, stopID).Scan(
		&storedVersion, &noticeSequence, &storedSource, &communicated, &pendingKey, &pendingOld, &pendingNew)
	if err != nil { return domain.ArrivalPrediction{}, nil, err }
	if storedVersion != planVersion { return domain.ArrivalPrediction{}, nil, fmt.Errorf("conflict: plan version changed") }
	if storedSource != nil && sourceAt != nil && sourceAt.Before(*storedSource) {
		prediction, err := scanArrivalPrediction(tx.QueryRow(ctx, arrivalPredictionSelect, stopID))
		if err != nil { return domain.ArrivalPrediction{}, nil, err }
		if err := tx.Commit(ctx); err != nil { return domain.ArrivalPrediction{}, nil, err }
		return prediction, nil, nil
	}
	if pendingKey == "" && arrivalChange(communicated, eta) {
		pendingKey = fmt.Sprintf("arrival:%s:%d", stopID, noticeSequence+1)
		pendingOld, pendingNew = &communicated, &eta
		_, err = tx.Exec(ctx, `UPDATE delivery.arrival_predictions SET current_eta=$2,communicated_eta=$2,
			previous_notified_eta=$3,notified_at=NULL,range_lower=$4,range_upper=$5,risk=$6,
			pending_event_key=$7,pending_old_eta=$3,pending_new_eta=$2,source_event_at=COALESCE($8,source_event_at),notice_sequence=notice_sequence+1,updated_at=now()
			WHERE stop_id=$1::uuid`, stopID, eta, communicated, lower, upper, risk, pendingKey, sourceAt)
	} else {
		_, err = tx.Exec(ctx, `UPDATE delivery.arrival_predictions SET current_eta=$2,range_lower=$3,range_upper=$4,
			risk=$5,source_event_at=COALESCE($6,source_event_at),updated_at=now() WHERE stop_id=$1::uuid`, stopID, eta, lower, upper, risk, sourceAt)
	}
	if err != nil { return domain.ArrivalPrediction{}, nil, err }
	prediction, err := scanArrivalPrediction(tx.QueryRow(ctx, arrivalPredictionSelect, stopID))
	if err != nil { return domain.ArrivalPrediction{}, nil, err }
	if err := tx.Commit(ctx); err != nil { return domain.ArrivalPrediction{}, nil, err }
	var pending *PendingArrivalNotice
	if pendingKey != "" && pendingOld != nil && pendingNew != nil {
		pending = &PendingArrivalNotice{EventKey: pendingKey, OldETA: *pendingOld, NewETA: *pendingNew}
	}
	return prediction, pending, nil
}

const arrivalPredictionSelect = `SELECT current_eta,previous_notified_eta,
	CASE WHEN notified_at IS NULL THEN NULL ELSE communicated_eta END,
	range_lower,range_upper,risk,updated_at FROM delivery.arrival_predictions WHERE stop_id=$1::uuid`

func scanArrivalPrediction(row pgx.Row) (domain.ArrivalPrediction, error) {
	var p domain.ArrivalPrediction
	err := row.Scan(&p.EstimatedArrivalAt, &p.PreviouslyCommunicatedAt, &p.NotifiedArrivalAt,
		&p.ArrivalRangeLower, &p.ArrivalRangeUpper, &p.LateRisk, &p.UpdatedAt)
	return p, err
}

func (p Postgres) ArrivalPrediction(ctx context.Context, stopID string) (*domain.ArrivalPrediction, error) {
	prediction, err := scanArrivalPrediction(p.Pool.QueryRow(ctx, arrivalPredictionSelect, stopID))
	if err == pgx.ErrNoRows { return nil, nil }
	if err != nil { return nil, err }
	prediction.StopID = stopID
	return &prediction, nil
}

func (p Postgres) MarkArrivalNoticeQueued(ctx context.Context, stopID, eventKey string) error {
	_, err := p.Pool.Exec(ctx, `UPDATE delivery.arrival_predictions SET
		pending_event_key='',pending_old_eta=NULL,pending_new_eta=NULL,notified_at=now()
		WHERE stop_id=$1::uuid AND pending_event_key=$2`, stopID, eventKey)
	return err
}


func (p Postgres) MarkArrivalNoticeSuppressed(ctx context.Context, stopID, eventKey string) error {
	_, err := p.Pool.Exec(ctx, `UPDATE delivery.arrival_predictions SET
		communicated_eta=COALESCE(pending_old_eta,communicated_eta),
		pending_event_key='',pending_old_eta=NULL,pending_new_eta=NULL,previous_notified_eta=NULL,notified_at=NULL
		WHERE stop_id=$1::uuid AND pending_event_key=$2`, stopID, eventKey)
	return err
}
