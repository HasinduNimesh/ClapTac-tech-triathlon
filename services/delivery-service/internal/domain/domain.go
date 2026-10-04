package domain

import "time"

const (
	RunPrepared   = "prepared"
	RunInProgress = "in_progress"
	RunCompleted  = "completed"

	StopPending   = "pending"
	StopArrived   = "arrived"
	StopCompleted = "completed"

	OutcomeDelivered    = "DELIVERED"
	OutcomePartial      = "PARTIAL"
	OutcomeNotDelivered = "NOT_DELIVERED"
	OutcomeFailed       = "FAILED"  // accepted for compatibility with older clients
	OutcomeRefused      = "REFUSED" // accepted for compatibility with older clients

	ReasonOutletClosed        = "OUTLET_CLOSED"
	ReasonAccessBlocked       = "ACCESS_BLOCKED"
	ReasonReceiverUnavailable = "RECEIVER_UNAVAILABLE"
	ReasonGoodsRejected       = "GOODS_REJECTED"
	ReasonVehicleIssue        = "VEHICLE_ISSUE"
	ReasonOther               = "OTHER"

	ProofSignature = "SIGNATURE"
	ProofPhoto     = "PHOTO"

	OpArrived            = "ARRIVED"
	OpStart              = "START"
	OpProofUpload        = "PROOF_UPLOAD"
	OpStopOutcome        = "STOP_OUTCOME"
	OpTemperatureReading = "TEMPERATURE_READING"
	OpRouteCompleted     = "ROUTE_COMPLETED"
	OpIncidentReport     = "INCIDENT_REPORT"

	// FR-22: categories for a Driver-reported incident.
	IncidentVehicle = "VEHICLE"
	IncidentRoad    = "ROAD"
	IncidentOutlet  = "OUTLET"
	IncidentGoods   = "GOODS"
	IncidentSafety  = "SAFETY"
	IncidentOther   = "OTHER"

	ResultApplied   = "APPLIED"
	ResultConflict  = "CONFLICT"
	ResultRejected  = "REJECTED"
	ResultDuplicate = "DUPLICATE"

	MaxSignatureBytes = 512 * 1024
	MaxPhotoBytes     = 4 * 1024 * 1024
)

type Run struct {
	ID                           string     `json:"id"`
	TripID                       string     `json:"tripId"`
	PlanID                       string     `json:"planId"`
	PlanRef                      string     `json:"planRef"`
	DeliveryDate                 string     `json:"deliveryDate"`
	VehicleID                    string     `json:"vehicleId"`
	Depot                        string     `json:"depot"`
	TripNumber                   int        `json:"tripNumber"`
	VehicleType                  string     `json:"vehicleType,omitempty"`
	VehicleTemperatureCapability string     `json:"vehicleTemperatureCapability,omitempty"`
	Status                       string     `json:"status"`
	StartedBy                    string     `json:"startedBy,omitempty"`
	StartedAt                    *time.Time `json:"startedAt,omitempty"`
	CompletedBy                  string     `json:"completedBy,omitempty"`
	CompletedAt                  *time.Time `json:"completedAt,omitempty"`
	Version                      int        `json:"version"`
	PlanVersion                  int        `json:"planVersion"`
	AcknowledgedVersion          int        `json:"acknowledgedVersion"`
}

type TripMessage struct {
	ID             string     `json:"id"`
	TripID         string     `json:"tripId"`
	StopID         string     `json:"stopId,omitempty"`
	Body           string     `json:"body"`
	SentBy         string     `json:"sentBy"`
	CreatedAt      time.Time  `json:"createdAt"`
	AcknowledgedBy string     `json:"acknowledgedBy,omitempty"`
	AcknowledgedAt *time.Time `json:"acknowledgedAt,omitempty"`
}

type Stop struct {
	ID                          string               `json:"id"`
	RunID                       string               `json:"runId"`
	AllocationID                string               `json:"allocationId"`
	OrderID                     string               `json:"orderId"`
	OrderRef                    string               `json:"orderRef"`
	ExpectedUnits               *int                 `json:"expectedUnits,omitempty"`
	UnitLabel                   string               `json:"unitLabel"`
	OutletID                    string               `json:"outletId"`
	Brand                       string               `json:"brand,omitempty"`
	OutletName                  string               `json:"outletName,omitempty"`
	District                    string               `json:"district,omitempty"`
	DockType                    string               `json:"dockType,omitempty"`
	ParkingConstraint           string               `json:"parkingConstraint,omitempty"`
	AccessInstructions          string               `json:"accessInstructions,omitempty"`
	AccessInstructionsUpdatedAt *time.Time           `json:"accessInstructionsUpdatedAt,omitempty"`
	StopSequence                int                  `json:"stopSequence"`
	PlannedArrivalAt            *time.Time           `json:"plannedArrivalAt,omitempty"`
	TemperatureRequirement      string               `json:"temperatureRequirement,omitempty"`
	ChilledTemperatureMinC      *float64             `json:"chilledTemperatureMinC,omitempty"`
	ChilledTemperatureMaxC      *float64             `json:"chilledTemperatureMaxC,omitempty"`
	TemperatureReadings         []TemperatureReading `json:"temperatureReadings,omitempty"`
	PlannedWindowOpen           string               `json:"plannedWindowOpen,omitempty"`
	PlannedWindowClose          string               `json:"plannedWindowClose,omitempty"`
	LoadingStatus               string               `json:"loadingStatus"`
	LoadingShortfallSummary     []any                `json:"loadingShortfallSummary"`
	Status                      string               `json:"status"`
	ArrivedAt                   *time.Time           `json:"arrivedAt,omitempty"`
	ArrivedReceivedAt           *time.Time           `json:"arrivedReceivedAt,omitempty"`
	OutcomeCode                 string               `json:"outcomeCode,omitempty"`
	OutcomeReason               string               `json:"outcomeReason,omitempty"`
	OutcomeNote                 string               `json:"outcomeNote,omitempty"`
	DeliveredUnits              *int                 `json:"deliveredUnits,omitempty"`
	ShortfallUnits              *int                 `json:"shortfallUnits,omitempty"`
	OutcomeAt                   *time.Time           `json:"outcomeAt,omitempty"`
	OutcomeReceivedAt           *time.Time           `json:"outcomeReceivedAt,omitempty"`
	CompletedAt                 *time.Time           `json:"completedAt,omitempty"`
	Version                     int                  `json:"version"`
}

type TemperatureReading struct {
	OperationID string    `json:"operationId"`
	ValueC      float64   `json:"valueC"`
	Unit        string    `json:"unit"`
	OccurredAt  time.Time `json:"occurredAt"`
	ReceivedAt  time.Time `json:"receivedAt"`
	ActorID     string    `json:"actorId"`
	Source      string    `json:"source"`
	Evaluation  string    `json:"evaluation"`
	MinC        *float64  `json:"minC,omitempty"`
	MaxC        *float64  `json:"maxC,omitempty"`
	Note        string    `json:"note,omitempty"`
}

// DriverIncident is FR-22's categorised field report: vehicle, road, outlet,
// goods, safety, or other. StopID is optional because not every incident
// (a road closure, a vehicle fault between stops) happens at a stop.
type DriverIncident struct {
	ID          string    `json:"id"`
	OperationID string    `json:"operationId"`
	RunID       string    `json:"runId"`
	StopID      string    `json:"stopId,omitempty"`
	Category    string    `json:"category"`
	Description string    `json:"description"`
	ReportedBy  string    `json:"reportedBy"`
	OccurredAt  time.Time `json:"occurredAt"`
	CreatedAt   time.Time `json:"createdAt"`
}

type Proof struct {
	ID             string     `json:"id"`
	StopID         string     `json:"stopId"`
	ProofType      string     `json:"proofType"`
	ObjectKey      string     `json:"objectKey"`
	MimeType       string     `json:"mimeType"`
	SHA256         string     `json:"sha256,omitempty"`
	CapturedAt     *time.Time `json:"capturedAt,omitempty"`
	UploadedAt     *time.Time `json:"uploadedAt,omitempty"`
	CreatedBy      string     `json:"createdBy"`
	IdempotencyKey string     `json:"idempotencyKey"`
	Pending        bool       `json:"pending"`
	// ReceiverName is who accepted the delivery (FR-25), as reported by the
	// Driver. Optional: not every proof capture has a named recipient.
	ReceiverName string `json:"receiverName,omitempty"`
}

type OrderTracking struct {
	RunID                   string         `json:"runId"`
	TripID                  string         `json:"tripId"`
	VehicleID               string         `json:"vehicleId,omitempty"`
	RunStatus               string         `json:"runStatus"`
	StopID                  string         `json:"stopId"`
	Outcome                 string         `json:"outcome"`
	Reason                  string         `json:"reason,omitempty"`
	OccurredAt              *time.Time     `json:"occurredAt,omitempty"`
	CompletedAt             *time.Time     `json:"completedAt,omitempty"`
	Proofs                  []ProofSummary `json:"proofs"`
	LoadingShortfallSummary []any          `json:"loadingShortfallSummary"`
}

type OutletLastServed struct {
	OutletID     string    `json:"outletId"`
	LastServedAt time.Time `json:"lastServedAt"`
}

// OutletLastAttempted is distinct from OutletLastServed: it covers every
// terminal delivery outcome (DELIVERED, PARTIAL, NOT_DELIVERED, REFUSED),
// not just successful ones. FR-53's repeat-deferral warning needs this - an
// outlet whose last run was an attempted-but-failed delivery was not
// deferred on that run, even though it was not successfully served either.
type OutletLastAttempted struct {
	OutletID        string    `json:"outletId"`
	LastAttemptedAt time.Time `json:"lastAttemptedAt"`
}

type ProofSummary struct {
	OperationID  string     `json:"operationId,omitempty"`
	Type         string     `json:"type"`
	MimeType     string     `json:"mimeType"`
	UploadedAt   *time.Time `json:"uploadedAt,omitempty"`
	Pending      bool       `json:"pending"`
	ReceiverName string     `json:"receiverName,omitempty"`
}

type SyncOp struct {
	OperationID   string
	RunID         string
	StopID        string
	OperationType string
	OccurredAt    *time.Time
	ReceivedAt    time.Time
	ResultStatus  string
	ResultPayload map[string]any
}

// OfflineQueueHealth contains only bounded buckets. It deliberately omits
// operation IDs, trip/stop IDs, timestamps, and driver identifiers.
type OfflineQueueHealth struct {
	AgeBucket   string `json:"ageBucket"`
	CountBucket string `json:"countBucket"`
}

type LoadingTrip struct {
	TripID                       string                `json:"tripId"`
	PlanID                       string                `json:"planId"`
	PlanRef                      string                `json:"planRef"`
	DeliveryDate                 string                `json:"deliveryDate"`
	VehicleID                    string                `json:"vehicleId"`
	VehicleType                  string                `json:"vehicleType"`
	VehicleTemperatureCapability string                `json:"vehicleTemperatureCapability"`
	Depot                        string                `json:"depot"`
	TripNumber                   int                   `json:"tripNumber"`
	LoadingStatus                string                `json:"loadingStatus"`
	PlanVersion                  int                   `json:"planVersion"`
	PlanAcknowledgements         []PlanAcknowledgement `json:"planAcknowledgements"`
	Orders                       []LoadingOrder        `json:"orders"`
}

type PlanAcknowledgement struct {
	ActorID        string    `json:"actorId"`
	ActorRole      string    `json:"actorRole"`
	AcknowledgedAt time.Time `json:"acknowledgedAt"`
}

type LoadingOrder struct {
	AllocationID           string     `json:"allocationId"`
	OrderID                string     `json:"orderId"`
	OrderRef               string     `json:"orderRef"`
	OutletID               string     `json:"outletId"`
	Brand                  string     `json:"brand"`
	StopSequence           int        `json:"stopSequence"`
	PlannedArrivalAt       *time.Time `json:"plannedArrivalAt,omitempty"`
	LoadingStatus          string     `json:"loadingStatus"`
	TemperatureRequirement string     `json:"temperatureRequirement"`
	ExpectedUnits          int        `json:"expectedUnits"`
	ShortfallSummary       []any      `json:"shortfallSummary"`
}

type Outlet struct {
	ID                          string     `json:"id"`
	Name                        string     `json:"name"`
	District                    string     `json:"district"`
	DockType                    string     `json:"dockType"`
	ParkingConstraint           string     `json:"parkingConstraint"`
	WindowOpenTime              string     `json:"windowOpenTime"`
	WindowCloseTime             string     `json:"windowCloseTime"`
	AccessInstructions          string     `json:"accessInstructions"`
	AccessInstructionsUpdatedAt *time.Time `json:"accessInstructionsUpdatedAt"`
	ChilledTemperatureMinC      *float64   `json:"chilledTemperatureMinC,omitempty"`
	ChilledTemperatureMaxC      *float64   `json:"chilledTemperatureMaxC,omitempty"`
}

type SyncRequest struct {
	Operations []SyncOperation `json:"operations"`
}

type SyncOperation struct {
	OperationID          string         `json:"operationId"`
	Type                 string         `json:"type"`
	RunID                string         `json:"runId"`
	StopID               string         `json:"stopId"`
	TripID               string         `json:"tripId"`
	OccurredAt           string         `json:"occurredAt"`
	DependsOnOperationID string         `json:"dependsOnOperationId"`
	Payload              map[string]any `json:"payload"`
	// PlanVersion is the plan version the driver app was showing when the
	// record was made offline. Optional; zero means unknown.
	PlanVersion int `json:"planVersion,omitempty"`
}
