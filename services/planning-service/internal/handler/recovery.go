package handler

import (
	"context"
	"encoding/json"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/authorization"
	apierrors "github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/errors"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/httpx"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/recovery"
	"github.com/go-chi/chi/v5"
	"net/http"
	"time"
)

type recoveryPlanner interface {
	RecoveryCases(context.Context, *authorization.Profile, string) ([]recovery.Case, error)
	CreateRecovery(context.Context, *authorization.Profile, string, string, time.Time) (recovery.Case, error)
	RecoveryPreference(context.Context, *authorization.Profile, string, string, string, string) error
	ApproveRecovery(context.Context, *authorization.Profile, string, string, string, bool) (recovery.Case, error)
}

func (h Handler) recoveryRoutes(r chi.Router) {
	view := authorization.RequireAnyWith(h.Authn, h.Profiles, authorization.PermPlanView, authorization.PermOrderViewOwn)
	write := authorization.RequireWith(h.Authn, h.Profiles, authorization.PermPlanUpdate)
	r.With(view).Get("/recovery", h.recoveryList)
	r.With(write).Post("/recovery", h.recoveryCreate)
	r.With(authorization.RequireWith(h.Authn, h.Profiles, authorization.PermOrderViewOwn)).Put("/recovery/{caseId}/preference", h.recoveryPreference)
	r.With(write).Post("/recovery/{caseId}/approve", h.recoveryApprove)
}
func (h Handler) recoveryService(w http.ResponseWriter) recoveryPlanner {
	s, ok := h.Service.(recoveryPlanner)
	if !ok {
		apierrors.Internal(w, "recovery unavailable")
		return nil
	}
	return s
}
func (h Handler) recoveryList(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	s := h.recoveryService(w)
	if s == nil {
		return
	}
	items, err := s.RecoveryCases(r.Context(), h.profile(r), r.URL.Query().Get("date"))
	if writeErr(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items, "serverNow": time.Now().UTC()})
}
func (h Handler) recoveryCreate(w http.ResponseWriter, r *http.Request) {
	s := h.recoveryService(w)
	if s == nil {
		return
	}
	var b struct {
		PlanID     string    `json:"planId"`
		IncidentID string    `json:"incidentId"`
		ReadyAt    time.Time `json:"readyAt"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&b) != nil {
		apierrors.BadRequest(w, "valid handover time required")
		return
	}
	c, err := s.CreateRecovery(r.Context(), h.profile(r), b.PlanID, b.IncidentID, b.ReadyAt)
	if writeErr(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, c)
}
func (h Handler) recoveryPreference(w http.ResponseWriter, r *http.Request) {
	s := h.recoveryService(w)
	if s == nil {
		return
	}
	var b struct {
		OutletID    string `json:"outletId"`
		CandidateID string `json:"candidateId"`
		Note        string `json:"note"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&b) != nil {
		apierrors.BadRequest(w, "invalid preference")
		return
	}
	if writeErr(w, s.RecoveryPreference(r.Context(), h.profile(r), chi.URLParam(r, "caseId"), b.OutletID, b.CandidateID, b.Note)) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]bool{"saved": true})
}
func (h Handler) recoveryApprove(w http.ResponseWriter, r *http.Request) {
	s := h.recoveryService(w)
	if s == nil {
		return
	}
	var b struct {
		CandidateID    string `json:"candidateId"`
		Note           string `json:"note"`
		ReadyConfirmed bool   `json:"readyConfirmed"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&b) != nil {
		apierrors.BadRequest(w, "invalid approval")
		return
	}
	c, err := s.ApproveRecovery(r.Context(), h.profile(r), chi.URLParam(r, "caseId"), b.CandidateID, b.Note, b.ReadyConfirmed)
	if writeErr(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, c)
}
