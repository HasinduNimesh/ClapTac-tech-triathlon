package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/auth"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/authorization"
	apierrors "github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/errors"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/httpx"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/delivery-service/internal/domain"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/delivery-service/internal/service"
)

type Driver interface {
	List(ctx context.Context, profile *authorization.Profile, date, vehicleID string) ([]map[string]any, error)
	LatenessHistory(ctx context.Context, profile *authorization.Profile, tripID string) ([]domain.LatenessProbability, error)
	Get(ctx context.Context, profile *authorization.Profile, tripID string) (map[string]any, error)
	Prepare(ctx context.Context, profile *authorization.Profile, tripID string) (map[string]any, error)
	Start(ctx context.Context, profile *authorization.Profile, tripID, opID string) (map[string]any, error)
	UpdateLocation(ctx context.Context, profile *authorization.Profile, tripID string, latitude, longitude float64, timestamp time.Time) (domain.Location, error)
	TripLocation(ctx context.Context, profile *authorization.Profile, tripID string) (*domain.Location, error)
	Arrive(ctx context.Context, profile *authorization.Profile, tripID, stopID, opID, occurred string) (map[string]any, error)
	Outcome(ctx context.Context, profile *authorization.Profile, tripID, stopID, opID, depends, code, reason, note, occurred string, deliveredUnits *int) (map[string]any, error)
	UploadProof(ctx context.Context, profile *authorization.Profile, tripID, stopID, opID, proofType, mime string, body []byte, captured, receiverName string) (domain.Proof, error)
	Complete(ctx context.Context, profile *authorization.Profile, tripID, opID, occurred string) (map[string]any, error)
	Sync(ctx context.Context, profile *authorization.Profile, req domain.SyncRequest) []map[string]any
	InternalOrder(ctx context.Context, orderID string) (domain.OrderTracking, error)
	OutletLastServed(ctx context.Context) ([]domain.OutletLastServed, error)
	OutletLastAttempted(ctx context.Context, beforeDate string) ([]domain.OutletLastAttempted, error)
	SendTripMessage(ctx context.Context, profile *authorization.Profile, tripID, stopID, body string) (domain.TripMessage, error)
	TripMessages(ctx context.Context, profile *authorization.Profile, tripID string) ([]domain.TripMessage, error)
	AcknowledgeTripMessage(ctx context.Context, profile *authorization.Profile, tripID, messageID string) (domain.TripMessage, error)
	ReportOfflineQueueHealth(profile *authorization.Profile, report domain.OfflineQueueHealth) error
}

type Handler struct {
	Authn    auth.Authenticator
	Profiles authorization.ProfileResolver
	Service  Driver
}

var _ Driver = service.Service{}

func (h Handler) Routes(r chi.Router) {
	view := authorization.RequireAnyWith(h.Authn, h.Profiles, authorization.PermDeliveryView, authorization.PermDeliveryViewAll, authorization.PermDeliveryViewAssigned)
	start := authorization.RequireWith(h.Authn, h.Profiles, authorization.PermDeliveryStart)
	update := authorization.RequireWith(h.Authn, h.Profiles, authorization.PermDeliveryUpdate)
	proof := authorization.RequireWith(h.Authn, h.Profiles, authorization.PermDeliveryProof)
	complete := authorization.RequireWith(h.Authn, h.Profiles, authorization.PermDeliveryComplete)
	sync := authorization.RequireWith(h.Authn, h.Profiles, authorization.PermDeliverySync)
	internal := authorization.RequireWith(h.Authn, h.Profiles, authorization.PermDeliveriesReadInternal)

	r.Route("/api/v1/delivery", func(r chi.Router) {
		r.With(view).Get("/trips", h.list)
		r.With(view).Get("/drivers/me/trips", h.list)
		r.With(view).Get("/trips/{tripId}", h.get)
		r.With(authorization.RequireWith(h.Authn, h.Profiles, authorization.PermDeliveryViewAll)).Get("/trips/{tripId}/lateness-history", h.latenessHistory)
		r.With(view).Get("/trips/{tripId}/messages", h.tripMessages)
		r.With(authorization.RequireWith(h.Authn, h.Profiles, authorization.PermDeliveryViewAll)).Post("/trips/{tripId}/messages", h.sendTripMessage)
		r.With(update).Post("/trips/{tripId}/messages/{messageId}/ack", h.ackTripMessage)
		r.With(start).Post("/trips/{tripId}/prepare", h.prepare)
		r.With(start).Post("/trips/{tripId}/start", h.start)
		r.With(update).Post("/trips/{tripId}/location", h.updateLocation)
		r.With(authorization.RequireWith(h.Authn, h.Profiles, authorization.PermDeliveryViewAll)).Get("/trips/{tripId}/location", h.tripLocation)
		r.With(update).Post("/trips/{tripId}/stops/{stopId}/arrive", h.arrive)
		r.With(proof).Post("/trips/{tripId}/stops/{stopId}/proofs", h.proof)
		r.With(update).Post("/trips/{tripId}/stops/{stopId}/outcome", h.outcome)
		r.With(complete).Post("/trips/{tripId}/complete", h.complete)
		r.With(sync).Post("/telemetry/offline-queue", h.offlineQueueHealth)
		r.With(sync).Post("/sync", h.sync)
		r.With(internal).Get("/internal/orders/{orderId}", h.internalOrder)
		r.With(internal).Get("/internal/outlets/last-served", h.outletLastServed)
		r.With(internal).Get("/internal/outlets/last-attempted", h.outletLastAttempted)
	})
}

func (h Handler) latenessHistory(w http.ResponseWriter, r *http.Request) {
	items, err := h.Service.LatenessHistory(r.Context(), h.profile(r), chi.URLParam(r, "tripId"))
	if writeErr(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h Handler) offlineQueueHealth(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	var report domain.OfflineQueueHealth
	if err := dec.Decode(&report); err != nil {
		apierrors.BadRequest(w, "invalid offline queue health report")
		return
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		apierrors.BadRequest(w, "offline queue health must contain one object")
		return
	}
	if err := h.Service.ReportOfflineQueueHealth(h.profile(r), report); writeErr(w, err) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h Handler) tripMessages(w http.ResponseWriter, r *http.Request) {
	items, err := h.Service.TripMessages(r.Context(), h.profile(r), chi.URLParam(r, "tripId"))
	if writeErr(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}
func (h Handler) sendTripMessage(w http.ResponseWriter, r *http.Request) {
	var req struct {
		StopID string `json:"stopId"`
		Body   string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierrors.BadRequest(w, "invalid JSON")
		return
	}
	item, err := h.Service.SendTripMessage(r.Context(), h.profile(r), chi.URLParam(r, "tripId"), req.StopID, req.Body)
	if writeErr(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"message": item})
}
func (h Handler) ackTripMessage(w http.ResponseWriter, r *http.Request) {
	item, err := h.Service.AcknowledgeTripMessage(r.Context(), h.profile(r), chi.URLParam(r, "tripId"), chi.URLParam(r, "messageId"))
	if writeErr(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"message": item})
}

func (h Handler) outletLastServed(w http.ResponseWriter, r *http.Request) {
	items, err := h.Service.OutletLastServed(r.Context())
	if writeErr(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h Handler) outletLastAttempted(w http.ResponseWriter, r *http.Request) {
	before := r.URL.Query().Get("before")
	if before == "" {
		apierrors.BadRequest(w, "before query parameter is required (YYYY-MM-DD)")
		return
	}
	items, err := h.Service.OutletLastAttempted(r.Context(), before)
	if writeErr(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h Handler) internalOrder(w http.ResponseWriter, r *http.Request) {
	item, err := h.Service.InternalOrder(r.Context(), chi.URLParam(r, "orderId"))
	if writeErr(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"delivery": item})
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
	items, err := h.Service.List(r.Context(), h.profile(r), date, r.URL.Query().Get("vehicleId"))
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

func (h Handler) prepare(w http.ResponseWriter, r *http.Request) {
	detail, err := h.Service.Prepare(r.Context(), h.profile(r), chi.URLParam(r, "tripId"))
	if writeErr(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, detail)
}

func (h Handler) start(w http.ResponseWriter, r *http.Request) {
	var body struct {
		OperationID string `json:"operationId"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	opID := first(r.Header.Get("Idempotency-Key"), body.OperationID)
	detail, err := h.Service.Start(r.Context(), h.profile(r), chi.URLParam(r, "tripId"), opID)
	if writeErr(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, detail)
}

func (h Handler) arrive(w http.ResponseWriter, r *http.Request) {
	var body struct {
		OccurredAt  string `json:"occurredAt"`
		OperationID string `json:"operationId"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	opID := first(r.Header.Get("Idempotency-Key"), body.OperationID)
	detail, err := h.Service.Arrive(r.Context(), h.profile(r), chi.URLParam(r, "tripId"), chi.URLParam(r, "stopId"), opID, body.OccurredAt)
	if writeErr(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, detail)
}

func (h Handler) outcome(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Code                 string `json:"code"`
		Reason               string `json:"reason"`
		Note                 string `json:"note"`
		DeliveredUnits       *int   `json:"deliveredUnits"`
		OccurredAt           string `json:"occurredAt"`
		OperationID          string `json:"operationId"`
		DependsOnOperationID string `json:"dependsOnOperationId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierrors.BadRequest(w, "invalid JSON")
		return
	}
	opID := first(r.Header.Get("Idempotency-Key"), body.OperationID)
	detail, err := h.Service.Outcome(r.Context(), h.profile(r), chi.URLParam(r, "tripId"), chi.URLParam(r, "stopId"), opID, body.DependsOnOperationID, body.Code, body.Reason, body.Note, body.OccurredAt, body.DeliveredUnits)
	if writeErr(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, detail)
}

func (h Handler) proof(w http.ResponseWriter, r *http.Request) {
	const multipartOverheadLimit = 256 << 10
	r.Body = http.MaxBytesReader(w, r.Body, domain.MaxPhotoBytes+multipartOverheadLimit)
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			apierrors.RequestEntityTooLarge(w, "proof upload exceeds the request size limit")
			return
		}
		apierrors.BadRequest(w, "multipart proof required")
		return
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		apierrors.BadRequest(w, "file is required")
		return
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, domain.MaxPhotoBytes+1))
	if err != nil {
		apierrors.BadRequest(w, "unable to read file")
		return
	}
	mime := hdr.Header.Get("Content-Type")
	if mime == "" || mime == "application/octet-stream" {
		mime = r.FormValue("mimeType")
	}
	opID := first(r.Header.Get("Idempotency-Key"), r.FormValue("operationId"))
	pr, err := h.Service.UploadProof(r.Context(), h.profile(r), chi.URLParam(r, "tripId"), chi.URLParam(r, "stopId"), opID, r.FormValue("type"), mime, body, r.FormValue("capturedAt"), r.FormValue("receiverName"))
	if writeErr(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"proof": pr})
}

func (h Handler) complete(w http.ResponseWriter, r *http.Request) {
	var body struct {
		OccurredAt  string `json:"occurredAt"`
		OperationID string `json:"operationId"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	opID := first(r.Header.Get("Idempotency-Key"), body.OperationID)
	detail, err := h.Service.Complete(r.Context(), h.profile(r), chi.URLParam(r, "tripId"), opID, body.OccurredAt)
	if writeErr(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, detail)
}

func (h Handler) sync(w http.ResponseWriter, r *http.Request) {
	var req domain.SyncRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierrors.BadRequest(w, "invalid JSON")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"results": h.Service.Sync(r.Context(), h.profile(r), req)})
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
			"type": "delivery_incomplete", "title": "Conflict", "status": 409,
			"detail": "stops remain without a terminal outcome", "pendingStopIds": inc.Pending,
		})
		return true
	}
	msg := err.Error()
	switch {
	case strings.HasPrefix(msg, "invalid"):
		apierrors.BadRequest(w, msg)
	case strings.HasPrefix(msg, "not found"):
		apierrors.NotFound(w, msg)
	case strings.HasPrefix(msg, "forbidden"):
		apierrors.Forbidden(w, msg)
	case strings.HasPrefix(msg, "rejected"):
		apierrors.BadRequest(w, msg)
	case strings.HasPrefix(msg, "conflict"):
		apierrors.Conflict(w, msg)
	case strings.Contains(msg, "object store"):
		apierrors.Internal(w, msg)
	default:
		apierrors.Internal(w, "unexpected error")
	}
	return true
}

func first(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
