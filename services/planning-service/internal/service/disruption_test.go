package service

import (
	"context"
	"math"
	"strings"
	"testing"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/authorization"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/domain"
)

func TestDisruptionRiskValidation(t *testing.T) {
	profile := &authorization.Profile{UserID: "USR-DISPATCHER"}
	valid := domain.DisruptionRisk{
		DeliveryDate: "2026-10-01", Scope: "DISTRICT", ScopeKey: "Colombo North", RiskType: "HEAVY_RAIN",
		Severity: "MEDIUM", Summary: "Flooding reported near the bridge", Source: "Manual dispatcher report", Confidence: 0.7,
	}
	tests := []struct {
		name    string
		profile *authorization.Profile
		change  func(*domain.DisruptionRisk)
		want    string
	}{
		{name: "unauthenticated", change: func(*domain.DisruptionRisk) {}, want: "forbidden"},
		{name: "invalid date", profile: profile, change: func(r *domain.DisruptionRisk) { r.DeliveryDate = "2026-02-30" }, want: "invalid"},
		{name: "invalid scope", profile: profile, change: func(r *domain.DisruptionRisk) { r.Scope = "COUNTRY" }, want: "invalid"},
		{name: "missing provenance", profile: profile, change: func(r *domain.DisruptionRisk) { r.Source = " " }, want: "invalid"},
		{name: "invalid confidence", profile: profile, change: func(r *domain.DisruptionRisk) { r.Confidence = 1.1 }, want: "invalid"},
		{name: "NaN confidence", profile: profile, change: func(r *domain.DisruptionRisk) { r.Confidence = math.NaN() }, want: "invalid"},
	}
	service := Service{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			risk := valid
			tt.change(&risk)
			_, err := service.CreateDisruptionRisk(context.Background(), tt.profile, risk)
			if err == nil || !strings.HasPrefix(err.Error(), tt.want) {
				t.Fatalf("error=%v, want prefix %q", err, tt.want)
			}
		})
	}
}

func TestDisruptionRiskOverrideRequiresAuditableDecision(t *testing.T) {
	profile := &authorization.Profile{UserID: "USR-DISPATCHER"}
	tests := []struct {
		name, decision, severity, reason, want string
	}{
		{name: "unknown decision", decision: "IGNORE", reason: "checked source details", want: "invalid"},
		{name: "short reason", decision: "DISMISSED", reason: "no", want: "invalid"},
		{name: "override without severity", decision: "OVERRIDE", reason: "checked road authority feed", want: "invalid"},
		{name: "severity on acknowledgement", decision: "ACKNOWLEDGED", severity: "HIGH", reason: "confirmed with local team", want: "invalid"},
	}
	service := Service{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := service.OverrideDisruptionRisk(context.Background(), profile, "risk-1", tt.decision, tt.severity, tt.reason)
			if err == nil || !strings.HasPrefix(err.Error(), tt.want) {
				t.Fatalf("error=%v, want prefix %q", err, tt.want)
			}
		})
	}
}
