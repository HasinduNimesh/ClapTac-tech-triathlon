package handler

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/auth"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/authorization"
	apierrors "github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/errors"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/httpx"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/order-service/internal/domain"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/order-service/internal/service"
)

type Handler struct {
	Authn    auth.Authenticator
	Profiles authorization.ProfileResolver
	Service  service.Service
}

type OrderWorkflows interface {
	Tracking(*authorization.Profile, string) (domain.Tracking, error)
	PendingReceipts(*authorization.Profile) ([]domain.ReceiptTask, error)
	Receipt(*authorization.Profile, string) (domain.Receipt, []domain.ReceiptIssue, error)
	ConfirmReceipt(*authorization.Profile, string, domain.ReceiptConfirmation) (domain.Receipt, []domain.ReceiptIssue, bool, error)
	ReportReceiptIssue(*authorization.Profile, string, domain.ReceiptIssueRequest) (domain.Receipt, domain.ReceiptIssue, bool, error)
	ReceiptIssues(*authorization.Profile) ([]domain.ReceiptIssueView, error)
	RecordCustody(*authorization.Profile, string, domain.CustodyEvent) (domain.CustodyEvent, bool, error)
}

func (h Handler) Routes(r chi.Router) {
	r.Route("/api/v1/orders", func(r chi.Router) {
		r.With(authorization.RequireWith(h.Authn, h.Profiles, authorization.PermOrderViewAll)).Get("/forecast", h.forecast)
		r.With(authorization.RequireWith(h.Authn, h.Profiles, authorization.PermOrderViewAll)).Get("/export.csv", h.exportCSV)
		r.With(authorization.RequireWith(h.Authn, h.Profiles, authorization.PermOrderViewAll)).Post("/import.csv", h.importCSV)
		r.With(authorization.RequireWith(h.Authn, h.Profiles, authorization.PermOrderViewAll)).Get("/export", h.exportJSON)
		r.With(authorization.RequireWith(h.Authn, h.Profiles, authorization.PermOrderViewAll)).Post("/import", h.importJSON)
		r.Post("/internal/delivery-followups", h.deliveryFollowup)
		r.With(authorization.RequireWith(h.Authn, h.Profiles, authorization.PermOrderCreate)).Post("/", h.create)
		r.With(authorization.RequireAnyWith(h.Authn, h.Profiles, authorization.PermOrderViewOwn, authorization.PermOrderViewAll, authorization.PermOrdersReadInternal)).Get("/", h.list)
		r.With(authorization.RequireAnyWith(h.Authn, h.Profiles, authorization.PermOrderViewOwn, authorization.PermOrderViewAll, authorization.PermOrdersReadInternal)).Get("/{id}", h.get)
		r.With(authorization.RequireAnyWith(h.Authn, h.Profiles, authorization.PermOrderViewOwn, authorization.PermOrderViewAll)).Get("/receipts/pending", h.pendingReceipts)
		r.With(authorization.RequireWith(h.Authn, h.Profiles, authorization.PermOrderViewAll)).Get("/receipt-issues", h.receiptIssues)
		r.With(authorization.RequireAnyWith(h.Authn, h.Profiles, authorization.PermOrderViewOwn, authorization.PermOrderViewAll)).Get("/{id}/tracking", h.tracking)
		r.With(authorization.RequireWith(h.Authn, h.Profiles, authorization.PermReceiptConfirm)).Post("/{id}/receipt/confirm", h.confirmReceipt)
		r.With(authorization.RequireWith(h.Authn, h.Profiles, authorization.PermDeliveryIssueCreate)).Post("/{id}/receipt/issues", h.reportReceiptIssue)
		r.With(authorization.RequireAnyWith(h.Authn, h.Profiles, authorization.PermOrderViewOwn, authorization.PermOrderViewAll)).Get("/{id}/receipt", h.receipt)
		r.With(authorization.RequireAnyWith(h.Authn, h.Profiles, authorization.PermOrderViewOwn, authorization.PermOrderViewAll, authorization.PermLoadingUpdate, authorization.PermDeliveryUpdate, authorization.PermReceiptConfirm)).Post("/{id}/custody", h.recordCustody)
	})
}

func (h Handler) recordCustody(w http.ResponseWriter, r *http.Request) {
	wf := h.workflows()
	if wf == nil {
		apierrors.Internal(w, "custody unavailable")
		return
	}
	var event domain.CustodyEvent
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&event); err != nil {
		apierrors.BadRequest(w, "invalid JSON")
		return
	}
	if event.IdempotencyKey == "" {
		event.IdempotencyKey = r.Header.Get("Idempotency-Key")
	}
	p, _ := authorization.ProfileFrom(r.Context())
	createdEvent, created, err := wf.RecordCustody(p, chi.URLParam(r, "id"), event)
	if writeServiceError(w, err) {
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	httpx.WriteJSON(w, status, map[string]any{"event": createdEvent, "created": created})
}

func (h Handler) exportJSON(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("version") != "1" {
		apierrors.BadRequest(w, "API export version=1 is required")
		return
	}
	p, _ := authorization.ProfileFrom(r.Context())
	items, err := h.Service.List(p, domain.ListFilter{})
	if writeServiceError(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"version": 1, "items": items})
}

func (h Handler) importJSON(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	var req struct {
		Version      int            `json:"version"`
		SourceSystem string         `json:"sourceSystem"`
		Orders       []domain.Order `json:"orders"`
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		apierrors.BadRequest(w, "invalid import JSON")
		return
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		apierrors.BadRequest(w, "import JSON must contain one object")
		return
	}
	if req.Version != 1 {
		apierrors.BadRequest(w, "API import version 1 is required")
		return
	}
	p, _ := authorization.ProfileFrom(r.Context())
	results, err := h.Service.ImportOrders(p, r.Header.Get("Authorization"), req.SourceSystem, req.Orders)
	if writeServiceError(w, err) {
		return
	}
	created, duplicates := 0, 0
	for _, v := range results {
		if v.Created {
			created++
		} else {
			duplicates++
		}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"version": 1, "sourceSystem": req.SourceSystem, "created": created, "duplicates": duplicates, "items": results})
}

var importCSVHeader = []string{"version", "external_order_id", "outlet_id", "brand", "requested_delivery_date", "order_units", "order_weight_kg", "order_volume_m3", "temperature_requirement"}

func (h Handler) exportCSV(w http.ResponseWriter, r *http.Request) {
	p, _ := authorization.ProfileFrom(r.Context())
	items, err := h.Service.List(p, domain.ListFilter{})
	if writeServiceError(w, err) {
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=waypoint-orders-v1.csv")
	cw := csv.NewWriter(w)
	_ = cw.Write(importCSVHeader)
	for _, o := range items {
		external := o.ExternalOrderID
		if external == "" {
			external = o.OrderRef
		}
		_ = cw.Write([]string{"1", external, o.OutletID, o.Brand, o.RequestedDeliveryDate, strconv.Itoa(o.OrderUnits), strconv.FormatFloat(o.OrderWeightKg, 'f', 3, 64), strconv.FormatFloat(o.OrderVolumeM3, 'f', 3, 64), string(o.TemperatureRequirement)})
	}
	cw.Flush()
}

func (h Handler) importCSV(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("version") != "1" {
		http.Error(w, "CSV version=1 is required", http.StatusBadRequest)
		return
	}
	source := r.URL.Query().Get("sourceSystem")
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	cr := csv.NewReader(r.Body)
	cr.FieldsPerRecord = len(importCSVHeader)
	rows, err := cr.ReadAll()
	if err != nil {
		http.Error(w, "invalid CSV: "+err.Error(), http.StatusBadRequest)
		return
	}
	if len(rows) < 2 || len(rows) > 501 {
		http.Error(w, "CSV must contain a header and 1 to 500 order rows", http.StatusBadRequest)
		return
	}
	for i, col := range importCSVHeader {
		if rows[0][i] != col {
			http.Error(w, fmt.Sprintf("invalid CSV header at column %d", i+1), http.StatusBadRequest)
			return
		}
	}
	items := make([]domain.Order, 0, len(rows)-1)
	for i, row := range rows[1:] {
		if row[0] != "1" {
			http.Error(w, fmt.Sprintf("unsupported row version at line %d", i+2), http.StatusBadRequest)
			return
		}
		units, e1 := strconv.Atoi(row[5])
		weight, e2 := strconv.ParseFloat(row[6], 64)
		volume, e3 := strconv.ParseFloat(row[7], 64)
		if e1 != nil || e2 != nil || e3 != nil {
			http.Error(w, fmt.Sprintf("invalid numeric field at line %d", i+2), http.StatusBadRequest)
			return
		}
		items = append(items, domain.Order{ExternalOrderID: row[1], OutletID: row[2], Brand: row[3], RequestedDeliveryDate: row[4], OrderUnits: units, OrderWeightKg: weight, OrderVolumeM3: volume, TemperatureRequirement: domain.Temperature(row[8])})
	}
	p, _ := authorization.ProfileFrom(r.Context())
	results, err := h.Service.ImportOrders(p, r.Header.Get("Authorization"), source, items)
	if writeServiceError(w, err) {
		return
	}
	created, duplicates := 0, 0
	for _, v := range results {
		if v.Created {
			created++
		} else {
			duplicates++
		}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"version": 1, "sourceSystem": source, "created": created, "duplicates": duplicates, "items": results})
}

func (h Handler) forecast(w http.ResponseWriter, r *http.Request) {
	p, _ := authorization.ProfileFrom(r.Context())
	v, err := h.Service.Forecast(p)
	if writeServiceError(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"forecast": v})
}

func (h Handler) workflows() OrderWorkflows { w, _ := any(h.Service).(OrderWorkflows); return w }
func (h Handler) tracking(w http.ResponseWriter, r *http.Request) {
	wf := h.workflows()
	if wf == nil {
		apierrors.Internal(w, "tracking unavailable")
		return
	}
	p, _ := authorization.ProfileFrom(r.Context())
	v, err := wf.Tracking(p, chi.URLParam(r, "id"))
	if writeServiceError(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"tracking": v})
}
func (h Handler) pendingReceipts(w http.ResponseWriter, r *http.Request) {
	wf := h.workflows()
	if wf == nil {
		apierrors.Internal(w, "receipts unavailable")
		return
	}
	p, _ := authorization.ProfileFrom(r.Context())
	items, err := wf.PendingReceipts(p)
	if writeServiceError(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}
func (h Handler) receipt(w http.ResponseWriter, r *http.Request) {
	wf := h.workflows()
	if wf == nil {
		apierrors.Internal(w, "receipt unavailable")
		return
	}
	p, _ := authorization.ProfileFrom(r.Context())
	receipt, issues, err := wf.Receipt(p, chi.URLParam(r, "id"))
	if writeServiceError(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"receipt": receipt, "issues": issues})
}
func (h Handler) confirmReceipt(w http.ResponseWriter, r *http.Request) {
	wf := h.workflows()
	if wf == nil {
		apierrors.Internal(w, "receipts unavailable")
		return
	}
	var req domain.ReceiptConfirmation
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierrors.BadRequest(w, "invalid JSON")
		return
	}
	if req.Issue != nil && req.Issue.IdempotencyKey == "" {
		req.Issue.IdempotencyKey = r.Header.Get("Idempotency-Key")
	}
	p, _ := authorization.ProfileFrom(r.Context())
	receipt, issues, created, err := wf.ConfirmReceipt(p, chi.URLParam(r, "id"), req)
	if writeServiceError(w, err) {
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	httpx.WriteJSON(w, status, map[string]any{"receipt": receipt, "issues": issues})
}
func (h Handler) reportReceiptIssue(w http.ResponseWriter, r *http.Request) {
	wf := h.workflows()
	if wf == nil {
		apierrors.Internal(w, "receipts unavailable")
		return
	}
	var req domain.ReceiptIssueRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierrors.BadRequest(w, "invalid JSON")
		return
	}
	if req.IdempotencyKey == "" {
		req.IdempotencyKey = r.Header.Get("Idempotency-Key")
	}
	p, _ := authorization.ProfileFrom(r.Context())
	receipt, issue, created, err := wf.ReportReceiptIssue(p, chi.URLParam(r, "id"), req)
	if writeServiceError(w, err) {
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	httpx.WriteJSON(w, status, map[string]any{"receipt": receipt, "issue": issue})
}
func (h Handler) receiptIssues(w http.ResponseWriter, r *http.Request) {
	wf := h.workflows()
	if wf == nil {
		apierrors.Internal(w, "receipts unavailable")
		return
	}
	p, _ := authorization.ProfileFrom(r.Context())
	items, err := wf.ReceiptIssues(p)
	if writeServiceError(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h Handler) create(w http.ResponseWriter, r *http.Request) {
	var req domain.CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierrors.BadRequest(w, "invalid JSON")
		return
	}
	profile, _ := authorization.ProfileFrom(r.Context())
	order, err := h.Service.Create(profile, r.Header.Get("Authorization"), req)
	if writeServiceError(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"order": order})
}

func (h Handler) list(w http.ResponseWriter, r *http.Request) {
	profile, _ := authorization.ProfileFrom(r.Context())
	q := r.URL.Query()
	items, err := h.Service.List(profile, domain.ListFilter{
		Status:                q.Get("status"),
		Brand:                 q.Get("brand"),
		OutletID:              q.Get("outlet_id"),
		RequestedDeliveryDate: q.Get("requested_delivery_date"),
	})
	if writeServiceError(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h Handler) get(w http.ResponseWriter, r *http.Request) {
	profile, _ := authorization.ProfileFrom(r.Context())
	order, err := h.Service.Get(profile, chi.URLParam(r, "id"))
	if writeServiceError(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"order": order})
}

func writeServiceError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, service.ErrInvalid):
		apierrors.BadRequest(w, err.Error())
	case errors.Is(err, service.ErrForbidden):
		apierrors.Forbidden(w, err.Error())
	case errors.Is(err, service.ErrNotFound):
		apierrors.NotFound(w, err.Error())
	case errors.Is(err, service.ErrUnavailable):
		apierrors.Write(w, http.StatusServiceUnavailable, "Service Unavailable", err.Error())
	case strings.HasPrefix(err.Error(), "conflict:"):
		apierrors.Conflict(w, err.Error())
	default:
		if strings.Contains(err.Error(), "invalid") {
			apierrors.BadRequest(w, err.Error())
			return true
		}
		apierrors.Internal(w, "unexpected error")
	}
	return true
}


func (h Handler) deliveryFollowup(w http.ResponseWriter, r *http.Request) {
    principal, err := h.Authn.Authenticate(r)
    if err != nil { apierrors.Unauthorized(w, err.Error()); return }
    authorized := false
    for _, scope := range principal.Scopes {
        if scope == "orders:write-internal" { authorized = true; break }
    }
    if !authorized { apierrors.Forbidden(w, "internal order scope required"); return }
    var body struct {
        SourceOrderID string `json:"sourceOrderId"`
        StopID string `json:"stopId"`
        TripDate string `json:"tripDate"`
        Units int `json:"units"`
        Resolution string `json:"resolution"`
    }
    dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
    dec.DisallowUnknownFields()
    if err := dec.Decode(&body); err != nil { apierrors.BadRequest(w, "invalid follow-up"); return }
    order, err := h.Service.CreateDeliveryFollowup(body.SourceOrderID, body.StopID, body.TripDate, body.Units, body.Resolution)
    if writeServiceError(w, err) { return }
    httpx.WriteJSON(w, http.StatusOK, map[string]any{"order": order})
}
