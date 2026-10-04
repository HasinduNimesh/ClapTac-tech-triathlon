package audit

import (
	"context"
	"log/slog"
	"time"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/httpx"
)

const (
	ActionOrderCreated                 = "order.created"
	ActionOrderDeferred                = "ORDER_DEFERRED"
	ActionAllocationChanged            = "allocation.changed"
	ActionPlanCreated                  = "PLAN_CREATED"
	ActionPlanGenerated                = "PLAN_GENERATED"
	ActionOrderAllocated               = "ORDER_ALLOCATED"
	ActionOrderReallocated             = "ORDER_REALLOCATED"
	ActionAllocationRemoved            = "ALLOCATION_REMOVED"
	ActionPlanConfirmed                = "PLAN_CONFIRMED"
	ActionPlanAckReminderSent          = "PLAN_ACK_REMINDER_SENT"
	ActionDisruptionRiskCreated        = "DISRUPTION_RISK_CREATED"
	ActionDisruptionRiskOverridden     = "DISRUPTION_RISK_OVERRIDDEN"
	ActionVehicleReassigned            = "vehicle.reassigned"
	ActionLoadingIssueReported         = "loading.issue_reported"
	ActionLoadingStarted               = "LOADING_STARTED"
	ActionOrderLoaded                  = "ORDER_LOADED"
	ActionLoadingShortfallRecorded     = "LOADING_SHORTFALL_RECORDED"
	ActionLoadingShortfallUpdated      = "LOADING_SHORTFALL_UPDATED"
	ActionLoadingShortfallRemoved      = "LOADING_SHORTFALL_REMOVED"
	ActionLoadingShortfallDecided      = "LOADING_SHORTFALL_DECIDED"
	ActionTripReadyForDeparture        = "TRIP_READY_FOR_DEPARTURE"
	ActionLoadingPlanVersionSynced     = "LOADING_PLAN_VERSION_SYNCED"
	ActionLoadingShortfallPhotoAdded   = "LOADING_SHORTFALL_PHOTO_ADDED"
	ActionLoadingDockAlertRaised       = "LOADING_DOCK_ALERT_RAISED"
	ActionLoadingDockAlertResolved     = "LOADING_DOCK_ALERT_RESOLVED"
	ActionDeliveryRunPrepared          = "DELIVERY_RUN_PREPARED"
	ActionDeliveryRunStarted           = "DELIVERY_RUN_STARTED"
	ActionDeliveryStopArrived          = "DELIVERY_STOP_ARRIVED"
	ActionDeliveryOutcomeRecorded      = "DELIVERY_OUTCOME_RECORDED"
	ActionDeliveryProofCaptured        = "DELIVERY_PROOF_CAPTURED"
	ActionDeliverySyncApplied          = "DELIVERY_SYNC_APPLIED"
	ActionDeliverySyncConflict         = "DELIVERY_SYNC_CONFLICT"
	ActionDeliveryTemperatureRecorded  = "DELIVERY_TEMPERATURE_RECORDED"
	ActionDeliveryTemperatureException = "DELIVERY_TEMPERATURE_EXCEPTION"
	ActionDeliveryIncidentReported     = "DELIVERY_INCIDENT_REPORTED"
	ActionDeliveryRunCompleted         = "DELIVERY_RUN_COMPLETED"
	ActionTripStarted                  = "trip.started"
	ActionDeliveryOutcomeChanged       = "delivery.outcome_changed"
	ActionReceiptConfirmed             = "receipt.confirmed"
	ActionAgentToolInvoked             = "agent.tool_invoked"
	ActionAgentActionApproved          = "agent.action_approved"
	ActionAgentActionRejected          = "agent.action_rejected"
	ActionAgentChat                    = "AGENT_CHAT"
	ActionAgentToolProposed            = "AGENT_TOOL_PROPOSED"
	ActionAgentToolInvokedM7           = "AGENT_TOOL_INVOKED"
	ActionAgentApprovalCreated         = "AGENT_APPROVAL_CREATED"
	ActionAgentApprovalApproved        = "AGENT_APPROVAL_APPROVED"
	ActionAgentApprovalRejected        = "AGENT_APPROVAL_REJECTED"
	ActionAgentToolFailed              = "AGENT_TOOL_FAILED"
)

type Event struct {
	EventID       string         `json:"event_id"`
	CorrelationID string         `json:"correlation_id"`
	ActorID       string         `json:"actor_id"`
	ActorType     string         `json:"actor_type"`
	Action        string         `json:"action"`
	ResourceType  string         `json:"resource_type"`
	ResourceID    string         `json:"resource_id"`
	PreviousState map[string]any `json:"previous_state,omitempty"`
	NewState      map[string]any `json:"new_state,omitempty"`
	Reason        string         `json:"reason,omitempty"`
	Timestamp     time.Time      `json:"timestamp"`
	Source        string         `json:"source"`
}

type Publisher interface {
	Publish(ctx context.Context, event Event) error
}

type LogPublisher struct {
	Logger *slog.Logger
	Source string
}

func (p LogPublisher) Publish(ctx context.Context, event Event) error {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}
	if event.CorrelationID == "" {
		event.CorrelationID = httpx.CorrelationIDFrom(ctx)
	}
	if event.Source == "" {
		event.Source = p.Source
	}
	logger := p.Logger
	if logger == nil {
		logger = slog.Default()
	}
	logger.Info("audit_event",
		"event_id", event.EventID,
		"correlation_id", event.CorrelationID,
		"actor_id", event.ActorID,
		"actor_type", event.ActorType,
		"action", event.Action,
		"resource_type", event.ResourceType,
		"resource_id", event.ResourceID,
		"reason", event.Reason,
		"source", event.Source,
	)
	return nil
}
