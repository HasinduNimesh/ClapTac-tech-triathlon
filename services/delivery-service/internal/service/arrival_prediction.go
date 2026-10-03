package service

import (
	"context"
	"fmt"
	"time"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/authorization"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/delivery-service/internal/domain"
)

func validArrivalRisk(risk string) bool {
	switch risk {
	case "On track", "Watch window", "Window at risk", "Window missed", "ETA passed":
		return true
	default:
		return false
	}
}

func (s Service) PublishArrivalPrediction(ctx context.Context, profile *authorization.Profile, tripID, stopID string, planVersion int, sourceAt *time.Time, eta time.Time, lower, upper *time.Time, risk string) (domain.ArrivalPrediction, error) {
	if profile == nil || !authorization.HasPermission(profile.Roles, authorization.PermDeliveryViewAll) {
		return domain.ArrivalPrediction{}, fmt.Errorf("forbidden: dispatcher required")
	}
	if eta.IsZero() || planVersion <= 0 || !validArrivalRisk(risk) ||
		(lower != nil && upper != nil && lower.After(*upper)) || (lower == nil) != (upper == nil) {
		return domain.ArrivalPrediction{}, fmt.Errorf("invalid: arrival prediction")
	}
	run, err := s.Repo.GetByTrip(ctx, tripID)
	if err != nil { return domain.ArrivalPrediction{}, fmt.Errorf("not found: trip") }
	if run.Status == domain.RunCompleted || run.PlanVersion != planVersion {
		return domain.ArrivalPrediction{}, fmt.Errorf("conflict: trip or plan version changed")
	}
	trip, err := s.Peers.ReadyTrip(ctx, tripID)
	if err != nil || trip.PlanVersion != planVersion {
		return domain.ArrivalPrediction{}, fmt.Errorf("conflict: current plan unavailable")
	}
	stops, err := s.Repo.ListStops(ctx, run.ID)
	if err != nil { return domain.ArrivalPrediction{}, err }
	var stop *domain.Stop
	for i := range stops {
		if stops[i].ID == stopID { stop = &stops[i]; break }
	}
	if stop == nil { return domain.ArrivalPrediction{}, fmt.Errorf("not found: stop") }
	if stop.Status != domain.StopPending { return domain.ArrivalPrediction{}, fmt.Errorf("conflict: stop already reported") }
	prediction, pending, err := s.Repo.SaveArrivalPrediction(ctx, stopID, planVersion, sourceAt, eta, lower, upper, risk)
	if err != nil { return domain.ArrivalPrediction{}, err }
	prediction.StopID = stopID
	if pending != nil {
		status, queueErr := s.Peers.QueueArrivalChange(ctx, pending.EventKey, stop.OutletID, stop.OrderRef, pending.OldETA, pending.NewETA)
		if queueErr == nil && status == "suppressed" {
			_ = s.Repo.MarkArrivalNoticeSuppressed(ctx, stopID, pending.EventKey)
			prediction.PreviouslyCommunicatedAt = nil
			prediction.NotifiedArrivalAt = nil
		} else if queueErr == nil && (status == "enqueued" || status == "duplicate") {
			if err := s.Repo.MarkArrivalNoticeQueued(ctx, stopID, pending.EventKey); err == nil {
				prediction.PreviouslyCommunicatedAt = &pending.OldETA
				prediction.NotifiedArrivalAt = &pending.NewETA
			}
		} else if s.Peers.Logger != nil {
			s.Peers.Logger.Error("arrival_notification_enqueue_failed", "stop_id", stopID, "status", status, "error", queueErr)
		}
	}
	return prediction, nil
}
