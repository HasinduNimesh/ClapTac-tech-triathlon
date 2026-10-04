package service

import (
	"regexp"
	"time"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/delivery-service/internal/domain"
)

// ArrivalEstimateVersion names the rule below. It is the same rule the dispatcher web app shows
// (apps/web/src/dispatcher/arrivalEstimate.mjs) so a store is told what the dispatcher sees.
const ArrivalEstimateVersion = "event_delay_propagation_v2"

// DefaultServiceMinutesPerStop is the fixed allowance the web falls back to ("fixed_20m_v1").
const DefaultServiceMinutesPerStop = 20

// ArrivalEstimateInput mirrors the web's estimateArrival input. A nil time means "not known".
type ArrivalEstimateInput struct {
	PlannedArrivalAt   *time.Time
	PlannedDepartureAt *time.Time // when the previous reported stop was planned to be finished
	PreviousOutcomeAt  *time.Time // the previous reported stop's recorded outcome time
	PreviousArrivedAt  *time.Time // the previous reported stop's arrival time
	// ServiceMinutesPerStop is the allowance added to an arrival whose outcome is not yet known.
	// nil or negative means the 20-minute default.
	ServiceMinutesPerStop *int
	WindowCloseAt         string // "HH:MM[:SS]" (Sri Lanka time on DeliveryDate) or an RFC 3339 instant
	DeliveryDate          string // "YYYY-MM-DD"
	Now                   time.Time
}

type ArrivalEstimate struct {
	Version      string
	Kind         string // "unknown", "risk", "late" or "estimate"
	ETA          time.Time
	Risk         string
	DelayMinutes int
}

var clockTime = regexp.MustCompile(`^\d{2}:\d{2}(:\d{2})?$`)

// previousReportedStop returns the closest stop before beforeIndex the driver has reported on.
func previousReportedStop(stops []domain.Stop, beforeIndex int) *domain.Stop {
	for i := beforeIndex - 1; i >= 0; i-- {
		if stops[i].OutcomeAt != nil || stops[i].OutcomeReceivedAt != nil || stops[i].ArrivedAt != nil {
			return &stops[i]
		}
	}
	return nil
}

// EstimateArrival projects a downstream stop from its published schedule and the last driver-reported
// event: the delay observed at the previous stop (its outcome time, or its arrival plus the service
// allowance, against the time it was planned to depart) is carried forward onto the planned arrival.
// It is event-based, not GPS, and deterministic for a given input.
func EstimateArrival(in ArrivalEstimateInput) ArrivalEstimate {
	unknown := ArrivalEstimate{Version: ArrivalEstimateVersion, Kind: "unknown"}
	if in.PlannedArrivalAt == nil || in.PlannedArrivalAt.IsZero() {
		return unknown
	}
	allowance := time.Duration(DefaultServiceMinutesPerStop) * time.Minute
	if in.ServiceMinutesPerStop != nil && *in.ServiceMinutesPerStop >= 0 {
		allowance = time.Duration(*in.ServiceMinutesPerStop) * time.Minute
	}
	var delay time.Duration
	var actualPrevious *time.Time
	if in.PreviousOutcomeAt != nil {
		actualPrevious = in.PreviousOutcomeAt
	} else if in.PreviousArrivedAt != nil {
		t := in.PreviousArrivedAt.Add(allowance)
		actualPrevious = &t
	}
	if actualPrevious != nil && in.PlannedDepartureAt != nil {
		if d := actualPrevious.Sub(*in.PlannedDepartureAt); d > 0 {
			delay = d
		}
	}
	estimated := in.PlannedArrivalAt.Add(delay)
	closeAt, hasClose := windowClose(in.WindowCloseAt, in.DeliveryDate)
	// An open stop is already a missed-window exception once its window has closed, even when stale
	// schedule math still predicts an earlier arrival. Otherwise tell a projected window breach from an
	// ETA that has already elapsed, then apply the near-window warning.
	var risk string
	switch {
	case hasClose && in.Now.After(closeAt):
		risk = "Window missed"
	case hasClose && estimated.After(closeAt):
		risk = "Window at risk"
	case estimated.Before(in.Now):
		risk = "ETA passed"
	case hasClose && closeAt.Sub(estimated) <= 15*time.Minute:
		risk = "Watch window"
	default:
		risk = "On track"
	}
	kind := "estimate"
	if risk == "Window missed" || risk == "Window at risk" {
		kind = "risk"
	} else if risk == "ETA passed" {
		kind = "late"
	}
	return ArrivalEstimate{
		Version: ArrivalEstimateVersion, Kind: kind, ETA: estimated.UTC(), Risk: risk,
		// Math.round on a non-negative number: halves round up.
		DelayMinutes: int((delay + 30*time.Second) / time.Minute),
	}
}

func windowClose(value, deliveryDate string) (time.Time, bool) {
	if clockTime.MatchString(value) && deliveryDate != "" {
		if len(value) == 5 {
			value += ":00"
		}
		if t, err := time.Parse(time.RFC3339, deliveryDate+"T"+value+"+05:30"); err == nil {
			return t, true
		}
		return time.Time{}, false
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t, true
	}
	return time.Time{}, false
}
