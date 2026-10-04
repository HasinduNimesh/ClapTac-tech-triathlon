package domain

import "time"

const (
	StatusDraft     = "draft"
	StatusValidated = "validated"
	StatusConfirmed = "confirmed"

	ReasonNoEligibleVehicle  = "NO_ELIGIBLE_VEHICLE"
	ReasonWeightExceeded     = "WEIGHT_CAPACITY_EXCEEDED"
	ReasonVolumeExceeded     = "VOLUME_CAPACITY_EXCEEDED"
	ReasonRefrigeration      = "REFRIGERATION_REQUIRED"
	ReasonVanRequired        = "VAN_REQUIRED"
	ReasonDepotMismatch      = "DEPOT_MISMATCH"
	ReasonDeliveryWindow     = "DELIVERY_WINDOW_CONFLICT"
	ReasonFuelExceeded       = "FUEL_QUOTA_EXCEEDED"
	ReasonTripLimit          = "TRIP_LIMIT_REACHED"
	ReasonVehicleUnavailable = "VEHICLE_UNAVAILABLE"
	ReasonManualDeferral     = "MANUAL_DISPATCHER_DEFERRAL"
	// ReasonUnavailable marks an order whose real persisted reason could not
	// be read back, so the Dispatcher is told the explanation is missing
	// rather than being shown a specific-looking but possibly wrong reason.
	ReasonUnavailable = "REASON_UNAVAILABLE"
)

type Result struct {
	Valid      bool           `json:"valid"`
	ReasonCode string         `json:"reasonCode"`
	Details    map[string]any `json:"details,omitempty"`
}

type Outlet struct {
	ID                string `json:"id"`
	Brand             string `json:"brand"`
	Name              string `json:"name,omitempty"`
	DockType          string `json:"dockType,omitempty"`
	District          string `json:"district"`
	Depot             string `json:"depot"`
	ParkingConstraint string `json:"parkingConstraint"`
	MallWindow        bool   `json:"mallWindow"`
	WindowOpen        string `json:"windowOpen"`
	WindowClose       string `json:"windowClose"`
}

type Order struct {
	ID                  string     `json:"id"`
	OrderRef            string     `json:"orderRef"`
	SourceSystem string `json:"sourceSystem,omitempty"`
	OutletID            string     `json:"outletId"`
	Brand               string     `json:"brand"`
	Temp                string     `json:"temperatureRequirement"`
	WeightKg            float64    `json:"orderWeightKg"`
	VolumeM3            float64    `json:"orderVolumeM3"`
	Outlet              Outlet     `json:"outlet"`
	OutletDeferralCount int        `json:"outletDeferralCount,omitempty"`
	DaysSinceLastServed int        `json:"daysSinceLastServed"`
	LastServedAt        *time.Time `json:"lastServedAt,omitempty"`
	FairnessScore       int        `json:"fairnessScore"`
	// DeferredLastRun and LastDeferralDate are FR-53's repeat-deferral warning:
	// whether this outlet's most recent earlier plan deferred it, so the
	// Dispatcher can see a pattern before deferring it again.
	DeferredLastRun bool   `json:"deferredLastRun,omitempty"`
	LastDeferralDate string `json:"lastDeferralDate,omitempty"`
	// PriorityNextPlan (W3) flags an outlet deferred on two consecutive runs
	// ("consecutive" as for DeferredLastRun: no delivery attempt between the
	// deferrals). It is derived from persisted deferral history, not from the
	// plan being viewed: it is true on the plan that records the second
	// deferral and stays true on the following plans until the outlet is
	// attempted. It is a flag only; the allocation score is not changed by it.
	PriorityNextPlan bool `json:"priorityNextPlan,omitempty"`
}

type Vehicle struct {
	ID               string  `json:"id"`
	Type             string  `json:"type"`
	Temp             string  `json:"temp"`
	HomeDepot        string  `json:"homeDepot"`
	Status           string  `json:"status"`
	WeightCap        float64 `json:"weightCapacityKg"`
	VolumeCap        float64 `json:"volumeCapacityM3"`
	KmPerL           float64 `json:"kmPerL"`
	WeeklyQuotaL     float64 `json:"weeklyFuelQuotaL"`
	WeekFuelUsedL    float64 `json:"weekFuelUsedL,omitempty"`
	WeekFuelPlannedL float64 `json:"weekFuelPlannedL"`
	WeekFuelActualL  float64 `json:"weekFuelActualL"`
	PlanFuelL        float64 `json:"planFuelL"`
}

type PlanningPolicy struct {
	Version              int    `json:"version"`
	CutoffLocalTime      string `json:"cutoffLocalTime"`
	DeferralWeightPoints int    `json:"deferralWeightPoints"`
	MaxDeferralCount     int    `json:"maxDeferralCount"`
	MaxUnservedDays      int    `json:"maxUnservedDays"`
	MaxTripsPerVehicle   int    `json:"maxTripsPerVehicle"`
}

type DisruptionRisk struct {
	ID               string     `json:"id"`
	DeliveryDate     string     `json:"deliveryDate"`
	Scope            string     `json:"scope"`
	ScopeKey         string     `json:"scopeKey"`
	RiskType         string     `json:"riskType"`
	Severity         string     `json:"severity"`
	Summary          string     `json:"summary"`
	Source           string     `json:"source"`
	SourceReference  string     `json:"sourceReference,omitempty"`
	Confidence       float64    `json:"confidence"`
	CreatedBy        string     `json:"createdBy"`
	CreatedAt        time.Time  `json:"createdAt"`
	OverrideDecision string     `json:"overrideDecision,omitempty"`
	OverrideSeverity string     `json:"overrideSeverity,omitempty"`
	OverrideReason   string     `json:"overrideReason,omitempty"`
	OverriddenBy     string     `json:"overriddenBy,omitempty"`
	OverriddenAt     *time.Time `json:"overriddenAt,omitempty"`
}

type Stop struct {
	OrderID, OutletID     string
	Arrival, ServiceStart time.Time
	Depart                time.Time
	WeightKg, VolumeM3    float64
}

type TripState struct {
	VehicleID  string
	TripNumber int
	Stops      []Stop
}

type Plan struct {
	ID             string     `json:"id"`
	PlanRef        string     `json:"planRef"`
	DeliveryDate   string     `json:"deliveryDate"`
	Status         string     `json:"status"`
	CreatedBy      string     `json:"createdBy"`
	GeneratedAt    *time.Time `json:"generatedAt,omitempty"`
	CurrentVersion int        `json:"currentVersion"`
	PublishedAt    *time.Time `json:"publishedAt,omitempty"`
}

type Publication struct {
	Version          int                   `json:"version"`
	ContentHash      string                `json:"contentHash"`
	PublishedBy      string                `json:"publishedBy"`
	PublishedAt      time.Time             `json:"publishedAt"`
	Acknowledgements []PlanAcknowledgement `json:"acknowledgements"`
}

type PlanAcknowledgement struct {
	ActorID        string    `json:"actorId"`
	ActorRole      string    `json:"actorRole"`
	AcknowledgedAt time.Time `json:"acknowledgedAt"`
}

type Trip struct {
	ID         string `json:"id"`
	PlanID     string `json:"planId"`
	VehicleID  string `json:"vehicleId"`
	TripNumber int    `json:"tripNumber"`
	Status     string `json:"status"`
}

type Allocation struct {
	ID                    string     `json:"id"`
	PlanID                string     `json:"planId"`
	OrderID               string     `json:"orderId"`
	TripID                string     `json:"tripId"`
	VehicleID             string     `json:"vehicleId"`
	Sequence              int        `json:"sequence"`
	PlannedArrivalAt      *time.Time `json:"plannedArrivalAt,omitempty"`
	PlannedServiceStartAt *time.Time `json:"plannedServiceStartAt,omitempty"`
	PlannedDepartureAt    *time.Time `json:"plannedDepartureAt,omitempty"`
}

type Deferral struct {
	ID            string         `json:"id"`
	PlanID        string         `json:"planId"`
	OrderID       string         `json:"orderId"`
	OutletID      string         `json:"outletId"`
	ReasonCode    string         `json:"reasonCode"`
	Comment       string         `json:"comment,omitempty"`
	DeferredBy    string         `json:"deferredBy"`
	ReasonDetail  map[string]any `json:"reasonDetail,omitempty"`
	// NextRunTarget is the delivery date (YYYY-MM-DD) the Dispatcher expects to
	// retry this order on. Optional: not every deferral has a known next run.
	NextRunTarget string `json:"nextRunTarget,omitempty"`
}

type ConstraintFailure struct {
	OrderID    string         `json:"orderId"`
	ReasonCode string         `json:"reasonCode"`
	Details    map[string]any `json:"details,omitempty"`
}

// UnallocatedReason is the persisted form of a ConstraintFailure: the allocator's
// explanation for one order on one plan, kept so it survives past the generate
// response and still explains the order if the Dispatcher reopens the plan later.
type UnallocatedReason struct {
	PlanID     string         `json:"planId"`
	OrderID    string         `json:"orderId"`
	ReasonCode string         `json:"reasonCode"`
	Details    map[string]any `json:"details,omitempty"`
}

type GenerateResult struct {
	Allocated               int                 `json:"allocated"`
	Unallocated             int                 `json:"unallocated"`
	Failures                []ConstraintFailure `json:"failures"`
	FairnessSignalAvailable bool                `json:"fairnessSignalAvailable"`
	FairnessPolicy          string              `json:"fairnessPolicy"`
}

type InternalAllocation struct {
	AllocationID       string     `json:"allocationId"`
	OrderID            string     `json:"orderId"`
	OrderRef           string     `json:"orderRef"`
	OutletID           string     `json:"outletId"`
	StopSequence       int        `json:"stopSequence"`
	PlannedArrivalAt   *time.Time `json:"plannedArrivalAt,omitempty"`
	PlannedDepartureAt *time.Time `json:"plannedDepartureAt,omitempty"`
	// Load and access details the loader needs at the dock.
	Brand             string  `json:"brand,omitempty"`
	WeightKg          float64 `json:"weightKg"`
	VolumeM3          float64 `json:"volumeM3"`
	Temperature       string  `json:"temperatureRequirement,omitempty"`
	OutletName        string  `json:"outletName,omitempty"`
	DockType          string  `json:"dockType,omitempty"`
	District          string  `json:"district,omitempty"`
	WindowOpen        string  `json:"windowOpen,omitempty"`
	WindowClose       string  `json:"windowClose,omitempty"`
	ParkingConstraint string  `json:"parkingConstraint,omitempty"`
	MallWindow        bool    `json:"mallWindow,omitempty"`
}

type InternalTrip struct {
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
	// PlannedDepartureAt is when the trip leaves the depot and PlannedReturnAt
	// when it is back, from the same travel estimate the plan was built with.
	PlannedDepartureAt *time.Time           `json:"plannedDepartureAt,omitempty"`
	PlannedReturnAt    *time.Time           `json:"plannedReturnAt,omitempty"`
	Allocations        []InternalAllocation `json:"allocations"`
}

type OrderTracking struct {
	State                 string         `json:"state"`
	PlanID                string         `json:"planId,omitempty"`
	PlanRef               string         `json:"planRef,omitempty"`
	TripID                string         `json:"tripId,omitempty"`
	VehicleID             string         `json:"vehicleId,omitempty"`
	Depot                 string         `json:"depot,omitempty"`
	StopSequence          int            `json:"stopSequence,omitempty"`
	ReasonCode            string         `json:"reasonCode,omitempty"`
	ReasonComment         string         `json:"reasonComment,omitempty"`
	ReasonDetail          map[string]any `json:"reasonDetail,omitempty"`
	PlannedArrivalAt      *time.Time     `json:"plannedArrivalAt,omitempty"`
	PlannedServiceStartAt *time.Time     `json:"plannedServiceStartAt,omitempty"`
}
