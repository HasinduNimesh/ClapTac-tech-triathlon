package handler

import (
	"context"
	"encoding/json"
	apierrors "github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/errors"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/httpx"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/delivery-service/internal/store"
	"net/http"
)

func (h Handler) suspendRecovery(w http.ResponseWriter, r *http.Request) {
	s, ok := h.Service.(interface {
		SuspendRecovery(context.Context, string, []store.RecoveryStop) error
	})
	if !ok {
		apierrors.Internal(w, "recovery unavailable")
		return
	}
	var body struct {
		CaseID string               `json:"caseId"`
		Stops  []store.RecoveryStop `json:"stops"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 65536)).Decode(&body) != nil {
		apierrors.BadRequest(w, "invalid recovery request")
		return
	}
	if writeErr(w, s.SuspendRecovery(r.Context(), body.CaseID, body.Stops)) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]bool{"suspended": true})
}
