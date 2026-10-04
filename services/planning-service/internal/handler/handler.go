package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/auth"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/authorization"
	apierrors "github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/errors"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/httpx"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/domain"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/service"
)

type Planner interface {
	Create(ctx context.Context, profile *authorization.Profile, date string) (domain.Plan, bool, error)
	Get(ctx context.Context, id string) (map[string]any, error)
	GetByDate(ctx context.Context, date string) (map[string]any, error)
	Generate(ctx context.Context, profile *authorization.Profile, id string) (domain.GenerateResult, error)
	Simulate(ctx context.Context, id string) (domain.GenerateResult, error)
	Reset(ctx context.Context, profile *authorization.Profile, id string) error
	Assign(ctx context.Context, profile *authorization.Profile, planID, orderID, vehicleID string, tripNo int, reason string) (domain.Allocation, []domain.Result, error)
	Reassign(ctx context.Context, profile *authorization.Profile, planID, allocID, vehicleID string, tripNo int, reason string) ([]domain.Result, error)
	Remove(ctx context.Context, profile *authorization.Profile, planID, allocID string) error
	Defer(ctx context.Context, profile *authorization.Profile, planID, orderID, code, comment, nextRunTarget string) error
	Confirm(ctx context.Context, profile *authorization.Profile, id string) error
	InternalTrips(ctx context.Context, date, depot string) ([]domain.InternalTrip, error)
	InternalTrip(ctx context.Context, tripID string) (domain.InternalTrip, error)
	InternalOrder(ctx context.Context, orderID string) (domain.OrderTracking, error)
	Acknowledge(ctx context.Context, profile *authorization.Profile, planID string, version int, tripID string) error
	Remind(ctx context.Context, profile *authorization.Profile, planID, tripID, audience string) (domain.ReminderResult, error)
	Reminders(ctx context.Context, profile *authorization.Profile, planID, tripID string) ([]domain.PlanReminder, error)
	Revise(ctx context.Context, profile *authorization.Profile, planID string) error
	BreakdownProposals(ctx context.Context, planID, vehicleID string) (map[string]any, error)
	ConfirmBreakdown(ctx context.Context, profile *authorization.Profile, planID, sourceVehicle, targetVehicle string, tripNo int) (map[string]any, error)
	ListDisruptionRisks(ctx context.Context, date string) ([]domain.DisruptionRisk, error)
	CreateDisruptionRisk(ctx context.Context, profile *authorization.Profile, risk domain.DisruptionRisk) (domain.DisruptionRisk, error)
	OverrideDisruptionRisk(ctx context.Context, profile *authorization.Profile, riskID, decision, severity, reason string) (domain.DisruptionRisk, error)
}

type Handler struct {
	Authn    auth.Authenticator
	Profiles authorization.ProfileResolver
	Service  Planner
}

var _ Planner = service.Service{}

func (h Handler) Routes(r chi.Router) {
	r.Route("/api/v1/planning", func(r chi.Router) {
		r.With(authorization.RequireWith(h.Authn, h.Profiles, authorization.PermPlanCreate)).Post("/plans", h.create)
		r.With(authorization.RequireWith(h.Authn, h.Profiles, authorization.PermPlanView)).Get("/plans", h.getByDate)
		r.With(authorization.RequireWith(h.Authn, h.Profiles, authorization.PermPlanView)).Get("/plans/{id}", h.get)
		r.With(authorization.RequireWith(h.Authn, h.Profiles, authorization.PermPlanUpdate)).Post("/plans/{id}/generate", h.generate)
		r.With(authorization.RequireWith(h.Authn, h.Profiles, authorization.PermPlanView)).Post("/plans/{id}/simulate", h.simulate)
		r.With(authorization.RequireWith(h.Authn, h.Profiles, authorization.PermPlanUpdate)).Post("/plans/{id}/reset", h.reset)
		r.With(authorization.RequireWith(h.Authn, h.Profiles, authorization.PermAllocationCreate)).Post("/plans/{id}/allocations", h.assign)
		r.With(authorization.RequireWith(h.Authn, h.Profiles, authorization.PermAllocationUpdate)).Put("/plans/{id}/allocations/{allocId}", h.reassign)
		r.With(authorization.RequireWith(h.Authn, h.Profiles, authorization.PermAllocationUpdate)).Delete("/plans/{id}/allocations/{allocId}", h.remove)
		r.With(authorization.RequireWith(h.Authn, h.Profiles, authorization.PermOrderDefer)).Post("/plans/{id}/deferrals", h.deferOrder)
		r.With(authorization.RequireWith(h.Authn, h.Profiles, authorization.PermPlanUpdate)).Post("/plans/{id}/confirm", h.confirm)
		r.With(authorization.RequireWith(h.Authn, h.Profiles, authorization.PermPlanUpdate)).Post("/plans/{id}/revise", h.revise)
		r.With(authorization.RequireWith(h.Authn, h.Profiles, authorization.PermPlanView)).Get("/plans/{id}/breakdowns/proposals", h.breakdownProposals)
		r.With(authorization.RequireWith(h.Authn, h.Profiles, authorization.PermPlanUpdate)).Post("/plans/{id}/breakdowns/reassign", h.confirmBreakdown)
		r.With(authorization.RequireWith(h.Authn, h.Profiles, authorization.PermPlanAcknowledge)).Post("/plans/{id}/acknowledgements", h.acknowledge)
		r.With(authorization.RequireWith(h.Authn, h.Profiles, authorization.PermPlanUpdate)).Post("/plans/{id}/reminders", h.remind)
		r.With(authorization.RequireWith(h.Authn, h.Profiles, authorization.PermPlanAcknowledge)).Get("/plans/{id}/reminders", h.reminders)
		r.With(authorization.RequireWith(h.Authn, h.Profiles, authorization.PermPlanView)).Get("/disruption-risks", h.listDisruptionRisks)
		r.With(authorization.RequireWith(h.Authn, h.Profiles, authorization.PermPlanUpdate)).Post("/disruption-risks", h.createDisruptionRisk)
		r.With(authorization.RequireWith(h.Authn, h.Profiles, authorization.PermPlanUpdate)).Post("/disruption-risks/{riskId}/override", h.overrideDisruptionRisk)
		r.With(authorization.RequireAnyWith(h.Authn, h.Profiles, authorization.PermPlanView, authorization.PermPlansReadInternal)).Get("/internal/trips", h.internalTrips)
		r.With(authorization.RequireAnyWith(h.Authn, h.Profiles, authorization.PermPlanView, authorization.PermPlansReadInternal)).Get("/internal/trips/{tripId}", h.internalTrip)
		r.With(authorization.RequireWith(h.Authn, h.Profiles, authorization.PermPlansReadInternal)).Get("/internal/orders/{orderId}", h.internalOrder)
	})
}

func (h Handler) listDisruptionRisks(w http.ResponseWriter, r *http.Request) {
	items, err := h.Service.ListDisruptionRisks(r.Context(), r.URL.Query().Get("date"))
	if writeErr(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h Handler) createDisruptionRisk(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DeliveryDate    string   `json:"deliveryDate"`
		Scope           string   `json:"scope"`
		ScopeKey        string   `json:"scopeKey"`
		RiskType        string   `json:"riskType"`
		Severity        string   `json:"severity"`
		Summary         string   `json:"summary"`
		Source          string   `json:"source"`
		SourceReference string   `json:"sourceReference"`
		Confidence      *float64 `json:"confidence"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Confidence == nil {
		apierrors.BadRequest(w, "valid disruption risk and confidence are required")
		return
	}
	created, err := h.Service.CreateDisruptionRisk(r.Context(), h.profile(r), domain.DisruptionRisk{
		DeliveryDate: body.DeliveryDate, Scope: body.Scope, ScopeKey: body.ScopeKey, RiskType: body.RiskType,
		Severity: body.Severity, Summary: body.Summary, Source: body.Source, SourceReference: body.SourceReference, Confidence: *body.Confidence,
	})
	if writeErr(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, created)
}

func (h Handler) overrideDisruptionRisk(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Decision string `json:"decision"`
		Severity string `json:"severity"`
		Reason   string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierrors.BadRequest(w, "valid override decision and reason are required")
		return
	}
	updated, err := h.Service.OverrideDisruptionRisk(r.Context(), h.profile(r), chi.URLParam(r, "riskId"), body.Decision, body.Severity, body.Reason)
	if writeErr(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, updated)
}

func (h Handler) confirmBreakdown(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SourceVehicleID      string `json:"sourceVehicleId"`
		ReplacementVehicleID string `json:"replacementVehicleId"`
		TripNumber           int    `json:"tripNumber"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.SourceVehicleID == "" || body.ReplacementVehicleID == "" || body.TripNumber < 1 || body.TripNumber > 2 {
		apierrors.BadRequest(w, "source, replacement, and valid trip number are required")
		return
	}
	out, err := h.Service.ConfirmBreakdown(r.Context(), h.profile(r), chi.URLParam(r, "id"), body.SourceVehicleID, body.ReplacementVehicleID, body.TripNumber)
	if writeErr(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h Handler) breakdownProposals(w http.ResponseWriter, r *http.Request) {
	vehicle := strings.TrimSpace(r.URL.Query().Get("vehicleId"))
	if vehicle == "" {
		apierrors.BadRequest(w, "vehicleId is required")
		return
	}
	out, err := h.Service.BreakdownProposals(r.Context(), chi.URLParam(r, "id"), vehicle)
	if writeErr(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h Handler) revise(w http.ResponseWriter, r *http.Request) {
	if writeErr(w, h.Service.Revise(r.Context(), h.profile(r), chi.URLParam(r, "id"))) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "revision_open"})
}

func (h Handler) acknowledge(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Version int    `json:"version"`
		TripID  string `json:"tripId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Version < 1 {
		apierrors.BadRequest(w, "valid version is required")
		return
	}
	if err := h.Service.Acknowledge(r.Context(), h.profile(r), chi.URLParam(r, "id"), body.Version, body.TripID); writeErr(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "acknowledged", "version": body.Version, "tripId": body.TripID})
}

func (h Handler) remind(w http.ResponseWriter, r *http.Request) {
	var body struct {
		TripID   string `json:"tripId"`
		Audience string `json:"audience"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierrors.BadRequest(w, "tripId and audience are required")
		return
	}
	result, err := h.Service.Remind(r.Context(), h.profile(r), chi.URLParam(r, "id"), body.TripID, body.Audience)
	if writeErr(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, result)
}

func (h Handler) reminders(w http.ResponseWriter, r *http.Request) {
	items, err := h.Service.Reminders(r.Context(), h.profile(r), chi.URLParam(r, "id"), r.URL.Query().Get("tripId"))
	if writeErr(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h Handler) profile(r *http.Request) *authorization.Profile {
	p, _ := authorization.ProfileFrom(r.Context())
	return p
}

func (h Handler) create(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DeliveryDate string `json:"deliveryDate"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierrors.BadRequest(w, "invalid JSON")
		return
	}
	pl, created, err := h.Service.Create(r.Context(), h.profile(r), body.DeliveryDate)
	if writeErr(w, err) {
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	httpx.WriteJSON(w, status, map[string]any{"plan": pl})
}

func (h Handler) getByDate(w http.ResponseWriter, r *http.Request) {
	date := r.URL.Query().Get("date")
	if date == "" {
		apierrors.BadRequest(w, "date is required")
		return
	}
	detail, err := h.Service.GetByDate(r.Context(), date)
	if writeErr(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, detail)
}

func (h Handler) get(w http.ResponseWriter, r *http.Request) {
	detail, err := h.Service.Get(r.Context(), chi.URLParam(r, "id"))
	if writeErr(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, detail)
}

func (h Handler) generate(w http.ResponseWriter, r *http.Request) {
	out, err := h.Service.Generate(r.Context(), h.profile(r), chi.URLParam(r, "id"))
	if writeErr(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h Handler) simulate(w http.ResponseWriter, r *http.Request) {
	out, err := h.Service.Simulate(r.Context(), chi.URLParam(r, "id"))
	if writeErr(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h Handler) reset(w http.ResponseWriter, r *http.Request) {
	if writeErr(w, h.Service.Reset(r.Context(), h.profile(r), chi.URLParam(r, "id"))) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "reset"})
}

func (h Handler) assign(w http.ResponseWriter, r *http.Request) {
	var body struct {
		OrderID    string `json:"orderId"`
		VehicleID  string `json:"vehicleId"`
		TripNumber int    `json:"tripNumber"`
		Reason     string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierrors.BadRequest(w, "invalid JSON")
		return
	}
	alloc, fails, err := h.Service.Assign(r.Context(), h.profile(r), chi.URLParam(r, "id"), body.OrderID, body.VehicleID, body.TripNumber, body.Reason)
	if writeAllocErr(w, err, fails) {
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"allocation": alloc})
}

func (h Handler) reassign(w http.ResponseWriter, r *http.Request) {
	var body struct {
		VehicleID  string `json:"vehicleId"`
		TripNumber int    `json:"tripNumber"`
		Reason     string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierrors.BadRequest(w, "invalid JSON")
		return
	}
	fails, err := h.Service.Reassign(r.Context(), h.profile(r), chi.URLParam(r, "id"), chi.URLParam(r, "allocId"), body.VehicleID, body.TripNumber, body.Reason)
	if writeAllocErr(w, err, fails) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "reassigned"})
}

func (h Handler) remove(w http.ResponseWriter, r *http.Request) {
	if writeErr(w, h.Service.Remove(r.Context(), h.profile(r), chi.URLParam(r, "id"), chi.URLParam(r, "allocId"))) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h Handler) deferOrder(w http.ResponseWriter, r *http.Request) {
	var body struct {
		OrderID       string `json:"orderId"`
		ReasonCode    string `json:"reasonCode"`
		Comment       string `json:"comment"`
		NextRunTarget string `json:"nextRunTarget"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierrors.BadRequest(w, "invalid JSON")
		return
	}
	if writeErr(w, h.Service.Defer(r.Context(), h.profile(r), chi.URLParam(r, "id"), body.OrderID, body.ReasonCode, body.Comment, body.NextRunTarget)) {
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"status": "deferred"})
}

func (h Handler) internalTrips(w http.ResponseWriter, r *http.Request) {
	items, err := h.Service.InternalTrips(r.Context(), r.URL.Query().Get("date"), r.URL.Query().Get("depot"))
	if writeErr(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h Handler) internalTrip(w http.ResponseWriter, r *http.Request) {
	item, err := h.Service.InternalTrip(r.Context(), chi.URLParam(r, "tripId"))
	if writeErr(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"trip": item})
}

func (h Handler) internalOrder(w http.ResponseWriter, r *http.Request) {
	item, err := h.Service.InternalOrder(r.Context(), chi.URLParam(r, "orderId"))
	if writeErr(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"tracking": item})
}

func (h Handler) confirm(w http.ResponseWriter, r *http.Request) {
	if writeErr(w, h.Service.Confirm(r.Context(), h.profile(r), chi.URLParam(r, "id"))) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "confirmed"})
}

func writeAllocErr(w http.ResponseWriter, err error, fails []domain.Result) bool {
	if err == nil {
		return false
	}
	if strings.Contains(err.Error(), "allocation_invalid") {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"type": "about:blank", "title": "Conflict", "status": 409,
			"detail": "allocation rejected by constraint engine", "failures": fails,
		})
		return true
	}
	return writeErr(w, err)
}

func writeErr(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	switch {
	case strings.HasPrefix(msg, "invalid"):
		apierrors.BadRequest(w, msg)
	case strings.HasPrefix(msg, "not found"):
		apierrors.NotFound(w, msg)
	case strings.HasPrefix(msg, "conflict"):
		apierrors.Conflict(w, msg)
	case strings.HasPrefix(msg, "stale_version"):
		apierrors.Conflict(w, "plan version is stale; reload and acknowledge the current version")
	case strings.HasPrefix(msg, "forbidden: "):
		apierrors.Forbidden(w, strings.TrimPrefix(msg, "forbidden: "))
	case strings.HasPrefix(msg, "forbidden"):
		apierrors.Forbidden(w, "acknowledgement requires an authenticated field role")
	default:
		apierrors.Internal(w, "unexpected error")
	}
	return true
}
