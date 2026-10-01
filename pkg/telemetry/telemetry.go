package telemetry

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

var (
	HTTPRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Total HTTP requests.",
	}, []string{"service", "method", "path", "status"})

	HTTPRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "HTTP request duration in seconds.",
		Buckets: prometheus.DefBuckets,
	}, []string{"service", "method", "path"})

	HTTPErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "http_errors_total",
		Help: "Total HTTP responses with status >= 400.",
	}, []string{"service", "method", "path", "status"})

	OrdersCreated                 = promauto.NewCounter(prometheus.CounterOpts{Name: "waypoint_orders_created_total", Help: "Orders created."})
	OrdersDeferred                = promauto.NewCounter(prometheus.CounterOpts{Name: "waypoint_orders_deferred_total", Help: "Orders deferred."})
	PlansCreated                  = promauto.NewCounter(prometheus.CounterOpts{Name: "waypoint_plans_created_total", Help: "Plans created."})
	PlansGenerated                = promauto.NewCounter(prometheus.CounterOpts{Name: "waypoint_plans_generated_total", Help: "Suggested allocations generated."})
	OrdersAllocated               = promauto.NewCounter(prometheus.CounterOpts{Name: "waypoint_orders_allocated_total", Help: "Orders allocated to trips."})
	ConstraintFailures            = promauto.NewCounterVec(prometheus.CounterOpts{Name: "waypoint_constraint_failures_total", Help: "Constraint failures by reason."}, []string{"reason"})
	AuditPublishFailures          = promauto.NewCounter(prometheus.CounterOpts{Name: "waypoint_audit_publish_failures_total", Help: "Audit ingest failures."})
	DeliveriesCompleted           = promauto.NewCounter(prometheus.CounterOpts{Name: "waypoint_deliveries_completed_total", Help: "Deliveries completed."})
	DeliveryFailures              = promauto.NewCounter(prometheus.CounterOpts{Name: "waypoint_delivery_failures_total", Help: "Delivery failures."})
	AllocationDuration            = promauto.NewHistogram(prometheus.HistogramOpts{Name: "waypoint_allocation_duration_seconds", Help: "Allocation duration.", Buckets: prometheus.DefBuckets})
	LoadingSessionsStarted        = promauto.NewCounter(prometheus.CounterOpts{Name: "waypoint_loading_sessions_started_total", Help: "Loading sessions started."})
	LoadingOrdersCompleted        = promauto.NewCounter(prometheus.CounterOpts{Name: "waypoint_loading_orders_completed_total", Help: "Orders marked loaded."})
	LoadingShortfalls             = promauto.NewCounterVec(prometheus.CounterOpts{Name: "waypoint_loading_shortfalls_total", Help: "Loading shortfalls by type."}, []string{"type"})
	LoadingTripsReady             = promauto.NewCounter(prometheus.CounterOpts{Name: "waypoint_loading_trips_ready_total", Help: "Trips marked ready for departure."})
	LoadingDuration               = promauto.NewHistogram(prometheus.HistogramOpts{Name: "waypoint_loading_duration_seconds", Help: "Time from start to ready.", Buckets: prometheus.DefBuckets})
	DeliveryRunsStarted           = promauto.NewCounter(prometheus.CounterOpts{Name: "waypoint_delivery_runs_started_total", Help: "Delivery runs started."})
	DeliveryStopsCompleted        = promauto.NewCounter(prometheus.CounterOpts{Name: "waypoint_delivery_stops_completed_total", Help: "Delivery stops completed."})
	DeliveryOutcomes              = promauto.NewCounterVec(prometheus.CounterOpts{Name: "waypoint_delivery_outcomes_total", Help: "Delivery outcomes."}, []string{"outcome"})
	DeliveryProofs                = promauto.NewCounterVec(prometheus.CounterOpts{Name: "waypoint_delivery_proofs_total", Help: "Proofs captured."}, []string{"type"})
	DeliverySyncOps               = promauto.NewCounterVec(prometheus.CounterOpts{Name: "waypoint_delivery_sync_operations_total", Help: "Sync operations."}, []string{"status"})
	DeliverySyncConflicts         = promauto.NewCounter(prometheus.CounterOpts{Name: "waypoint_delivery_sync_conflicts_total", Help: "Sync conflicts."})
	DriverOfflineQueueReports     = promauto.NewCounterVec(prometheus.CounterOpts{Name: "waypoint_driver_offline_queue_reports_total", Help: "Privacy-minimized reports of driver device queue age and count buckets."}, []string{"age_bucket", "count_bucket"})
	DeliveryRunsCompleted         = promauto.NewCounter(prometheus.CounterOpts{Name: "waypoint_delivery_runs_completed_total", Help: "Delivery runs completed."})
	DeliveryRunDuration           = promauto.NewHistogram(prometheus.HistogramOpts{Name: "waypoint_delivery_run_duration_seconds", Help: "Time from start to complete.", Buckets: prometheus.DefBuckets})
	ReceiptConfirmations          = promauto.NewCounter(prometheus.CounterOpts{Name: "waypoint_receipt_confirmations_total", Help: "Store receipt confirmations."})
	ReceiptIssues                 = promauto.NewCounterVec(prometheus.CounterOpts{Name: "waypoint_receipt_issues_total", Help: "Store receipt issues by bounded issue type."}, []string{"issue_type"})
	ReceiptDelay                  = promauto.NewHistogram(prometheus.HistogramOpts{Name: "waypoint_receipt_confirmation_delay_seconds", Help: "Time from driver completion to store receipt confirmation.", Buckets: prometheus.DefBuckets})
	FleetAuditOutboxPending       = promauto.NewGauge(prometheus.GaugeOpts{Name: "waypoint_fleet_audit_outbox_pending", Help: "Undelivered fleet audit records."})
	FleetAuditOutboxOldestSeconds = promauto.NewGauge(prometheus.GaugeOpts{Name: "waypoint_fleet_audit_outbox_oldest_age_seconds", Help: "Age of the oldest undelivered fleet audit record."})
	NotificationEvents            = promauto.NewCounterVec(prometheus.CounterOpts{Name: "waypoint_sms_notifications_total", Help: "SMS notification events by bounded event and state."}, []string{"event", "state"})
	NotificationOutboxPending     = promauto.NewGauge(prometheus.GaugeOpts{Name: "waypoint_sms_outbox_pending", Help: "SMS outbox events pending or being sent."})
	NotificationOutboxOldest      = promauto.NewGauge(prometheus.GaugeOpts{Name: "waypoint_sms_outbox_oldest_age_seconds", Help: "Age of the oldest pending SMS outbox event."})
	NotificationsAwaitingStatus   = promauto.NewGauge(prometheus.GaugeOpts{Name: "waypoint_sms_awaiting_status", Help: "SMS messages accepted by the provider but not yet terminal."})
	NotificationsAwaitingOldest   = promauto.NewGauge(prometheus.GaugeOpts{Name: "waypoint_sms_awaiting_status_oldest_age_seconds", Help: "Age of the oldest accepted SMS without terminal provider status."})
	AgentRequests                 = promauto.NewCounter(prometheus.CounterOpts{Name: "waypoint_agent_requests_total", Help: "Agent orchestrator requests."})
	AgentToolCalls                = promauto.NewCounter(prometheus.CounterOpts{Name: "waypoint_agent_tool_calls_total", Help: "Agent tool invocations."})
	AgentToolCallsByName          = promauto.NewCounterVec(prometheus.CounterOpts{Name: "waypoint_agent_tool_invocations_total", Help: "Agent tool invocations by allowlisted tool name."}, []string{"tool_name"})
	AgentToolFailures             = promauto.NewCounter(prometheus.CounterOpts{Name: "waypoint_agent_tool_failures_total", Help: "Agent tool failures."})
	AgentAuthorizationDenied      = promauto.NewCounter(prometheus.CounterOpts{Name: "waypoint_agent_authorization_denied_total", Help: "Authorization denials."})
	AgentApprovals                = promauto.NewCounterVec(prometheus.CounterOpts{Name: "waypoint_agent_approvals_total", Help: "Human approval decisions by bounded outcome."}, []string{"decision"})
	AgentFailures                 = promauto.NewCounterVec(prometheus.CounterOpts{Name: "waypoint_agent_failures_total", Help: "Agent failures by bounded stage."}, []string{"stage"})
	AgentLatency                  = promauto.NewHistogram(prometheus.HistogramOpts{Name: "waypoint_agent_tool_latency_seconds", Help: "Agent business tool latency.", Buckets: prometheus.DefBuckets})
	AgentRequestLatency           = promauto.NewHistogram(prometheus.HistogramOpts{Name: "waypoint_agent_request_latency_seconds", Help: "Agent chat request latency.", Buckets: prometheus.DefBuckets})
)

func Init(ctx context.Context, serviceName, otlpEndpoint string) (func(context.Context) error, error) {
	if otlpEndpoint == "" {
		return func(context.Context) error { return nil }, nil
	}
	exporter, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(otlpEndpoint), otlptracehttp.WithInsecure())
	if err != nil {
		return nil, err
	}
	res, err := resource.Merge(resource.Default(), resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(serviceName),
	))
	if err != nil {
		return nil, err
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter, sdktrace.WithBatchTimeout(time.Second)),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	return tp.Shutdown, nil
}
