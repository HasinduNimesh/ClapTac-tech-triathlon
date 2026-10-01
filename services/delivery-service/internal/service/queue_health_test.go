package service

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/authorization"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/telemetry"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/delivery-service/internal/domain"
)

func TestOfflineQueueHealthCountsOnlyBoundedAssignedDriverBuckets(t *testing.T) {
	profile := &authorization.Profile{UserID: "USR006", Roles: []string{"DRIVER"}, VehicleID: "VEH001"}
	report := domain.OfflineQueueHealth{AgeBucket: "7d_30d", CountBucket: "2_5"}
	metric := telemetry.DriverOfflineQueueReports.WithLabelValues(report.AgeBucket, report.CountBucket)
	before := testutil.ToFloat64(metric)
	if err := (Service{}).ReportOfflineQueueHealth(profile, report); err != nil {
		t.Fatal(err)
	}
	if after := testutil.ToFloat64(metric); after != before+1 {
		t.Fatalf("metric=%v want %v", after, before+1)
	}
	for _, input := range []struct {
		profile *authorization.Profile
		report  domain.OfflineQueueHealth
	}{
		{profile: &authorization.Profile{Roles: []string{"DRIVER"}}, report: report},
		{profile: &authorization.Profile{Roles: []string{"DISPATCHER"}, VehicleID: "VEH001"}, report: report},
		{profile: profile, report: domain.OfflineQueueHealth{AgeBucket: "trip-TRIP1", CountBucket: "2_5"}},
		{profile: profile, report: domain.OfflineQueueHealth{AgeBucket: "7d_30d", CountBucket: "exactly-4"}},
		{profile: profile, report: domain.OfflineQueueHealth{AgeBucket: "30d_plus", CountBucket: "none"}},
	} {
		if err := (Service{}).ReportOfflineQueueHealth(input.profile, input.report); err == nil {
			t.Fatalf("accepted unauthorized or unbounded report: profile=%+v report=%+v", input.profile, input.report)
		}
	}
	if after := testutil.ToFloat64(metric); after != before+1 {
		t.Fatalf("invalid reports changed metric: got=%v want=%v", after, before+1)
	}
}
