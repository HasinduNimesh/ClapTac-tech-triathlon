package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/delivery-service/internal/domain"
)

// Arrival predictions are published by the backend, not by a dispatcher's browser: every driver event that
// can move the downstream arrival times of an in-progress run (a stop arrival or a stop outcome, whether
// it came online or through offline sync) recomputes the predicted arrival of each remaining stop with
// EstimateArrival and records it through recordArrivalPrediction, which applies the 30-minute rule and
// its idempotency and queues the ARRIVAL_CHANGE store notice. This works with nobody watching the
// dispatcher screen.
//
// Limits: only arrivals and outcomes trigger a refresh (not location pings; the estimate is event-based
// and ignores GPS anyway). The service-time allowance is Service.ServiceMinutesPerStop, a configured
// value rather than the order forecast the dispatcher page reads, because delivery-service has no feed of
// that forecast. The previous stop's planned departure comes from the loading-service trip when it
// supplies one and is otherwise the planned arrival plus the same allowance.

var predictionJobs sync.WaitGroup

// WaitForArrivalPredictions blocks until in-flight arrival-prediction refreshes finish. Servers use it on
// shutdown; tests use it to observe a refresh deterministically.
func (Service) WaitForArrivalPredictions() { predictionJobs.Wait() }

// refreshArrivalPredictionsAsync never blocks or fails the driver's operation: the work runs in the
// background, on its own bounded context, and any failure is logged and dropped.
func (s Service) refreshArrivalPredictionsAsync(tripID string) {
	predictionJobs.Add(1)
	go func() {
		defer predictionJobs.Done()
		defer func() {
			if r := recover(); r != nil {
				s.logPredictionError("arrival_prediction_refresh_panic", tripID, "", fmt.Errorf("%v", r))
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		s.refreshArrivalPredictions(ctx, tripID, time.Now())
	}()
}

func (s Service) logPredictionError(msg, tripID, stopID string, err error) {
	if s.Peers.Logger != nil {
		s.Peers.Logger.Error(msg, "trip_id", tripID, "stop_id", stopID, "error", err)
	}
}

func (s Service) serviceMinutes() int {
	if s.ServiceMinutesPerStop > 0 {
		return s.ServiceMinutesPerStop
	}
	return DefaultServiceMinutesPerStop
}

// refreshArrivalPredictions recomputes and records the predicted arrival of every pending stop that
// follows a reported one. Failures on one stop are logged and do not stop the others.
func (s Service) refreshArrivalPredictions(ctx context.Context, tripID string, now time.Time) {
	run, err := s.Repo.GetByTrip(ctx, tripID)
	if err != nil {
		s.logPredictionError("arrival_prediction_run_unavailable", tripID, "", err)
		return
	}
	if run.Status != domain.RunInProgress {
		return
	}
	trip, err := s.Peers.ReadyTrip(ctx, tripID)
	if err != nil {
		s.logPredictionError("arrival_prediction_plan_unavailable", tripID, "", err)
		return
	}
	if trip.PlanVersion != run.PlanVersion {
		s.logPredictionError("arrival_prediction_plan_version_changed", tripID, "", fmt.Errorf("run is on plan version %d, current is %d", run.PlanVersion, trip.PlanVersion))
		return
	}
	stops, err := s.Repo.ListStops(ctx, run.ID)
	if err != nil {
		s.logPredictionError("arrival_prediction_stops_unavailable", tripID, "", err)
		return
	}
	departures := map[string]*time.Time{}
	for _, order := range trip.Orders {
		if order.PlannedDepartureAt != nil {
			departures[order.AllocationID] = order.PlannedDepartureAt
		}
	}
	allowance := s.serviceMinutes()
	for i, stop := range stops {
		if stop.Status != domain.StopPending || stop.PlannedArrivalAt == nil {
			continue
		}
		previous := previousReportedStop(stops, i)
		if previous == nil {
			continue
		}
		plannedDeparture := departures[previous.AllocationID]
		if plannedDeparture == nil && previous.PlannedArrivalAt != nil {
			t := previous.PlannedArrivalAt.Add(time.Duration(allowance) * time.Minute)
			plannedDeparture = &t
		}
		outcomeAt := previous.OutcomeAt
		if outcomeAt == nil {
			outcomeAt = previous.OutcomeReceivedAt
		}
		estimate := EstimateArrival(ArrivalEstimateInput{
			PlannedArrivalAt: stop.PlannedArrivalAt, PlannedDepartureAt: plannedDeparture,
			PreviousOutcomeAt: outcomeAt, PreviousArrivedAt: previous.ArrivedAt,
			ServiceMinutesPerStop: &allowance, WindowCloseAt: stop.PlannedWindowClose,
			DeliveryDate: run.DeliveryDate, Now: now,
		})
		if estimate.Kind == "unknown" {
			continue
		}
		// The event that drove this estimate orders predictions: an older event never overwrites a newer one.
		source := outcomeAt
		if source == nil {
			source = previous.ArrivedAt
		}
		if _, err := s.recordArrivalPrediction(ctx, stop, run.PlanVersion, source, estimate.ETA, nil, nil, estimate.Risk); err != nil {
			s.logPredictionError("arrival_prediction_record_failed", tripID, stop.ID, err)
		}
	}
}
