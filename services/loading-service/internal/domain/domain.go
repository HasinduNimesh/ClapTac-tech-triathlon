package domain

import "time"

const (
	SessionPending    = "pending"
	SessionInProgress = "in_progress"
	SessionReady      = "ready"

	LoadPending   = "pending"
	LoadLoaded    = "loaded"
	LoadShortfall = "shortfall"

	IssueMissing   = "MISSING"
	IssueDamaged   = "DAMAGED"
	IssueWrongItem = "WRONG_ITEM"

	AlertWrongVehicle = "WRONG_VEHICLE"

	// FreshTripBudgetMinutes is the Fresh delivery window (03:30 to 08:00)
	// each Fresh trip must fit in.
	FreshTripBudgetMinutes = 270
	// Chilled zone range the loader confirms before a refrigerated trip leaves.
	ChilledZoneMinC = 2.0
	ChilledZoneMaxC = 4.0

	// Dispatcher decisions on a loader shortfall.
	DecisionPartialLoad    = "PARTIAL_LOAD"
	DecisionHold           = "HOLD"
	DecisionMoveToNextRun  = "MOVE_TO_NEXT_RUN"
)

// DecisionAllowsDeparture reports whether a decided shortfall lets the trip leave.
func DecisionAllowsDeparture(decision string) bool {
	return decision == DecisionPartialLoad || decision == DecisionMoveToNextRun
}

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
	ReadyTemperatureC            *float64   `json:"readyTemperatureC,omitempty"`
	ReadySeal                    string     `json:"readySeal,omitempty"`
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
	// ChangedInVersion and ChangeNote record how a newer plan version changed
	// this line (for example "Moved from Stop 4"), so the loader can recheck it.
	ChangedInVersion int    `json:"changedInVersion,omitempty"`
	ChangeNote       string `json:"changeNote,omitempty"`
}

type Issue struct {
	ID             string `json:"id"`
	OrderLoadID    string `json:"orderLoadId"`
	IssueType      string `json:"type"`
	AffectedUnits  int    `json:"affectedUnits"`
	Note           string `json:"note,omitempty"`
	ReportedBy     string     `json:"reportedBy"`
	IdempotencyKey string     `json:"idempotencyKey,omitempty"`
	Decision       string     `json:"decision,omitempty"`
	DecisionNote   string     `json:"decisionNote,omitempty"`
	DecidedBy      string     `json:"decidedBy,omitempty"`
	DecidedAt      *time.Time `json:"decidedAt,omitempty"`
	ReportedAt     *time.Time `json:"reportedAt,omitempty"`
	HasPhoto       bool       `json:"hasPhoto"`
	PhotoKey       string     `json:"-"`
	PhotoMime      string     `json:"-"`
	SeenBy         string     `json:"seenBy,omitempty"`
	SeenAt         *time.Time `json:"seenAt,omitempty"`
}

// DockAlert is a loader's "tell dispatcher" note, such as goods staged at the
// wrong vehicle.
type DockAlert struct {
	ID               string     `json:"id"`
	TripID           string     `json:"tripId"`
	Depot            string     `json:"depot"`
	DeliveryDate     string     `json:"deliveryDate"`
	Type             string     `json:"type"`
	OrderRef         string     `json:"orderRef"`
	BelongsVehicleID string     `json:"belongsVehicleId,omitempty"`
	Note             string     `json:"note,omitempty"`
	ReportedBy       string     `json:"reportedBy"`
	CreatedAt        time.Time  `json:"createdAt"`
	ResolvedBy       string     `json:"resolvedBy,omitempty"`
	ResolvedAt       *time.Time `json:"resolvedAt,omitempty"`
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
	VehicleWeightCapacityKg      float64               `json:"vehicleWeightCapacityKg"`
	VehicleVolumeCapacityM3      float64               `json:"vehicleVolumeCapacityM3"`
	PlanPublishedBy              string                `json:"planPublishedBy,omitempty"`
	PlannedDepartureAt           *time.Time            `json:"plannedDepartureAt,omitempty"`
	PlannedReturnAt              *time.Time            `json:"plannedReturnAt,omitempty"`
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
	StopSequence       int        `json:"stopSequence"`
	PlannedArrivalAt   *time.Time `json:"plannedArrivalAt,omitempty"`
	PlannedDepartureAt *time.Time `json:"plannedDepartureAt,omitempty"`
	Brand              string     `json:"brand,omitempty"`
	WeightKg           float64    `json:"weightKg"`
	VolumeM3           float64    `json:"volumeM3"`
	Temperature        string     `json:"temperatureRequirement,omitempty"`
	OutletName         string     `json:"outletName,omitempty"`
	DockType           string     `json:"dockType,omitempty"`
	District           string     `json:"district,omitempty"`
	WindowOpen         string     `json:"windowOpen,omitempty"`
	WindowClose        string     `json:"windowClose,omitempty"`
	ParkingConstraint  string     `json:"parkingConstraint,omitempty"`
	MallWindow         bool       `json:"mallWindow,omitempty"`
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
