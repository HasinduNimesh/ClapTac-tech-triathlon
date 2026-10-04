package handler

import (
    "encoding/json"
    "io"
    "net/http"
    "time"

    "github.com/go-chi/chi/v5"
    apierrors "github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/errors"
    "github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/httpx"
)

func (h Handler) updateLocation(w http.ResponseWriter, r *http.Request) {
    r.Body = http.MaxBytesReader(w, r.Body, 1024)
    decoder := json.NewDecoder(r.Body)
    decoder.DisallowUnknownFields()
    var body struct {
        Latitude *float64 `json:"latitude"`
        Longitude *float64 `json:"longitude"`
        Timestamp string `json:"timestamp"`
    }
    if decoder.Decode(&body) != nil || body.Latitude == nil || body.Longitude == nil || body.Timestamp == "" {
        apierrors.BadRequest(w, "invalid: latitude, longitude and timestamp required")
        return
    }
    var extra any
    if decoder.Decode(&extra) != io.EOF {
        apierrors.BadRequest(w, "invalid: one location object required")
        return
    }
    at, err := time.Parse(time.RFC3339Nano, body.Timestamp)
    if err != nil {
        apierrors.BadRequest(w, "invalid: timestamp must be RFC3339")
        return
    }
    point, err := h.Service.UpdateLocation(r.Context(), h.profile(r), chi.URLParam(r, "tripId"), *body.Latitude, *body.Longitude, at)
    if writeErr(w, err) { return }
    httpx.WriteJSON(w, http.StatusOK, map[string]any{"location": point})
}

func (h Handler) tripLocation(w http.ResponseWriter, r *http.Request) {
    point, err := h.Service.TripLocation(r.Context(), h.profile(r), chi.URLParam(r, "tripId"))
    if writeErr(w, err) { return }
    httpx.WriteJSON(w, http.StatusOK, map[string]any{"location": point})
}
