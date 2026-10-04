package handler

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/authorization"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/httpx"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/delivery-service/internal/domain"
)

// ConflictDriver is the dispatcher's sync-conflict list. It is optional so the
// base Driver interface (and its test doubles) stay unchanged.
type ConflictDriver interface {
	ListSyncConflicts(ctx context.Context, profile *authorization.Profile, date string, openOnly bool) ([]domain.SyncConflict, error)
	SettleSyncConflict(ctx context.Context, profile *authorization.Profile, id string) (domain.SyncConflict, error)
}

func (h Handler) conflictRoutes(r chi.Router) {
	cd, ok := h.Service.(ConflictDriver)
	if !ok {
		return
	}
	all := authorization.RequireWith(h.Authn, h.Profiles, authorization.PermDeliveryViewAll)
	r.With(all).Get("/sync-conflicts", func(w http.ResponseWriter, req *http.Request) {
		items, err := cd.ListSyncConflicts(req.Context(), h.profile(req), req.URL.Query().Get("date"), req.URL.Query().Get("status") == "open")
		if writeErr(w, err) {
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
	})
	r.With(all).Post("/sync-conflicts/{id}/settle", func(w http.ResponseWriter, req *http.Request) {
		c, err := cd.SettleSyncConflict(req.Context(), h.profile(req), chi.URLParam(req, "id"))
		if writeErr(w, err) {
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"conflict": c})
	})
}
