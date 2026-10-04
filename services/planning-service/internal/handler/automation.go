package handler

import (
	"context"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/automation"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/httpx"
	"net/http"
	"time"
)

type AutomationHistory interface {
	AutomationSnapshot(context.Context, time.Time) (automation.Snapshot, error)
}

func (h Handler) automationSnapshot(w http.ResponseWriter, r *http.Request) {
	at, err := time.Parse(time.RFC3339, r.URL.Query().Get("at"))
	if err != nil || at.After(time.Now().Add(time.Minute)) {
		http.Error(w, "valid past at timestamp required", 400)
		return
	}
	repo, ok := h.Automation.(AutomationHistory)
	if !ok {
		http.Error(w, "history unavailable", 503)
		return
	}
	snapshot, err := repo.AutomationSnapshot(r.Context(), at)
	if err != nil {
		http.Error(w, "history unavailable", 503)
		return
	}
	httpx.WriteJSON(w, 200, snapshot)
}
