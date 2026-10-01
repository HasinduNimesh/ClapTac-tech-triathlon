package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/auth"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/authorization"
	apierrors "github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/errors"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/httpx"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/fleet-service/internal/domain"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/fleet-service/internal/service"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/fleet-service/internal/store"
)

type Handler struct {
	Authn    auth.Authenticator
	Profiles authorization.ProfileResolver
	Store    store.Store
}

func (h Handler) Routes(r chi.Router) {
	r.Route("/api/v1/fleet", func(r chi.Router) {
		r.With(authorization.RequireAnyWith(h.Authn, h.Profiles, authorization.PermFleetView, authorization.PermFleetReadInternal)).Get("/vehicles", h.list)
		r.With(authorization.RequireAnyWith(h.Authn, h.Profiles, authorization.PermFleetView, authorization.PermFleetReadInternal)).Get("/vehicles/{id}", h.get)
		r.With(authorization.RequireAnyWith(h.Authn, h.Profiles, authorization.PermFleetView, authorization.PermFleetReadInternal)).Get("/availability", h.availability)
		r.With(authorization.RequireWith(h.Authn, h.Profiles, authorization.PermFleetUpdate)).Put("/vehicles/{id}/availability", h.putAvailability)
		r.With(authorization.RequireAnyWith(h.Authn, h.Profiles, authorization.PermFleetView, authorization.PermFleetReadInternal)).Get("/incidents", h.listIncidents)
		r.With(authorization.RequireWith(h.Authn, h.Profiles, authorization.PermFleetUpdate)).Post("/incidents", h.createIncident)
		r.With(authorization.RequireWith(h.Authn, h.Profiles, authorization.PermFleetUpdate)).Put("/vehicles/{id}/master-data", h.updateMasterData)
		r.With(authorization.RequireWith(h.Authn, h.Profiles, authorization.PermFleetUpdate)).Post("/fuel/entries", h.recordFuel)
		r.With(authorization.RequireAnyWith(h.Authn, h.Profiles, authorization.PermFleetView, authorization.PermFleetReadInternal)).Get("/fuel/ledger", h.fuelLedger)
	})
}

func (h Handler) createIncident(w http.ResponseWriter, r *http.Request) {
	var body domain.VehicleIncident
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierrors.BadRequest(w, "invalid JSON")
		return
	}
	body.VehicleID = strings.TrimSpace(body.VehicleID)
	body.TripID = strings.TrimSpace(body.TripID)
	body.Description = strings.TrimSpace(body.Description)
	location := time.FixedZone("Asia/Colombo", 5*3600+30*60)
	if body.VehicleID == "" || body.Description == "" || len(body.Description) > 1000 || len(body.AffectedStops) > 100 || body.Type != "breakdown" && body.Type != "accident" && body.Type != "temperature_failure" && body.Type != "other" {
		apierrors.BadRequest(w, "vehicle, incident type, concise description, and up to 100 affected stops are required")
		return
	}
	date, err := time.ParseInLocation(time.DateOnly, body.Date, location)
	if err != nil {
		apierrors.BadRequest(w, "valid local incident date is required")
		return
	}
	body.Date = date.Format(time.DateOnly)
	profile, _ := authorization.ProfileFrom(r.Context())
	if profile != nil {
		body.ReportedBy = profile.UserID
	}
	incident, err := h.Store.CreateIncident(r.Context(), body)
	if err != nil {
		if strings.Contains(err.Error(), "foreign key") {
			apierrors.NotFound(w, "vehicle not found")
		} else {
			apierrors.Internal(w, "incident could not be recorded")
		}
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"incident": incident, "vehicleStatus": "unavailable"})
}

func (h Handler) listIncidents(w http.ResponseWriter, r *http.Request) {
	items, err := h.Store.Incidents(r.Context(), r.URL.Query().Get("openOnly") == "true")
	if err != nil {
		apierrors.Internal(w, "incident list unavailable")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h Handler) updateMasterData(w http.ResponseWriter, r *http.Request) {
	var v domain.Vehicle
	if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
		apierrors.BadRequest(w, "invalid JSON")
		return
	}
	v.ID = chi.URLParam(r, "id")
	if v.Version < 1 || v.Type != "truck" && v.Type != "van" || v.Temp != "ambient" && v.Temp != "reefer" || v.FuelType == "" || v.HomeDepot == "" || v.WeightCapacityKg <= 0 || v.VolumeCapacityM3 <= 0 || v.KmPerL <= 0 || v.WeeklyFuelQuotaL <= 0 {
		apierrors.BadRequest(w, "vehicle capabilities, positive limits, home depot, and expected version are required")
		return
	}
	profile, _ := authorization.ProfileFrom(r.Context())
	actor := ""
	if profile != nil {
		actor = profile.UserID
	}
	updated, err := h.Store.UpdateMasterData(r.Context(), v, v.Version, actor)
	if err != nil {
		if strings.Contains(err.Error(), "conflict") {
			apierrors.Conflict(w, err.Error())
		} else if strings.Contains(err.Error(), "no rows") {
			apierrors.NotFound(w, "vehicle not found")
		} else {
			apierrors.Internal(w, "vehicle update failed")
		}
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"vehicle": updated, "audit": "queued"})
}

func (h Handler) recordFuel(w http.ResponseWriter, r *http.Request) {
	var body struct {
		VehicleID  string  `json:"vehicleId"`
		Date       string  `json:"date"`
		Liters     float64 `json:"liters"`
		ReceiptRef string  `json:"receiptRef"`
		Note       string  `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierrors.BadRequest(w, "invalid JSON")
		return
	}
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" || len(key) > 200 {
		apierrors.BadRequest(w, "Idempotency-Key is required and must be at most 200 characters")
		return
	}
	body.VehicleID = strings.TrimSpace(body.VehicleID)
	body.Date = strings.TrimSpace(body.Date)
	body.ReceiptRef = strings.TrimSpace(body.ReceiptRef)
	body.Note = strings.TrimSpace(body.Note)
	location := time.FixedZone("Asia/Colombo", 5*60*60+30*60)
	day, err := time.ParseInLocation(time.DateOnly, body.Date, location)
	if body.VehicleID == "" || err != nil {
		apierrors.BadRequest(w, "vehicleId and a valid date are required")
		return
	}
	localNow := time.Now().In(location)
	today := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, location)
	if day.After(today) {
		apierrors.BadRequest(w, "fuel consumption cannot be recorded for a future date")
		return
	}
	if body.Liters <= 0 || body.Liters > 10000 || body.ReceiptRef == "" && body.Note == "" || len(body.ReceiptRef) > 120 || len(body.Note) > 1000 {
		apierrors.BadRequest(w, "liters must be between 0 and 10000 and a receipt reference or note is required")
		return
	}
	if _, err := h.Store.Get(r.Context(), body.VehicleID); err != nil {
		apierrors.NotFound(w, "vehicle not found")
		return
	}
	profile, _ := authorization.ProfileFrom(r.Context())
	actor := ""
	if profile != nil {
		actor = profile.UserID
	}
	entry, err := h.Store.RecordFuel(r.Context(), domain.FuelEntry{
		VehicleID: body.VehicleID, Date: body.Date, Liters: body.Liters,
		ReceiptRef: body.ReceiptRef, Note: body.Note, RecordedBy: actor,
	}, key)
	if err != nil {
		if strings.Contains(err.Error(), "conflict") {
			apierrors.Conflict(w, err.Error())
			return
		}
		apierrors.Internal(w, "fuel entry could not be recorded")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"entry": entry})
}

func (h Handler) fuelLedger(w http.ResponseWriter, r *http.Request) {
	weekOf := r.URL.Query().Get("weekOf")
	weekStart, weekEnd, err := service.ISOWeekRange(weekOf)
	if err != nil {
		apierrors.BadRequest(w, err.Error())
		return
	}
	ledger, err := h.Store.FuelLedger(r.Context(), weekOf, weekStart, weekEnd)
	if err != nil {
		apierrors.Internal(w, "fuel ledger unavailable")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, ledger)
}

func (h Handler) list(w http.ResponseWriter, r *http.Request) {
	items, err := h.Store.List(r.Context())
	if err != nil {
		apierrors.Internal(w, "list vehicles failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h Handler) get(w http.ResponseWriter, r *http.Request) {
	v, err := h.Store.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		apierrors.NotFound(w, "vehicle not found")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"vehicle": v})
}

func (h Handler) availability(w http.ResponseWriter, r *http.Request) {
	date := r.URL.Query().Get("date")
	if date == "" {
		apierrors.BadRequest(w, "date is required")
		return
	}
	items, err := h.Store.Availability(r.Context(), date)
	if err != nil {
		apierrors.Internal(w, "availability failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h Handler) putAvailability(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Date   string `json:"date"`
		Status string `json:"status"`
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierrors.BadRequest(w, "invalid JSON")
		return
	}
	if body.Date == "" || body.Status == "" {
		apierrors.BadRequest(w, "date and status are required")
		return
	}
	if body.Status != "available" && body.Status != "unavailable" && body.Status != "in_workshop" {
		apierrors.BadRequest(w, "invalid status")
		return
	}
	a := domain.Availability{VehicleID: chi.URLParam(r, "id"), Date: body.Date, Status: body.Status, Reason: body.Reason}
	if err := h.Store.UpsertAvailability(r.Context(), a); err != nil {
		apierrors.Internal(w, "update availability failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"availability": a})
}
