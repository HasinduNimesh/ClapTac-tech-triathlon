package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/audit"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/authorization"
	apierrors "github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/errors"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/httpx"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/shared-service/internal/dashboards"
)

// Saved dashboards are owned by the signed-in application user. Every query is
// keyed by that user id, so one person can never read or change another's.

func dashboardOwner(w http.ResponseWriter, r *http.Request) (string, bool) {
	profile, _ := authorization.ProfileFrom(r.Context())
	if profile == nil || profile.UserID == "" {
		apierrors.Forbidden(w, "application user required")
		return "", false
	}
	return profile.UserID, true
}

func decodeDashboard(w http.ResponseWriter, r *http.Request) (dashboards.Spec, int, bool) {
	var body struct {
		Spec    dashboards.Spec `json:"spec"`
		Version int             `json:"version"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		apierrors.BadRequest(w, "invalid dashboard JSON")
		return dashboards.Spec{}, 0, false
	}
	spec, err := dashboards.Normalize(body.Spec)
	if err != nil {
		apierrors.BadRequest(w, err.Error())
		return dashboards.Spec{}, 0, false
	}
	return spec, body.Version, true
}

func (h Handler) listDashboards(w http.ResponseWriter, r *http.Request) {
	owner, ok := dashboardOwner(w, r)
	if !ok {
		return
	}
	items, err := h.Store.ListDashboards(r.Context(), owner)
	if err != nil {
		apierrors.Internal(w, "list dashboards failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h Handler) createDashboard(w http.ResponseWriter, r *http.Request) {
	owner, ok := dashboardOwner(w, r)
	if !ok {
		return
	}
	spec, _, ok := decodeDashboard(w, r)
	if !ok {
		return
	}
	saved, err := h.Store.CreateDashboard(r.Context(), owner, spec, audit.Event{Action: "USER_DASHBOARD_CREATED", CorrelationID: httpx.CorrelationIDFrom(r.Context())})
	if err != nil {
		writeDashboardError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"dashboard": saved})
}

func (h Handler) updateDashboard(w http.ResponseWriter, r *http.Request) {
	owner, ok := dashboardOwner(w, r)
	if !ok {
		return
	}
	spec, version, ok := decodeDashboard(w, r)
	if !ok {
		return
	}
	saved, err := h.Store.UpdateDashboard(r.Context(), owner, chi.URLParam(r, "id"), spec, version, audit.Event{Action: "USER_DASHBOARD_UPDATED", CorrelationID: httpx.CorrelationIDFrom(r.Context())})
	if err != nil {
		writeDashboardError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"dashboard": saved})
}

func (h Handler) deleteDashboard(w http.ResponseWriter, r *http.Request) {
	owner, ok := dashboardOwner(w, r)
	if !ok {
		return
	}
	if err := h.Store.DeleteDashboard(r.Context(), owner, chi.URLParam(r, "id"), audit.Event{Action: "USER_DASHBOARD_DELETED", CorrelationID: httpx.CorrelationIDFrom(r.Context())}); err != nil {
		writeDashboardError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeDashboardError(w http.ResponseWriter, err error) {
	switch msg := err.Error(); {
	case strings.HasPrefix(msg, "not found"):
		apierrors.NotFound(w, "dashboard not found")
	case strings.HasPrefix(msg, "conflict"):
		apierrors.Conflict(w, strings.TrimPrefix(msg, "conflict: "))
	default:
		apierrors.Internal(w, "dashboard save failed")
	}
}
