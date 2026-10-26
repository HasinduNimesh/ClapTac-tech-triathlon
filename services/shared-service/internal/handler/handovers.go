package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/audit"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/authorization"
	apierrors "github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/errors"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/httpx"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/shared-service/internal/handovers"
	"github.com/go-chi/chi/v5"
)

func handoverScope(w http.ResponseWriter, r *http.Request) (*authorization.Profile, string, string, bool) {
	w.Header().Set("Cache-Control", "no-store")
	p, _ := authorization.ProfileFrom(r.Context())
	role, scope := r.URL.Query().Get("role"), r.URL.Query().Get("scope")
	if !handovers.Authorized(p, role, scope) {
		apierrors.Forbidden(w, "handover scope not authorized")
		return nil, "", "", false
	}
	return p, role, scope, true
}
func (h Handler) listHandovers(w http.ResponseWriter, r *http.Request) {
	_, role, scope, ok := handoverScope(w, r)
	if !ok {
		return
	}
	offset := 0
	if value := r.URL.Query().Get("offset"); value != "" {
		n, err := strconv.Atoi(value)
		if err != nil || n < 0 || n > 10000 {
			apierrors.BadRequest(w, "invalid history offset")
			return
		}
		offset = n
	}
	items, more, err := h.Store.ListHandovers(r.Context(), role, scope, offset)
	if err != nil {
		apierrors.Internal(w, "could not load handovers")
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"items": items, "hasMore": more, "serverTime": time.Now().UTC()})
}
func (h Handler) publishHandover(w http.ResponseWriter, r *http.Request) {
	p, role, scope, ok := handoverScope(w, r)
	if !ok {
		return
	}
	var body struct {
		ID       string             `json:"id"`
		Snapshot handovers.Snapshot `json:"snapshot"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 512<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		apierrors.BadRequest(w, "invalid handover JSON")
		return
	}
	if dec.Decode(&struct{}{}) != io.EOF || !handovers.UUID.MatchString(body.ID) {
		apierrors.BadRequest(w, "one handover and a valid publication ID are required")
		return
	}
	if err := handovers.Validate(body.Snapshot, role); err != nil {
		apierrors.BadRequest(w, err.Error())
		return
	}
	name := p.DisplayName
	if name == "" {
		name = p.UserID
	}
	saved, err := h.Store.PublishHandover(r.Context(), handovers.Record{ID: body.ID, Role: role, Scope: scope, AuthorID: p.UserID, AuthorName: name, Snapshot: body.Snapshot}, audit.Event{CorrelationID: httpx.CorrelationIDFrom(r.Context())})
	if err != nil {
		handoverError(w, err)
		return
	}
	httpx.WriteJSON(w, 201, map[string]any{"handover": saved})
}
func (h Handler) acknowledgeHandover(w http.ResponseWriter, r *http.Request) {
	p, role, scope, ok := handoverScope(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	if !handovers.UUID.MatchString(id) {
		apierrors.BadRequest(w, "invalid handover ID")
		return
	}
	name := p.DisplayName
	if name == "" {
		name = p.UserID
	}
	saved, err := h.Store.AcknowledgeHandover(r.Context(), id, role, scope, p.UserID, name, audit.Event{CorrelationID: httpx.CorrelationIDFrom(r.Context())})
	if err != nil {
		handoverError(w, err)
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"handover": saved})
}
func handoverError(w http.ResponseWriter, err error) {
	switch {
	case strings.HasPrefix(err.Error(), "conflict:"):
		apierrors.Conflict(w, strings.TrimPrefix(err.Error(), "conflict: "))
	case strings.HasPrefix(err.Error(), "not found"):
		apierrors.NotFound(w, "handover not found")
	default:
		apierrors.Internal(w, "handover could not be saved")
	}
}
