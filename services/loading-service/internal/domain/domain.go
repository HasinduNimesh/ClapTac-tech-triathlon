package domain

import "time"

const (
	SessionPending    = "pending"
	SessionInProgress = "in_progress"
	SessionReady      = "ready"

	LoadPending   = "pending"
	LoadLoaded    = "loaded"
	LoadShortfall = "shortfall"

	IssueMissing = "MISSING"
	IssueDamaged = "DAMAGED"
)

type Session struct {
	ID                           string     `json:"id"`
	TripID                       string     `json:"tripId"`
	PlanID                       string     `json:"planId"`
	PlanRef                      string     `json:"planRef"`
	DeliveryDate                 string     `json:"deliveryDate"`
	VehicleID                    string     `json:"vehicleId"`
	TripNumber                   int        `json:"tripNumber,omitempty"`
	VehicleType                  string     `json:"vehicleType,omitempty"`
	VehicleTemperatureCapability string     `json:"vehicleTemperatureCapability,omitempty"`
	Depot                        string     `json:"depot"`
	Status                       string     `json:"status"`
	StartedBy                    string     `json:"startedBy"`
	StartedAt                    time.Time  `json:"startedAt"`
	ReadyBy                      string     `json:"readyBy,omitempty"`
	ReadyAt                      *time.Time `json:"readyAt,omitempty"`
	PlanVersion                  int        `json:"planVersion"`
	AcknowledgedVersion          int        `json:"acknowledgedVersion"`
}

type OrderLoad struct {
	ID                     string `json:"id"`
	SessionID              string `json:"sessionId"`
	AllocationID           string `json:"allocationId"`
	OrderID                string `json:"orderId"`
	OrderRef               string `json:"orderRef,omitempty"`
	OutletID               string `json:"outletId,omitempty"`
	Brand                  string `json:"brand,omitempty"`
	TemperatureRequirement string `json:"temperatureRequirement,omitempty"`
	ExpectedUnits          int    `json:"expectedUnits"`
	StopSequence           int    `json:"stopSequence"`
	SuggestedLoadSequence  int    `json:"suggestedLoadSequence"`
	Status                 string `json:"status"`
	UpdatedBy              string `json:"updatedBy,omitempty"`
}

type Issue struct {
	ID             string `json:"id"`
	OrderLoadID    string `json:"orderLoadId"`
	IssueType      string `json:"type"`
	AffectedUnits  int    `json:"affectedUnits"`
	Note           string `json:"note,omitempty"`
	ReportedBy     string `json:"reportedBy"`
	IdempotencyKey string `json:"idempotencyKey,omitempty"`
}

type PlanningTrip struct {
	PlanID                       string                `json:"planId"`
	PlanRef                      string                `json:"planRef"`
	DeliveryDate                 string                `json:"deliveryDate"`
	PlanStatus                   string                `json:"planStatus"`
	PlanVersion                  int                   `json:"planVersion"`
	PlanPublishedAt              *time.Time            `json:"planPublishedAt,omitempty"`
	PlanAcknowledgements         []PlanAcknowledgement `json:"planAcknowledgements"`
	TripID                       string                `json:"tripId"`
	TripNumber                   int                   `json:"tripNumber"`
	VehicleID                    string                `json:"vehicleId"`
	VehicleType                  string                `json:"vehicleType"`
	VehicleTemperatureCapability string                `json:"vehicleTemperatureCapability"`
	VehicleDepot                 string                `json:"vehicleDepot"`
	Allocations                  []PlanningAlloc       `json:"allocations"`
}

type PlanAcknowledgement struct {
	ActorID        string    `json:"actorId"`
	ActorRole      string    `json:"actorRole"`
	AcknowledgedAt time.Time `json:"acknowledgedAt"`
}

type PlanningAlloc struct {
	AllocationID     string     `json:"allocationId"`
	OrderID          string     `json:"orderId"`
	OrderRef         string     `json:"orderRef"`
	OutletID         string     `json:"outletId"`
	StopSequence     int        `json:"stopSequence"`
	PlannedArrivalAt *time.Time `json:"plannedArrivalAt,omitempty"`
}

type OrderDetail struct {
	ID                     string  `json:"id"`
	OrderRef               string  `json:"orderRef"`
	OutletID               string  `json:"outletId"`
	Brand                  string  `json:"brand"`
	OrderUnits             int     `json:"orderUnits"`
	OrderWeightKg          float64 `json:"orderWeightKg"`
	OrderVolumeM3          float64 `json:"orderVolumeM3"`
	TemperatureRequirement string  `json:"temperatureRequirement"`
}

type Stop struct {
	OrderID      string
	StopSequence int
}

type LoadInstruction struct {
	OrderID               string
	StopSequence          int
	SuggestedLoadSequence int
}
