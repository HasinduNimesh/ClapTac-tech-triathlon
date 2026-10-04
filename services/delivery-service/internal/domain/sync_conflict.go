package domain

import "time"

// SyncConflict is an offline record that was made on an older plan version.
// The record itself is applied; the conflict row asks the dispatcher to look.
type SyncConflict struct {
	ID                  string     `json:"id"`
	RunID               string     `json:"runId"`
	TripID              string     `json:"tripId,omitempty"`
	VehicleID           string     `json:"vehicleId,omitempty"`
	DeliveryDate        string     `json:"deliveryDate,omitempty"`
	StopID              string     `json:"stopId,omitempty"`
	OperationID         string     `json:"operationId"`
	RecordedPlanVersion int        `json:"recordedPlanVersion"`
	CurrentPlanVersion  int        `json:"currentPlanVersion"`
	Detail              string     `json:"detail"`
	CreatedAt           time.Time  `json:"createdAt"`
	SettledBy           string     `json:"settledBy,omitempty"`
	SettledAt           *time.Time `json:"settledAt,omitempty"`
}
