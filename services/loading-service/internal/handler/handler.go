package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/auth"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/authorization"
	apierrors "github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/errors"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/httpx"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/loading-service/internal/domain"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/loading-service/internal/service"
)

type Loader interface {
	List(ctx context.Context, profile *authorization.Profile, date string) ([]map[string]any, error)
	Get(ctx context.Context, profile *authorization.Profile, tripID string) (map[string]any, error)
	Start(ctx context.Context, profile *authorization.Profile, tripID string, expectedPlanVersion int) (map[string]any, error)
	MarkLoaded(ctx context.Context, profile *authorization.Profile, tripID, orderID string) error
	CreateIssue(ctx context.Context, profile *authorization.Profile, tripID, orderID, typ, note, key string, units int) (domain.Issue, error)
	UpdateIssue(ctx context.Context, profile *authorization.Profile, tripID, orderID, issueID, typ, note string, units int) error
	DeleteIssue(ctx context.Context, profile *authorization.Profile, tripID, orderID, issueID string) error
	DecideIssue(ctx context.Context, profile *authorization.Profile, tripID, orderID, issueID, decision, note string) (domain.Issue, error)
	Ready(ctx context.Context, profile *authorization.Profile, tripID string, checks service.ReadyChecks) (map[string]any, error)
	SyncPlanVersion(ctx context.Context, profile *authorization.Profile, tripID string, expectedPlanVersion int) (map[string]any, error)
	AttachIssuePhoto(ctx context.Context, profile *authorization.Profile, tripID, orderID, issueID, mime string, body []byte) (domain.Issue, error)
	IssuePhoto(ctx context.Context, profile *authorization.Profile, tripID, orderID, issueID string) (io.ReadCloser, string, error)
	MarkIssueSeen(ctx context.Context, profile *authorization.Profile, tripID, orderID, issueID string) error
	RaiseDockAlert(ctx context.Context, profile *authorization.Profile, tripID, typ, orderRef, belongsVehicleID, note, key string) (domain.DockAlert, error)
	ListDockAlerts(ctx context.Context, profile *authorization.Profile, date string) ([]domain.DockAlert, error)
	ResolveDockAlert(ctx context.Context, profile *authorization.Profile, id string) (domain.DockAlert, error)
	InternalList(ctx context.Context, date, vehicleID string) ([]map[string]any, error)
	InternalGet(ctx context.Context, tripID string) (map[string]any, error)
}

type Handler struct {
	Authn    auth.Authenticator
	Profiles authorization.ProfileResolver
	Service  Loader
}

var _ Loader = service.Service{}

func (h Handler) Routes(r chi.Router) {
	view := authorization.RequireAnyWith(h.Authn, h.Profiles, authorization.PermLoadingView, authorization.PermLoadingViewAll)
	update := authorization.RequireWith(h.Authn, h.Profiles, authorization.PermLoadingUpdate)
	issue := authorization.RequireWith(h.Authn, h.Profiles, authorization.PermLoadingIssue)
	ready := authorization.RequireWith(h.Authn, h.Profiles, authorization.PermLoadingReady)
	decide := authorization.RequireWith(h.Authn, h.Profiles, authorization.PermLoadingDecide)

	r.Route("/api/v1/loading", func(r chi.Router) {
		r.With(view).Get("/trips", h.list)
		r.With(view).Get("/trips/{tripId}", h.get)
		r.With(update).Post("/trips/{tripId}/start", h.start)
		r.With(update).Post("/trips/{tripId}/sync", h.sync)
		r.With(update).Put("/trips/{tripId}/orders/{orderId}/loaded", h.loaded)
		r.With(issue).Post("/trips/{tripId}/orders/{orderId}/issues", h.createIssue)
		r.With(issue).Put("/trips/{tripId}/orders/{orderId}/issues/{issueId}", h.updateIssue)
		r.With(issue).Delete("/trips/{tripId}/orders/{orderId}/issues/{issueId}", h.deleteIssue)
		r.With(issue).Post("/trips/{tripId}/orders/{orderId}/issues/{issueId}/photo", h.issuePhotoUpload)
		r.With(view).Get("/trips/{tripId}/orders/{orderId}/issues/{issueId}/photo", h.issuePhoto)
		r.With(decide).Post("/trips/{tripId}/orders/{orderId}/issues/{issueId}/seen", h.issueSeen)
		r.With(decide).Post("/trips/{tripId}/orders/{orderId}/issues/{issueId}/decision", h.decideIssue)
		r.With(issue).Post("/trips/{tripId}/alerts", h.raiseAlert)
		r.With(view).Get("/alerts", h.listAlerts)
		r.With(decide).Post("/alerts/{alertId}/resolve", h.resolveAlert)
		r.With(ready).Post("/trips/{tripId}/ready", h.ready)
		internal := authorization.RequireAnyWith(h.Authn, h.Profiles, authorization.PermLoadingViewAll, authorization.PermLoadingReadInternal)
		r.With(internal).Get("/internal/trips", h.internalList)
		r.With(internal).Get("/internal/trips/{tripId}", h.internalGet)
	})
}

func (h Handler) profile(r *http.Request) *authorization.Profile {
	p, _ := authorization.ProfileFrom(r.Context())
	return p
}

func (h Handler) list(w http.ResponseWriter, r *http.Request) {
	date := r.URL.Query().Get("date")
	if date == "" {
		apierrors.BadRequest(w, "date is required")
		return
	}
	items, err := h.Service.List(r.Context(), h.profile(r), date)
	if writeErr(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h Handler) get(w http.ResponseWriter, r *http.Request) {
	detail, err := h.Service.Get(r.Context(), h.profile(r), chi.URLParam(r, "tripId"))
	if writeErr(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, detail)
}

func (h Handler) start(w http.ResponseWriter, r *http.Request) {
	expectedPlanVersion := 0
	if raw := r.Header.Get("If-Match"); raw != "" {
		parsed, err := strconv.Atoi(strings.Trim(raw, `"`))
		if err != nil || parsed < 1 {
			apierrors.BadRequest(w, "If-Match must contain the positive plan version used to load the manifest")
			return
		}
		expectedPlanVersion = parsed
	}
	detail, err := h.Service.Start(r.Context(), h.profile(r), chi.URLParam(r, "tripId"), expectedPlanVersion)
	if writeErr(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, detail)
}

func (h Handler) loaded(w http.ResponseWriter, r *http.Request) {
	if writeErr(w, h.Service.MarkLoaded(r.Context(), h.profile(r), chi.URLParam(r, "tripId"), chi.URLParam(r, "orderId"))) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "loaded"})
}

func (h Handler) createIssue(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Type          string `json:"type"`
		AffectedUnits int    `json:"affectedUnits"`
		Note          string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierrors.BadRequest(w, "invalid JSON")
		return
	}
	iss, err := h.Service.CreateIssue(r.Context(), h.profile(r), chi.URLParam(r, "tripId"), chi.URLParam(r, "orderId"), body.Type, body.Note, r.Header.Get("Idempotency-Key"), body.AffectedUnits)
	if writeErr(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"issue": iss})
}

func (h Handler) updateIssue(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Type          string `json:"type"`
		AffectedUnits int    `json:"affectedUnits"`
		Note          string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierrors.BadRequest(w, "invalid JSON")
		return
	}
	if writeErr(w, h.Service.UpdateIssue(r.Context(), h.profile(r), chi.URLParam(r, "tripId"), chi.URLParam(r, "orderId"), chi.URLParam(r, "issueId"), body.Type, body.Note, body.AffectedUnits)) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "updated"})
}

func (h Handler) deleteIssue(w http.ResponseWriter, r *http.Request) {
	if writeErr(w, h.Service.DeleteIssue(r.Context(), h.profile(r), chi.URLParam(r, "tripId"), chi.URLParam(r, "orderId"), chi.URLParam(r, "issueId"))) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "removed"})
}

func (h Handler) decideIssue(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Decision string `json:"decision"`
		Note     string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierrors.BadRequest(w, "invalid JSON")
		return
	}
	iss, err := h.Service.DecideIssue(r.Context(), h.profile(r), chi.URLParam(r, "tripId"), chi.URLParam(r, "orderId"), chi.URLParam(r, "issueId"), body.Decision, body.Note)
	if writeErr(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"issue": iss})
}

func (h Handler) ready(w http.ResponseWriter, r *http.Request) {
	// The body is optional: the loader app sends its departure checks.
	var body struct {
		ChilledTemperatureC *float64 `json:"chilledTemperatureC"`
		SealNumber          string   `json:"sealNumber"`
	}
	if r.ContentLength != 0 {
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
			apierrors.BadRequest(w, "invalid JSON")
			return
		}
	}
	detail, err := h.Service.Ready(r.Context(), h.profile(r), chi.URLParam(r, "tripId"), service.ReadyChecks{ChilledTemperatureC: body.ChilledTemperatureC, SealNumber: body.SealNumber})
	if writeErr(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, detail)
}

func (h Handler) sync(w http.ResponseWriter, r *http.Request) {
	expected := 0
	if raw := r.Header.Get("If-Match"); raw != "" {
		parsed, err := strconv.Atoi(strings.Trim(raw, `"`))
		if err != nil || parsed < 1 {
			apierrors.BadRequest(w, "If-Match must contain the positive plan version being acknowledged")
			return
		}
		expected = parsed
	}
	detail, err := h.Service.SyncPlanVersion(r.Context(), h.profile(r), chi.URLParam(r, "tripId"), expected)
	if writeErr(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, detail)
}

func (h Handler) issuePhotoUpload(w http.ResponseWriter, r *http.Request) {
	body, mime, ok := httpx.ReadImageUpload(w, r, service.MaxPhotoBytes)
	if !ok {
		return
	}
	iss, err := h.Service.AttachIssuePhoto(r.Context(), h.profile(r), chi.URLParam(r, "tripId"), chi.URLParam(r, "orderId"), chi.URLParam(r, "issueId"), mime, body)
	if writeErr(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"issue": iss})
}

func (h Handler) issuePhoto(w http.ResponseWriter, r *http.Request) {
	photo, mime, err := h.Service.IssuePhoto(r.Context(), h.profile(r), chi.URLParam(r, "tripId"), chi.URLParam(r, "orderId"), chi.URLParam(r, "issueId"))
	if writeErr(w, err) {
		return
	}
	defer photo.Close()
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// Stream straight from object storage; the photo is never held in memory.
	_, _ = io.Copy(w, photo)
}

func (h Handler) issueSeen(w http.ResponseWriter, r *http.Request) {
	if writeErr(w, h.Service.MarkIssueSeen(r.Context(), h.profile(r), chi.URLParam(r, "tripId"), chi.URLParam(r, "orderId"), chi.URLParam(r, "issueId"))) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "seen"})
}

func (h Handler) raiseAlert(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Type             string `json:"type"`
		OrderRef         string `json:"orderRef"`
		BelongsVehicleID string `json:"belongsVehicleId"`
		Note             string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierrors.BadRequest(w, "invalid JSON")
		return
	}
	a, err := h.Service.RaiseDockAlert(r.Context(), h.profile(r), chi.URLParam(r, "tripId"), body.Type, body.OrderRef, body.BelongsVehicleID, body.Note, r.Header.Get("Idempotency-Key"))
	if writeErr(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"alert": a})
}

func (h Handler) listAlerts(w http.ResponseWriter, r *http.Request) {
	date := r.URL.Query().Get("date")
	if date == "" {
		apierrors.BadRequest(w, "date is required")
		return
	}
	items, err := h.Service.ListDockAlerts(r.Context(), h.profile(r), date)
	if writeErr(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h Handler) resolveAlert(w http.ResponseWriter, r *http.Request) {
	a, err := h.Service.ResolveDockAlert(r.Context(), h.profile(r), chi.URLParam(r, "alertId"))
	if writeErr(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"alert": a})
}

func (h Handler) internalList(w http.ResponseWriter, r *http.Request) {
	date := r.URL.Query().Get("date")
	if date == "" {
		apierrors.BadRequest(w, "date is required")
		return
	}
	items, err := h.Service.InternalList(r.Context(), date, r.URL.Query().Get("vehicleId"))
	if writeErr(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h Handler) internalGet(w http.ResponseWriter, r *http.Request) {
	detail, err := h.Service.InternalGet(r.Context(), chi.URLParam(r, "tripId"))
	if writeErr(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, detail)
}

func writeErr(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	var inc service.IncompleteError
	if errors.As(err, &inc) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"type": "loading_incomplete", "title": "Conflict", "status": 409,
			"detail": "pending orders remain", "pendingOrderIds": inc.Pending,
		})
		return true
	}
	var dec service.DecisionRequiredError
	if errors.As(err, &dec) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusConflict)
		detail := "a loader shortfall is waiting for the dispatcher's decision"
		if len(dec.MoveUnpublished) > 0 {
			detail = "a line moved to the next run is still on the confirmed plan; the new plan must be published and the loader must acknowledge it first"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"type": "dispatcher_decision_required", "title": "Conflict", "status": 409,
			"detail": detail, "orderIds": dec.OrderIDs, "moveUnpublishedOrderIds": dec.MoveUnpublished,
		})
		return true
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "ACTIVE_LOADING_ISSUE"):
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"type": "ACTIVE_LOADING_ISSUE", "title": "Conflict", "status": 409,
			"detail": "order has an active loading issue",
		})
	case strings.HasPrefix(msg, "invalid"):
		apierrors.BadRequest(w, msg)
	case strings.HasPrefix(msg, "not found"):
		apierrors.NotFound(w, msg)
	case strings.HasPrefix(msg, "forbidden"):
		apierrors.Forbidden(w, msg)
	case strings.HasPrefix(msg, "conflict"):
		apierrors.Conflict(w, msg)
	default:
		apierrors.Internal(w, "unexpected error")
	}
	return true
}
