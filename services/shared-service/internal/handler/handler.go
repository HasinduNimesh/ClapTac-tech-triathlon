package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/audit"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/auth"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/authorization"
	apierrors "github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/errors"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/httpx"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/telemetry"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/shared-service/internal/store"
)

type Handler struct {
	Authn auth.Authenticator
	Store store.Store
}

func (h Handler) Routes(r chi.Router) {
	r.Route("/api/v1/shared", func(r chi.Router) {
		r.Get("/profiles/me", h.authed(h.me))
		r.Put("/profiles/me/display-name", h.authed(h.updateMyDisplayName))
		r.With(authorization.RequireAnyWith(h.Authn, h.Store, authorization.PermOutletsReadInternal, authorization.PermOrderViewAll, authorization.PermOrderViewOwn, authorization.PermFleetView)).Get("/outlets", h.listOutlets)
		r.With(authorization.RequireAnyWith(h.Authn, h.Store, authorization.PermOutletsReadInternal, authorization.PermOrderViewAll, authorization.PermOrderViewOwn, authorization.PermFleetView)).Get("/outlets/{id}", h.outlet)
		r.With(authorization.RequireWith(h.Authn, h.Store, authorization.PermMasterDataUpdate)).Put("/outlets/{id}", h.updateOutlet)
		r.With(authorization.RequireWith(h.Authn, h.Store, authorization.PermOrderViewOwn)).Post("/outlets/{id}/access-instructions/confirm", h.confirmOutletAccessInstructions)
		r.With(authorization.RequireWith(h.Authn, h.Store, authorization.PermMasterDataUpdate)).Get("/outlets/{id}/notification-preferences", h.notificationPreferences)
		r.With(authorization.RequireWith(h.Authn, h.Store, authorization.PermMasterDataUpdate)).Put("/outlets/{id}/notification-preferences", h.updateNotificationPreferences)
		r.With(authorization.RequireAnyWith(h.Authn, h.Store, authorization.PermMasterDataUpdate, authorization.PermOutletsReadInternal)).Get("/calendar", h.calendar)
		r.With(authorization.RequireWith(h.Authn, h.Store, authorization.PermMasterDataUpdate)).Put("/calendar/{date}", h.updateCalendar)
		r.With(authorization.RequireAnyWith(h.Authn, h.Store, authorization.PermMasterDataUpdate, authorization.PermPolicyReadInternal)).Get("/policies/current", h.currentPolicy)
		r.With(authorization.RequireWith(h.Authn, h.Store, authorization.PermMasterDataUpdate)).Post("/policies/preview", h.previewPolicy)
		r.With(authorization.RequireWith(h.Authn, h.Store, authorization.PermMasterDataUpdate)).Put("/policies/current", h.updatePolicy)
		r.With(authorization.RequireAnyWith(h.Authn, h.Store, authorization.PermOutletsReadInternal, authorization.PermOrderViewAll, authorization.PermFleetView)).Get("/travel", h.travel)
		r.Post("/audit-events", h.auditWrite(h.ingestAudit))
		r.Post("/internal/notifications/enqueue", h.auditWrite(h.enqueueNotification))
		r.Post("/internal/notifications/status", h.scopeWrite("notifications:write", h.updateNotificationStatus))
		r.With(authorization.RequireWith(h.Authn, h.Store, authorization.PermAuditRead)).Get("/audit/events", h.searchAudit)
		r.With(authorization.RequireWith(h.Authn, h.Store, authorization.PermAuditRead)).Get("/audit/kpis", h.auditKPIs)
		dashboards := authorization.RequireWith(h.Authn, h.Store, authorization.PermDashboardManageOwn)
		r.With(dashboards).Get("/dashboards", h.listDashboards)
		r.With(dashboards).Post("/dashboards", h.createDashboard)
		r.With(dashboards).Put("/dashboards/{id}", h.updateDashboard)
		r.With(dashboards).Delete("/dashboards/{id}", h.deleteDashboard)
	})
}

func (h Handler) updateNotificationStatus(w http.ResponseWriter, r *http.Request) {
	var u struct {
		MessageSID string `json:"messageSid"`
		Status     string `json:"status"`
		ErrorCode  string `json:"errorCode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&u); err != nil {
		apierrors.BadRequest(w, "invalid JSON")
		return
	}
	if err := h.Store.UpdateNotificationStatus(r.Context(), u.MessageSID, u.Status, u.ErrorCode); err != nil {
		if strings.Contains(err.Error(), "no rows") {
			apierrors.NotFound(w, "notification not found")
		} else if strings.Contains(err.Error(), "conflict") {
			apierrors.Conflict(w, err.Error())
		} else {
			apierrors.BadRequest(w, "invalid notification status")
		}
		return
	}
	state := strings.ToLower(u.Status)
	switch state {
	case "queued", "accepted", "scheduled", "sending":
		state = "queued"
	case "sent":
		state = "sent"
	case "delivered":
		state = "delivered"
	default:
		state = "failed"
	}
	telemetry.NotificationEvents.WithLabelValues("STATUS_CALLBACK", state).Inc()
	w.WriteHeader(http.StatusNoContent)
}

func (h Handler) enqueueNotification(w http.ResponseWriter, r *http.Request) {
	var e store.NotificationEvent
	if err := json.NewDecoder(r.Body).Decode(&e); err != nil {
		apierrors.BadRequest(w, "invalid JSON")
		return
	}
	e.EventKey = strings.TrimSpace(e.EventKey)
	e.OutletID = strings.TrimSpace(e.OutletID)
	e.OrderRef = strings.TrimSpace(e.OrderRef)
	e.Reason = strings.TrimSpace(e.Reason)
	if e.EventKey == "" || len(e.EventKey) > 200 || e.OutletID == "" || e.OrderRef == "" || len(e.OrderRef) > 80 {
		apierrors.BadRequest(w, "event key, outlet and order reference are required")
		return
	}
	if e.Type == "DEFERRAL" {
		if e.Reason == "" || len(e.Reason) > 80 {
			apierrors.BadRequest(w, "deferral reason is required")
			return
		}
	} else if e.Type == "MAJOR_DELAY" {
		if e.DelayMinutes < 30 || e.DelayMinutes > 1440 {
			apierrors.BadRequest(w, "major delay must be between 30 minutes and 24 hours")
			return
		}
    } else if e.Type == "DELIVERY_REJECTED" {
        if len(strings.TrimSpace(e.Goods)) == 0 || utf8.RuneCountInString(e.Goods) > 200 || e.Units < 1 ||
            e.Reason == "" || len(e.Reason) > 80 || (e.Resolution != "NEXT_RUN" && e.Resolution != "REQUEST_DEFERRAL") {
            apierrors.BadRequest(w, "rejected goods, units, reason and resolution required")
            return
        }
        if _, err := time.Parse(time.DateOnly, e.FollowupDate); err != nil {
            apierrors.BadRequest(w, "valid follow-up date required")
            return
        }
	} else if e.Type == "ARRIVAL_CHANGE" {
		oldETA, oldErr := time.Parse(time.RFC3339Nano, e.OldArrivalAt)
		newETA, newErr := time.Parse(time.RFC3339Nano, e.NewArrivalAt)
		if oldErr != nil || newErr != nil || !arrivalChangeAtLeastThirty(oldETA, newETA) {
			apierrors.BadRequest(w, "arrival change must be at least 30 minutes")
			return
		}
	} else {
		apierrors.BadRequest(w, "unsupported notification type")
		return
	}
	result, err := h.Store.EnqueueNotification(r.Context(), e)
	if err != nil {
		if strings.Contains(err.Error(), "no rows") {
			apierrors.NotFound(w, "outlet notification preferences not found")
		} else {
			apierrors.Internal(w, "notification enqueue failed")
		}
		return
	}
	telemetry.NotificationEvents.WithLabelValues(e.Type, result.Status).Inc()
	status := http.StatusAccepted
	if result.Status == "suppressed" {
		status = http.StatusOK
	}
	httpx.WriteJSON(w, status, map[string]any{"notification": result})
}

var notificationPhone = regexp.MustCompile(`^\+[1-9][0-9]{7,14}$`)

func (h Handler) notificationPreferences(w http.ResponseWriter, r *http.Request) {
	p, err := h.Store.NotificationPreferences(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		apierrors.NotFound(w, "outlet notification preferences not found")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"preferences": p})
}

func (h Handler) updateNotificationPreferences(w http.ResponseWriter, r *http.Request) {
	var p store.NotificationPreferences
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		apierrors.BadRequest(w, "invalid JSON")
		return
	}
	p.OutletID = chi.URLParam(r, "id")
	p.PhoneE164 = strings.TrimSpace(p.PhoneE164)
	p.ConsentedAt = nil
	if !notificationPhone.MatchString(p.PhoneE164) || p.Version < 0 || (p.Locale != "en" && p.Locale != "si" && p.Locale != "ta") || (p.ConsentEnabled && !p.DeferralsEnabled && !p.MajorDelaysEnabled) {
		apierrors.BadRequest(w, "valid E.164 phone, locale, version, consent and at least one alert type are required")
		return
	}
	profile, _ := authorization.ProfileFrom(r.Context())
	actor := ""
	if profile != nil {
		actor = profile.UserID
	}
	saved, err := h.Store.SaveNotificationPreferences(r.Context(), p, p.Version, actor, audit.Event{Action: "OUTLET_NOTIFICATION_PREFERENCES_UPDATED"})
	if err != nil {
		if strings.Contains(err.Error(), "conflict") {
			apierrors.Conflict(w, err.Error())
		} else {
			apierrors.Internal(w, "notification preferences update failed")
		}
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"preferences": saved})
}

func decodePolicy(r *http.Request) (store.PlanningPolicy, error) {
	var p store.PlanningPolicy
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		return p, err
	}
	p.CutoffLocalTime = strings.TrimSpace(p.CutoffLocalTime)
	parsed, err := time.Parse("15:04", p.CutoffLocalTime)
	if err != nil {
		return p, fmt.Errorf("cutoffLocalTime must use HH:MM")
	}
	p.CutoffLocalTime = parsed.Format("15:04")
	if p.DeferralWeightPoints < 1 || p.DeferralWeightPoints > 60 || p.MaxDeferralCount < 1 || p.MaxDeferralCount > 30 || p.MaxUnservedDays < 30 || p.MaxUnservedDays > 730 || p.MaxTripsPerVehicle < 1 || p.MaxTripsPerVehicle > 2 {
		return p, fmt.Errorf("policy values are outside supported safe ranges")
	}
	return p, nil
}

func (h Handler) currentPolicy(w http.ResponseWriter, r *http.Request) {
	p, err := h.Store.CurrentPolicy(r.Context())
	if err != nil {
		apierrors.Internal(w, "current planning policy unavailable")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"policy": p})
}

func (h Handler) previewPolicy(w http.ResponseWriter, r *http.Request) {
	p, err := decodePolicy(r)
	if err != nil {
		apierrors.BadRequest(w, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"preview": p, "examples": []map[string]int{
		{"priorDeferrals": 1, "daysSinceLastServed": 0, "priorityScore": p.DeferralWeightPoints},
		{"priorDeferrals": 0, "daysSinceLastServed": p.MaxUnservedDays, "priorityScore": p.MaxUnservedDays},
		{"priorDeferrals": p.MaxDeferralCount, "daysSinceLastServed": p.MaxUnservedDays, "priorityScore": p.MaxDeferralCount*p.DeferralWeightPoints + p.MaxUnservedDays},
	}, "hardConstraintsUnchanged": true})
}

func (h Handler) updatePolicy(w http.ResponseWriter, r *http.Request) {
	p, err := decodePolicy(r)
	if err != nil {
		apierrors.BadRequest(w, err.Error())
		return
	}
	if p.Version < 1 {
		apierrors.BadRequest(w, "expected policy version is required")
		return
	}
	profile, _ := authorization.ProfileFrom(r.Context())
	actor := ""
	if profile != nil {
		actor = profile.UserID
	}
	p.CreatedBy = actor
	saved, err := h.Store.CreatePolicy(r.Context(), p, p.Version, audit.Event{ActorID: actor, ActorType: "human", Action: "PLANNING_POLICY_VERSION_CREATED", ResourceType: "PLANNING_POLICY", Source: "shared-service"})
	if err != nil {
		if strings.Contains(err.Error(), "conflict") {
			apierrors.Conflict(w, err.Error())
			return
		}
		apierrors.Internal(w, "planning policy update failed")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"policy": saved})
}

func (h Handler) updateOutlet(w http.ResponseWriter, r *http.Request) {
	var req struct {
		store.Outlet
		AccessInstructions *string `json:"accessInstructions"`
		// Location is the outlet's verified position: {"latitude":..,"longitude":..} sets it, null
		// clears it (back to the approximate district position), absent leaves it alone. The
		// latitude/longitude a client echoes from a read are ignored, because on a read they may be
		// approximate.
		Location json.RawMessage `json:"location"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierrors.BadRequest(w, "invalid JSON")
		return
	}
	var location store.LocationChange
	if raw := bytes.TrimSpace(req.Location); len(raw) > 0 {
		if string(raw) == "null" {
			location.Clear = true
		} else {
			var pair struct {
				Latitude  *float64 `json:"latitude"`
				Longitude *float64 `json:"longitude"`
			}
			dec := json.NewDecoder(bytes.NewReader(raw))
			dec.DisallowUnknownFields()
			if err := dec.Decode(&pair); err != nil || pair.Latitude == nil || pair.Longitude == nil {
				apierrors.BadRequest(w, "location needs both a latitude and a longitude, or null to clear it")
				return
			}
			if err := store.ValidateLocation(*pair.Latitude, *pair.Longitude); err != nil {
				apierrors.BadRequest(w, err.Error())
				return
			}
			location = store.LocationChange{Set: true, Latitude: *pair.Latitude, Longitude: *pair.Longitude}
		}
	}
	o := req.Outlet
	o.ID = chi.URLParam(r, "id")
	if req.AccessInstructions != nil {
		o.AccessInstructions = *req.AccessInstructions
	} else if current, err := h.Store.Outlet(r.Context(), o.ID); err == nil {
		// Old clients that predate this field must not erase dispatcher notes.
		o.AccessInstructions = current.AccessInstructions
	} else if strings.Contains(err.Error(), "not found") {
		apierrors.NotFound(w, "outlet not found")
		return
	} else {
		apierrors.Internal(w, "outlet could not be loaded")
		return
	}
	o.Brand = strings.TrimSpace(o.Brand)
	o.Name = strings.TrimSpace(o.Name)
	o.District = strings.TrimSpace(o.District)
	o.Depot = strings.TrimSpace(o.Depot)
	o.DockType = strings.TrimSpace(o.DockType)
	o.ParkingConstraint = strings.TrimSpace(o.ParkingConstraint)
	o.AccessInstructions = strings.TrimSpace(strings.ReplaceAll(o.AccessInstructions, "\r\n", "\n"))
	if utf8.RuneCountInString(o.AccessInstructions) > 1000 || strings.IndexByte(o.AccessInstructions, 0) >= 0 {
		apierrors.BadRequest(w, "access instructions must be at most 1000 characters and contain no null bytes")
		return
	}
	for _, c := range o.AccessInstructions {
		if unicode.IsControl(c) && c != '\n' && c != '\t' {
			apierrors.BadRequest(w, "access instructions contain an unsupported control character")
			return
		}
	}
	if o.Brand == "" || o.Name == "" || o.District == "" || o.Depot == "" || o.Version < 1 || o.DockType != "normal" && o.DockType != "mall_dock" || o.ParkingConstraint != "normal" && o.ParkingConstraint != "van_only" {
		apierrors.BadRequest(w, "complete valid outlet master data and expected version are required")
		return
	}
	if (o.WindowOpenTime == "") != (o.WindowCloseTime == "") {
		apierrors.BadRequest(w, "both delivery-window endpoints are required")
		return
	}
	if o.WindowOpenTime != "" {
		open, e1 := time.Parse("15:04", o.WindowOpenTime)
		close, e2 := time.Parse("15:04", o.WindowCloseTime)
		if e1 != nil || e2 != nil || !open.Before(close) {
			apierrors.BadRequest(w, "delivery window must be valid and close after open")
			return
		}
	}
	if (o.ChilledTemperatureMinC == nil) != (o.ChilledTemperatureMaxC == nil) || (o.ChilledTemperatureMinC != nil && (*o.ChilledTemperatureMinC < -40 || *o.ChilledTemperatureMinC > 40 || *o.ChilledTemperatureMaxC < -40 || *o.ChilledTemperatureMaxC > 40 || *o.ChilledTemperatureMinC >= *o.ChilledTemperatureMaxC)) {
		apierrors.BadRequest(w, "chilled temperature range must provide both limits, between -40 and 40 °C, with minimum below maximum")
		return
	}
	profile, _ := authorization.ProfileFrom(r.Context())
	actor := ""
	if profile != nil {
		actor = profile.UserID
	}
	updated, err := h.Store.UpdateOutletLocation(r.Context(), o, o.Version, location, audit.Event{ActorID: actor, ActorType: "human", Action: "MASTER_DATA_OUTLET_UPDATED", ResourceType: "OUTLET", ResourceID: o.ID, Source: "shared-service"})
	if err != nil {
		if strings.Contains(err.Error(), "conflict") {
			apierrors.Conflict(w, err.Error())
		} else if strings.Contains(err.Error(), "no rows") {
			apierrors.NotFound(w, "outlet not found")
		} else {
			apierrors.Internal(w, "outlet update failed")
		}
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"outlet": updated})
}

func (h Handler) confirmOutletAccessInstructions(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ExpectedVersion int `json:"expectedVersion"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ExpectedVersion < 1 {
		apierrors.BadRequest(w, "expected outlet version is required")
		return
	}
	profile, ok := authorization.ProfileFrom(r.Context())
	if !ok || !authorization.HasPermission(profile.Roles, authorization.PermOrderViewOwn) || authorization.HasPermission(profile.Roles, authorization.PermOrderViewAll) {
		apierrors.Forbidden(w, "only a store manager can confirm outlet access instructions")
		return
	}
	outletID := chi.URLParam(r, "id")
	assigned := false
	for _, id := range profile.OutletIDs {
		if id == outletID {
			assigned = true
			break
		}
	}
	if !assigned {
		apierrors.Forbidden(w, "store manager may only confirm their assigned outlet")
		return
	}
	saved, err := h.Store.ConfirmOutletAccessInstructions(r.Context(), outletID, req.ExpectedVersion, audit.Event{ActorID: profile.UserID, ActorType: "human", Source: "shared-service"})
	if err != nil {
		if strings.Contains(err.Error(), "conflict") {
			apierrors.Conflict(w, err.Error())
		} else if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "no rows") {
			apierrors.NotFound(w, "outlet not found")
		} else if strings.Contains(err.Error(), "invalid") {
			apierrors.BadRequest(w, err.Error())
		} else {
			apierrors.Internal(w, "access instructions confirmation failed")
		}
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"outlet": saved})
}

func (h Handler) calendar(w http.ResponseWriter, r *http.Request) {
	fromRaw, toRaw := r.URL.Query().Get("from"), r.URL.Query().Get("to")
	from, e1 := time.Parse(time.DateOnly, fromRaw)
	to, e2 := time.Parse(time.DateOnly, toRaw)
	if e1 != nil || e2 != nil || to.Before(from) || to.Sub(from) > 366*24*time.Hour {
		apierrors.BadRequest(w, "valid from and to dates covering at most one year are required")
		return
	}
	items, err := h.Store.Calendar(r.Context(), fromRaw, toRaw)
	if err != nil {
		apierrors.Internal(w, "calendar unavailable")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h Handler) updateCalendar(w http.ResponseWriter, r *http.Request) {
	date := chi.URLParam(r, "date")
	if _, err := time.Parse(time.DateOnly, date); err != nil {
		apierrors.BadRequest(w, "date must be YYYY-MM-DD")
		return
	}
	var body struct {
		IsOperating     bool `json:"isOperating"`
		ExpectedVersion int  `json:"expectedVersion"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierrors.BadRequest(w, "invalid JSON")
		return
	}
	if body.ExpectedVersion < 0 {
		apierrors.BadRequest(w, "expectedVersion must be zero or greater")
		return
	}
	profile, _ := authorization.ProfileFrom(r.Context())
	actor := ""
	if profile != nil {
		actor = profile.UserID
	}
	day, err := h.Store.UpdateCalendar(r.Context(), store.CalendarDay{Date: date, IsOperating: body.IsOperating}, body.ExpectedVersion, audit.Event{ActorID: actor, ActorType: "human", Action: "MASTER_DATA_CALENDAR_UPDATED", ResourceType: "CALENDAR", ResourceID: date, Source: "shared-service"})
	if err != nil {
		if strings.Contains(err.Error(), "conflict") {
			apierrors.Conflict(w, err.Error())
		} else {
			apierrors.Internal(w, "calendar update failed")
		}
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"calendarDay": day})
}

func (h Handler) searchAudit(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit := 50
	if raw := q.Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 100 {
			apierrors.BadRequest(w, "limit must be between 1 and 100")
			return
		}
		limit = parsed
	}
	offset := 0
	if raw := q.Get("offset"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 0 || parsed > 10000 {
			apierrors.BadRequest(w, "offset must be between 0 and 10000")
			return
		}
		offset = parsed
	}
	filter := store.AuditFilter{Query: q.Get("q"), Action: q.Get("action"), ResourceType: q.Get("resourceType"), ResourceID: q.Get("resourceId"), ActorID: q.Get("actorId"), Limit: limit, Offset: offset}
	var err error
	if raw := q.Get("from"); raw != "" {
		filter.From, err = time.Parse(time.RFC3339, raw)
		if err != nil {
			apierrors.BadRequest(w, "from must be RFC3339")
			return
		}
	}
	if raw := q.Get("to"); raw != "" {
		filter.To, err = time.Parse(time.RFC3339, raw)
		if err != nil {
			apierrors.BadRequest(w, "to must be RFC3339")
			return
		}
	}
	if !filter.From.IsZero() && !filter.To.IsZero() && filter.To.Before(filter.From) {
		apierrors.BadRequest(w, "to must be after from")
		return
	}
	items, total, err := h.Store.SearchAudit(r.Context(), filter)
	if err != nil {
		apierrors.Internal(w, "audit search failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items, "total": total, "limit": limit, "offset": offset})
}

func (h Handler) auditKPIs(w http.ResponseWriter, r *http.Request) {
	to := time.Now().UTC()
	from := to.AddDate(0, 0, -7)
	var err error
	if raw := r.URL.Query().Get("from"); raw != "" {
		from, err = time.Parse(time.RFC3339, raw)
		if err != nil {
			apierrors.BadRequest(w, "from must be RFC3339")
			return
		}
	}
	if raw := r.URL.Query().Get("to"); raw != "" {
		to, err = time.Parse(time.RFC3339, raw)
		if err != nil {
			apierrors.BadRequest(w, "to must be RFC3339")
			return
		}
	}
	if to.Before(from) || to.Sub(from) > 90*24*time.Hour {
		apierrors.BadRequest(w, "KPI range must be ordered and no longer than 90 days")
		return
	}
	result, err := h.Store.AuditKPIs(r.Context(), from, to)
	if err != nil {
		apierrors.Internal(w, "audit KPIs unavailable")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, result)
}

func (h Handler) authed(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, err := h.Authn.Authenticate(r)
		if err != nil {
			apierrors.Unauthorized(w, err.Error())
			return
		}
		next(w, r.WithContext(auth.WithPrincipal(r.Context(), p)))
	}
}

func (h Handler) auditWrite(next http.HandlerFunc) http.HandlerFunc {
	return h.scopeWrite(authorization.PermAuditWrite, next)
}

func (h Handler) scopeWrite(scope string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, err := h.Authn.Authenticate(r)
		if err != nil {
			apierrors.Unauthorized(w, err.Error())
			return
		}
		if !hasScope(p.Scopes, scope) {
			apierrors.Forbidden(w, "missing required service scope")
			return
		}
		next(w, r.WithContext(auth.WithPrincipal(r.Context(), p)))
	}
}

func (h Handler) me(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFrom(r.Context())
	profile, err := h.Store.ProfileBySubject(r.Context(), p.Subject)
	if err != nil {
		apierrors.NotFound(w, "application user not found")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"profile": profile})
}

func (h Handler) updateMyDisplayName(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFrom(r.Context())
	var req struct {
		DisplayName string `json:"displayName"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		apierrors.BadRequest(w, "invalid display name request")
		return
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		apierrors.BadRequest(w, "invalid display name request")
		return
	}
	name := strings.TrimSpace(req.DisplayName)
	if name == "" || utf8.RuneCountInString(name) > 120 {
		apierrors.BadRequest(w, "display name must be between 1 and 120 characters")
		return
	}
	for _, char := range name {
		if unicode.IsControl(char) {
			apierrors.BadRequest(w, "display name contains a control character")
			return
		}
	}
	if err := h.Store.SetDisplayName(r.Context(), p.Subject, name); err != nil {
		if strings.Contains(err.Error(), "not found") {
			apierrors.NotFound(w, "application user not found")
		} else {
			apierrors.Internal(w, "display name could not be saved")
		}
		return
	}
	profile, err := h.Store.ProfileBySubject(r.Context(), p.Subject)
	if err != nil {
		apierrors.Internal(w, "profile could not be loaded")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"profile": profile})
}

func (h Handler) listOutlets(w http.ResponseWriter, r *http.Request) {
	items, err := h.Store.ListOutlets(r.Context())
	if err != nil {
		apierrors.Internal(w, "list outlets failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h Handler) outlet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	o, err := h.Store.Outlet(r.Context(), id)
	if err != nil {
		apierrors.NotFound(w, "outlet not found")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"outlet": o})
}

func (h Handler) travel(w http.ResponseWriter, r *http.Request) {
	rows, err := h.Store.TravelRows(r.Context())
	if err != nil {
		apierrors.Internal(w, "travel table failed")
		return
	}
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		items = append(items, map[string]any{"from": row[0], "to": row[1], "km": row[2], "minutes": row[3]})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"items":          items,
		"defaultService": h.Store.ServiceMinutes(r.Context(), "default"),
		"mallService":    h.Store.ServiceMinutes(r.Context(), "mall"),
	})
}

func (h Handler) ingestAudit(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierrors.BadRequest(w, "invalid JSON")
		return
	}
	ev := audit.Event{
		EventID:       str(body["eventId"]),
		CorrelationID: str(body["correlationId"]),
		ActorID:       str(body["actorId"]),
		ActorType:     str(body["actorType"]),
		Action:        str(body["action"]),
		ResourceType:  str(body["resourceType"]),
		ResourceID:    str(body["resourceId"]),
		Source:        str(body["source"]),
		Timestamp:     time.Now().UTC(),
	}
	if ns, ok := body["newState"].(map[string]any); ok {
		ev.NewState = ns
	}
	if err := h.Store.InsertAudit(r.Context(), ev); err != nil {
		apierrors.Internal(w, "audit persist failed")
		return
	}
	httpx.WriteJSON(w, http.StatusAccepted, map[string]any{"status": "ingested"})
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func hasScope(scopes []string, want string) bool {
	for _, s := range scopes {
		if s == want {
			return true
		}
	}
	return false
}

func arrivalChangeAtLeastThirty(oldETA, newETA time.Time) bool {
	delta := newETA.Sub(oldETA)
	return delta >= 30*time.Minute || delta <= -30*time.Minute
}
