package prometheus

import (
	"os"
	"strings"
	"testing"
)

func TestRepeatedDeliverySyncConflictAlertUsesInstrumentedMetric(t *testing.T) {
	const metric = "waypoint_delivery_sync_conflicts_total"
	alerts, err := os.ReadFile("alerts.yml")
	if err != nil {
		t.Fatal(err)
	}
	telemetry, err := os.ReadFile("../../../pkg/telemetry/telemetry.go")
	if err != nil {
		t.Fatal(err)
	}
	delivery, err := os.ReadFile("../../../services/delivery-service/internal/service/service.go")
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(alerts), "alert: WaypointRepeatedDeliverySyncConflicts") ||
		!strings.Contains(string(alerts), "increase("+metric+"[10m]) >= 3") {
		t.Fatalf("alert does not query the expected conflict metric")
	}
	if !strings.Contains(string(telemetry), `Name: "`+metric+`"`) {
		t.Fatalf("%s is not declared by the Go telemetry registry", metric)
	}
	recordStart := strings.Index(string(delivery), "func (s Service) record(")
	if recordStart < 0 {
		t.Fatal("delivery sync record function is missing")
	}
	recordEnd := strings.Index(string(delivery)[recordStart:], "\nfunc (s Service) finishRejected(")
	if recordEnd < 0 {
		t.Fatal("could not isolate delivery sync record function")
	}
	recordFunction := string(delivery)[recordStart : recordStart+recordEnd]
	if !strings.Contains(recordFunction, "telemetry.DeliverySyncConflicts.Inc()") {
		t.Fatalf("%s is declared but has no delivery-service increment", metric)
	}
	if strings.Contains(string(alerts), "waypoint_offline_sync_failures_total") {
		t.Fatal("alert references an unobservable client-side network failure counter")
	}
}

func TestDriverQueueAlertUsesOnlinePrivacyMinimizedMetric(t *testing.T) {
	const metric = "waypoint_driver_offline_queue_reports_total"
	alerts, err := os.ReadFile("alerts.yml")
	if err != nil {
		t.Fatal(err)
	}
	telemetry, err := os.ReadFile("../../../pkg/telemetry/telemetry.go")
	if err != nil {
		t.Fatal(err)
	}
	service, err := os.ReadFile("../../../services/delivery-service/internal/service/service.go")
	if err != nil {
		t.Fatal(err)
	}
	handler, err := os.ReadFile("../../../services/delivery-service/internal/handler/handler.go")
	if err != nil {
		t.Fatal(err)
	}
	client, err := os.ReadFile("../../../apps/web/src/driver/DriverTripsPage.tsx")
	if err != nil {
		t.Fatal(err)
	}
	helper, err := os.ReadFile("../../../apps/web/src/offline/queueHealth.mjs")
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(alerts), "alert: WaypointDriverQueueStale") ||
		!strings.Contains(string(alerts), "increase("+metric+`{age_bucket=~"7d_30d|30d_plus"}[20m]) > 0`) ||
		!strings.Contains(string(alerts), `alert: WaypointDriverQueueCritical`) {
		t.Fatal("driver queue alerts do not query the long-lived queue buckets")
	}
	if !strings.Contains(string(telemetry), `Name: "`+metric+`"`) ||
		!strings.Contains(string(service), "DriverOfflineQueueReports.WithLabelValues(report.AgeBucket, report.CountBucket).Inc()") {
		t.Fatal("driver queue metric is not declared and incremented by the service")
	}
	if !strings.Contains(string(handler), `Post("/telemetry/offline-queue", h.offlineQueueHealth)`) ||
		!strings.Contains(string(client), `"/delivery/telemetry/offline-queue"`) ||
		!strings.Contains(string(client), "offlineQueueHealth(await listQueue(ownerId))") {
		t.Fatal("driver online reporting path is disconnected")
	}
	if strings.Contains(string(helper), "tripId:") || strings.Contains(string(helper), "stopId:") || strings.Contains(string(helper), "operationId:") {
		t.Fatal("offline queue telemetry helper contains record identifiers")
	}
}
