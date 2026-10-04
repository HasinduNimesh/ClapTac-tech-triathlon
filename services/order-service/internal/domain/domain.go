package domain

import "time"

type Temperature string

const (
	TempAmbient     Temperature = "ambient"
	TempChilled     Temperature = "chilled"
	StatusConfirmed             = "confirmed"
)

type Order struct {
	ID                     string      `json:"id"`
	OrderRef               string      `json:"orderRef"`
	OutletID               string      `json:"outletId"`
	Brand                  string      `json:"brand"`
	RequestedDeliveryDate  string      `json:"requestedDeliveryDate"`
	OrderUnits             int         `json:"orderUnits"`
	OrderWeightKg          float64     `json:"orderWeightKg"`
	OrderVolumeM3          float64     `json:"orderVolumeM3"`
	TemperatureRequirement Temperature `json:"temperatureRequirement"`
	Status                 string      `json:"status"`
	CreatedBy              string      `json:"createdBy"`
	CreatedAt              time.Time   `json:"createdAt"`
	SourceSystem           string      `json:"sourceSystem,omitempty"`
	ExternalOrderID        string      `json:"externalOrderId,omitempty"`
}

type ImportResult struct {
	Order   Order `json:"order"`
	Created bool  `json:"created"`
}

type CreateRequest struct {
	RequestedDeliveryDate  string      `json:"requestedDeliveryDate"`
	OrderUnits             int         `json:"orderUnits"`
	OrderWeightKg          float64     `json:"orderWeightKg"`
	OrderVolumeM3          float64     `json:"orderVolumeM3"`
	TemperatureRequirement Temperature `json:"temperatureRequirement"`
}

type ListFilter struct {
	OutletID              string
	Status                string
	Brand                 string
	RequestedDeliveryDate string
}

// Forecast is a deterministic, history-based operational estimate. It is not a
// calibrated prediction and includes an explicit data window and method.
type Forecast struct {
	GeneratedAt                    time.Time               `json:"generatedAt"`
	ForecastVersion                string                  `json:"forecastVersion"`
	Method                         string                  `json:"method"`
	HistoryWeeks                   int                     `json:"historyWeeks"`
	DriftModelVersion              string                  `json:"driftModelVersion"`
	BacktestModelVersion           string                  `json:"backtestModelVersion"`
	InputDrift                     []ForecastInputDrift    `json:"inputDrift"`
	Weekly                         []ForecastBucket        `json:"weekly"`
	ServiceMinutesPerStop          int                     `json:"serviceMinutesPerStop"`
	ServiceEstimateVersion         string                  `json:"serviceEstimateVersion"`
	ServiceEstimateSource          string                  `json:"serviceEstimateSource"`
	ServiceTimeBacktestVersion     string                  `json:"serviceTimeBacktestVersion"`
	ServiceTimeBacktestWindowStart string                  `json:"serviceTimeBacktestWindowStart"`
	ServiceTimeBacktestWindowEnd   string                  `json:"serviceTimeBacktestWindowEnd"`
	ServiceTimeEvaluation          []ServiceTimeEvaluation `json:"serviceTimeEvaluation"`
	Capacity                       []ForecastCapacity      `json:"capacity"`
}
type ServiceTimeEvaluation struct {
	Depot                    string   `json:"depot"`
	Brand                    string   `json:"brand"`
	ActualStopCount          int      `json:"actualStopCount"`
	ConfiguredMinutes        int      `json:"configuredMinutes"`
	MeanObservedMinutes      *float64 `json:"meanObservedMinutes,omitempty"`
	MeanAbsoluteErrorMinutes *float64 `json:"meanAbsoluteErrorMinutes,omitempty"`
	Status                   string   `json:"status"`
}
type ForecastInputDrift struct {
	Depot              string   `json:"depot"`
	Brand              string   `json:"brand"`
	PreviousOrderCount int      `json:"previousOrderCount"`
	RecentOrderCount   int      `json:"recentOrderCount"`
	ChangePercent      *float64 `json:"changePercent,omitempty"`
	BacktestAPEPercent *float64 `json:"backtestAPEPercent,omitempty"`
	Status             string   `json:"status"`
}
type ForecastBucket struct {
	WeekStarting      string  `json:"weekStarting"`
	Depot             string  `json:"depot"`
	Brand             string  `json:"brand"`
	ChilledOrders     int     `json:"chilledOrders"`
	AmbientOrders     int     `json:"ambientOrders"`
	EstimatedWeightKg float64 `json:"estimatedWeightKg"`
	EstimatedVolumeM3 float64 `json:"estimatedVolumeM3"`
	Estimate          bool    `json:"estimate"`
}
type ForecastCapacity struct {
	Depot                     string  `json:"depot"`
	ProjectedWeightKg         float64 `json:"projectedWeightKg"`
	ProjectedVolumeM3         float64 `json:"projectedVolumeM3"`
	EstimatedWeightCapacityKg float64 `json:"estimatedWeightCapacityKg"`
	EstimatedVolumeCapacityM3 float64 `json:"estimatedVolumeCapacityM3"`
	Pressure                  string  `json:"pressure"`
}

type Repository interface {
	Create(order Order) (Order, error)
	Get(id string) (Order, error)
	List(filter ListFilter) ([]Order, error)
	GetReceipt(orderID string) (Receipt, []ReceiptIssue, error)
	AddCustodyEvent(order Order, event CustodyEvent) (CustodyEvent, bool, error)
	ListCustodyEvents(orderID string) ([]CustodyEvent, error)
	ConfirmReceipt(order Order, confirmation ReceiptConfirmation) (Receipt, []ReceiptIssue, bool, error)
	AddReceiptIssue(order Order, issue ReceiptIssueRequest, actor string) (Receipt, ReceiptIssue, bool, error)
	ListReceiptIssues() ([]ReceiptIssueView, error)
	Forecast(now time.Time) (Forecast, error)
	ImportOrders(source string, orders []Order) ([]ImportResult, error)
}

type ReceiptIssueRequest struct {
	IssueType      string `json:"issueType"`
	AffectedUnits  int    `json:"affectedUnits"`
	Note           string `json:"note"`
	IdempotencyKey string `json:"idempotencyKey"`
}

type ReceiptConfirmation struct {
	ReceivedUnits   int                  `json:"receivedUnits"`
	Issue           *ReceiptIssueRequest `json:"issue,omitempty"`
	ConfirmedBy     string               `json:"-"`
	DeliveryRunID   string               `json:"-"`
	DeliveryStopID  string               `json:"-"`
	DeliveryOutcome string               `json:"-"`
	ExpectedUnits   int                  `json:"-"`
}

type Receipt struct {
	ID              string    `json:"id"`
	OrderID         string    `json:"orderId"`
	DeliveryRunID   string    `json:"deliveryRunId"`
	DeliveryStopID  string    `json:"deliveryStopId"`
	DeliveryOutcome string    `json:"deliveryOutcome"`
	ExpectedUnits   int       `json:"expectedUnits"`
	ReceivedUnits   int       `json:"receivedUnits"`
	Status          string    `json:"status"`
	ConfirmedBy     string    `json:"confirmedBy"`
	ConfirmedAt     time.Time `json:"confirmedAt"`
	Version         int       `json:"version"`
}

type ReceiptIssue struct {
	ID             string    `json:"id"`
	ReceiptID      string    `json:"receiptId"`
	IssueType      string    `json:"issueType"`
	AffectedUnits  int       `json:"affectedUnits"`
	Note           string    `json:"note,omitempty"`
	IdempotencyKey string    `json:"idempotencyKey"`
	CreatedBy      string    `json:"createdBy"`
	CreatedAt      time.Time `json:"createdAt"`
}

type ReceiptTask struct {
	Order       Order      `json:"order"`
	Tracking    Tracking   `json:"tracking"`
	ReportBy    *time.Time `json:"reportBy,omitempty"`
	ReportState string     `json:"reportState,omitempty"`
}

// ReceiptDeadline is the store's report-by deadline for an open receipt.
// State is open, due_tomorrow, due_today or overdue.
type ReceiptDeadline struct {
	ReportBy time.Time `json:"reportBy"`
	State    string    `json:"state"`
}

type ReceiptIssueView struct {
	OrderRef string       `json:"orderRef"`
	OutletID string       `json:"outletId"`
	Receipt  Receipt      `json:"receipt"`
	Issue    ReceiptIssue `json:"issue"`
}

// CustodyEvent is an append-only hand-off record for risk-classified Tech orders.
type CustodyEvent struct {
	ID             string    `json:"id"`
	OrderID        string    `json:"orderId"`
	Stage          string    `json:"stage"`
	SealID         string    `json:"sealId"`
	SerialNumbers  []string  `json:"serialNumbers"`
	Condition      string    `json:"condition"`
	EvidenceRef    string    `json:"evidenceRef,omitempty"`
	ReceiverName   string    `json:"receiverName,omitempty"`
	IdempotencyKey string    `json:"idempotencyKey"`
	RecordedBy     string    `json:"recordedBy"`
	RecordedAt     time.Time `json:"recordedAt"`
}

type PlanningTracking struct {
	State                 string         `json:"state"`
	PlanID                string         `json:"planId,omitempty"`
	PlanRef               string         `json:"planRef,omitempty"`
	TripID                string         `json:"tripId,omitempty"`
	Depot                 string         `json:"depot,omitempty"`
	StopSequence          int            `json:"stopSequence,omitempty"`
	ReasonCode            string         `json:"reasonCode,omitempty"`
	ReasonComment         string         `json:"reasonComment,omitempty"`
	ReasonDetail          map[string]any `json:"reasonDetail,omitempty"`
	PlannedArrivalAt      *time.Time     `json:"plannedArrivalAt,omitempty"`
	PlannedServiceStartAt *time.Time     `json:"plannedServiceStartAt,omitempty"`
}

type DeliveryTracking struct {
	RunID                   string         `json:"runId"`
	TripID                  string         `json:"tripId"`
	VehicleID               string         `json:"vehicleId,omitempty"`
	RunStatus               string         `json:"runStatus"`
	StopID                  string         `json:"stopId"`
	Outcome                 string         `json:"outcome,omitempty"`
	Reason                  string         `json:"reason,omitempty"`
	OccurredAt              *time.Time     `json:"occurredAt,omitempty"`
	CompletedAt             *time.Time     `json:"completedAt,omitempty"`
	Proofs                  []ProofSummary `json:"proofs"`
	LoadingShortfallSummary []any          `json:"loadingShortfallSummary"`
	DeliveredUnits          *int           `json:"deliveredUnits,omitempty"`
}

type ProofSummary struct {
	OperationID  string     `json:"operationId,omitempty"`
	Type         string     `json:"type"`
	MimeType     string     `json:"mimeType"`
	UploadedAt   *time.Time `json:"uploadedAt,omitempty"`
	Pending      bool       `json:"pending"`
	ReceiverName string     `json:"receiverName,omitempty"`
}

type Tracking struct {
	Order         Order             `json:"order"`
	Stage         string            `json:"stage"`
	Planning      PlanningTracking  `json:"planning"`
	Delivery      *DeliveryTracking `json:"delivery,omitempty"`
	Receipt       *Receipt          `json:"receipt,omitempty"`
	ReceiptDue    *ReceiptDeadline  `json:"receiptDue,omitempty"`
	ReceiptIssues []ReceiptIssue    `json:"receiptIssues"`
	Custody       []CustodyEvent    `json:"custody"`
}

type PlanningReader interface {
	ByOrder(orderID string) (PlanningTracking, error)
}
type DeliveryReader interface {
	ByOrder(orderID string) (DeliveryTracking, error)
}

type Outlet struct {
	ID    string
	Brand string
}

type ProfileClient interface {
	Resolve(subject, bearer string) (userID string, roles []string, outletIDs []string, err error)
	Outlet(id, bearer string) (Outlet, error)
}

type AuditPublisher interface {
	PublishCreated(correlationID, actorID, orderRef string, state map[string]any) error
}

type PolicyReader interface{ CutoffLocalTime() (string, error) }

type OperatingDay struct {
	Date        string `json:"date"`
	IsOperating bool   `json:"isOperating"`
}
type CalendarReader interface {
	OperatingDays(from, to string) ([]OperatingDay, error)
}
