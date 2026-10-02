package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/telemetry"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/domain"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/travel"
)

type TokenSource interface {
	Token(ctx context.Context) (string, error)
}

type Peers struct {
	OrdersURL   string
	FleetURL    string
	SharedURL   string
	DeliveryURL string
	M2M         TokenSource
	HTTP        *http.Client
	Logger      *slog.Logger
}

func (p Peers) OutletLastServed(ctx context.Context) (map[string]time.Time, error) {
	tok, err := p.m2m(ctx)
	if err != nil {
		return nil, err
	}
	var result struct {
		Items []struct {
			OutletID     string    `json:"outletId"`
			LastServedAt time.Time `json:"lastServedAt"`
		} `json:"items"`
	}
	if err := p.getJSON(ctx, p.DeliveryURL+"/api/v1/delivery/internal/outlets/last-served", tok, &result); err != nil {
		return nil, err
	}
	items := make(map[string]time.Time, len(result.Items))
	for _, item := range result.Items {
		items[item.OutletID] = item.LastServedAt
	}
	return items, nil
}

// OutletLastAttempted covers every terminal delivery outcome, not just
// successful ones - FR-53's repeat-deferral warning needs this to avoid
// claiming an outlet was deferred on its last run when it was actually
// attempted (and failed) instead.
func (p Peers) OutletLastAttempted(ctx context.Context, beforeDate string) (map[string]time.Time, error) {
	tok, err := p.m2m(ctx)
	if err != nil {
		return nil, err
	}
	var result struct {
		Items []struct {
			OutletID        string    `json:"outletId"`
			LastAttemptedAt time.Time `json:"lastAttemptedAt"`
		} `json:"items"`
	}
	path := p.DeliveryURL + "/api/v1/delivery/internal/outlets/last-attempted?before=" + url.QueryEscape(beforeDate)
	if err := p.getJSON(ctx, path, tok, &result); err != nil {
		return nil, err
	}
	items := make(map[string]time.Time, len(result.Items))
	for _, item := range result.Items {
		items[item.OutletID] = item.LastAttemptedAt
	}
	return items, nil
}

func (p Peers) http() *http.Client {
	if p.HTTP != nil {
		return p.HTTP
	}
	return http.DefaultClient
}

func (p Peers) m2m(ctx context.Context) (string, error) {
	if p.M2M == nil {
		return "", nil
	}
	return p.M2M.Token(ctx)
}

func (p Peers) getJSON(ctx context.Context, url, token string, dest any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := p.http().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("%s: %s", url, string(body))
	}
	return json.Unmarshal(body, dest)
}

// QueueNotification asks shared-service to apply the outlet's consent and alert
// preferences before it snapshots a message into its deduplicated outbox.
func (p Peers) QueueNotification(ctx context.Context, eventKey, outletID, kind, orderRef, reason string, delayMinutes int) error {
	tok, err := p.m2m(ctx)
	if err != nil {
		return err
	}
	body, _ := json.Marshal(map[string]any{"eventKey": eventKey, "outletId": outletID, "type": kind, "orderRef": orderRef, "reason": reason, "delayMinutes": delayMinutes})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.SharedURL+"/api/v1/shared/internal/notifications/enqueue", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := p.http().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("shared notification enqueue returned HTTP %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

func (p Peers) Orders(ctx context.Context, date string) ([]domain.Order, error) {
	tok, err := p.m2m(ctx)
	if err != nil {
		return nil, err
	}
	var out struct {
		Items []struct {
			ID, OrderRef, OutletID, Brand, TemperatureRequirement, RequestedDeliveryDate string
			OrderWeightKg, OrderVolumeM3                                                 float64
		} `json:"items"`
	}
	if err := p.getJSON(ctx, p.OrdersURL+"/api/v1/orders?status=confirmed&requested_delivery_date="+date, tok, &out); err != nil {
		return nil, err
	}
	var orders []domain.Order
	for _, it := range out.Items {
		orders = append(orders, domain.Order{
			ID: it.ID, OrderRef: it.OrderRef, OutletID: it.OutletID, Brand: it.Brand,
			Temp: it.TemperatureRequirement, WeightKg: it.OrderWeightKg, VolumeM3: it.OrderVolumeM3,
		})
	}
	return orders, nil
}

func (p Peers) Vehicles(ctx context.Context, date string) ([]domain.Vehicle, error) {
	tok, err := p.m2m(ctx)
	if err != nil {
		return nil, err
	}
	var list struct {
		Items []struct {
			ID, Type, Temp, HomeDepot                                    string
			WeightCapacityKg, VolumeCapacityM3, KmPerL, WeeklyFuelQuotaL float64
		} `json:"items"`
	}
	if err := p.getJSON(ctx, p.FleetURL+"/api/v1/fleet/vehicles", tok, &list); err != nil {
		return nil, err
	}
	var avail struct {
		Items []struct {
			VehicleID, Status string
		} `json:"items"`
	}
	_ = p.getJSON(ctx, p.FleetURL+"/api/v1/fleet/availability?date="+date, tok, &avail)
	st := map[string]string{}
	for _, a := range avail.Items {
		st[a.VehicleID] = a.Status
	}
	var out []domain.Vehicle
	for _, v := range list.Items {
		status := st[v.ID]
		if status == "" {
			status = "available"
		}
		out = append(out, domain.Vehicle{
			ID: v.ID, Type: v.Type, Temp: v.Temp, HomeDepot: v.HomeDepot, Status: status,
			WeightCap: v.WeightCapacityKg, VolumeCap: v.VolumeCapacityM3, KmPerL: v.KmPerL, WeeklyQuotaL: v.WeeklyFuelQuotaL,
		})
	}
	return out, nil
}

func (p Peers) Vehicle(ctx context.Context, id string) (domain.Vehicle, error) {
	tok, err := p.m2m(ctx)
	if err != nil {
		return domain.Vehicle{}, err
	}
	var out struct {
		Vehicle domain.Vehicle `json:"vehicle"`
	}
	if err := p.getJSON(ctx, p.FleetURL+"/api/v1/fleet/vehicles/"+url.PathEscape(id), tok, &out); err != nil {
		return domain.Vehicle{}, err
	}
	return out.Vehicle, nil
}

func (p Peers) ActualFuelByVehicle(ctx context.Context, weekOf string) (map[string]float64, error) {
	tok, err := p.m2m(ctx)
	if err != nil {
		return nil, err
	}
	var result struct {
		Items []struct {
			VehicleID     string  `json:"vehicleId"`
			ActualLitersL float64 `json:"actualLitersL"`
		} `json:"items"`
	}
	if err := p.getJSON(ctx, p.FleetURL+"/api/v1/fleet/fuel/ledger?weekOf="+weekOf, tok, &result); err != nil {
		return nil, err
	}
	items := make(map[string]float64, len(result.Items))
	for _, item := range result.Items {
		items[item.VehicleID] = item.ActualLitersL
	}
	return items, nil
}

func (p Peers) PlanningPolicy(ctx context.Context) (domain.PlanningPolicy, error) {
	tok, err := p.m2m(ctx)
	if err != nil {
		return domain.PlanningPolicy{}, err
	}
	var result struct {
		Policy domain.PlanningPolicy `json:"policy"`
	}
	if err := p.getJSON(ctx, p.SharedURL+"/api/v1/shared/policies/current", tok, &result); err != nil {
		return domain.PlanningPolicy{}, err
	}
	return result.Policy, nil
}

func (p Peers) Outlets(ctx context.Context) (map[string]domain.Outlet, error) {
	tok, err := p.m2m(ctx)
	if err != nil {
		return nil, err
	}
	var out struct {
		Items []struct {
			ID, Brand, District, Depot, ParkingConstraint, WindowOpenTime, WindowCloseTime string
			MallWindow                                                                     bool
		} `json:"items"`
	}
	if err := p.getJSON(ctx, p.SharedURL+"/api/v1/shared/outlets", tok, &out); err != nil {
		return nil, err
	}
	m := map[string]domain.Outlet{}
	for _, o := range out.Items {
		m[o.ID] = domain.Outlet{
			ID: o.ID, Brand: o.Brand, District: o.District, Depot: o.Depot,
			ParkingConstraint: o.ParkingConstraint, MallWindow: o.MallWindow,
			WindowOpen: o.WindowOpenTime, WindowClose: o.WindowCloseTime,
		}
	}
	return m, nil
}

func (p Peers) Estimator(ctx context.Context) travel.Estimator {
	tok, _ := p.m2m(ctx)
	var body struct {
		Items []struct {
			From, To, Km, Minutes string
		} `json:"items"`
		DefaultService, MallService int
	}
	if err := p.getJSON(ctx, p.SharedURL+"/api/v1/shared/travel", tok, &body); err != nil || len(body.Items) == 0 {
		return travel.New(nil, 15, 20)
	}
	legs := map[string]travel.Leg{}
	for _, it := range body.Items {
		legs[travel.Key(it.From, it.To)] = travel.Leg{Km: travel.ParseKm(it.Km), Minutes: travel.ParseMin(it.Minutes)}
	}
	return travel.New(legs, body.DefaultService, body.MallService)
}

func (p Peers) Resolve(ctx context.Context, subject string) (*struct {
	UserID string
	Roles  []string
}, error) {
	return nil, nil
}

func (p Peers) Publish(ctx context.Context, action, actor, resource, resourceID string, state map[string]any) {
	tok, err := p.m2m(ctx)
	if err != nil {
		telemetry.AuditPublishFailures.Inc()
		if p.Logger != nil {
			p.Logger.Error("audit_token_failed", "action", action, "error", err)
		}
		return
	}
	body, _ := json.Marshal(map[string]any{
		"action": action, "actorId": actor, "actorType": "user",
		"resourceType": resource, "resourceId": resourceID, "newState": state, "source": "planning-service",
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.SharedURL+"/api/v1/shared/audit-events", bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := p.http().Do(req)
	if err != nil || resp.StatusCode >= 300 {
		telemetry.AuditPublishFailures.Inc()
		if p.Logger != nil {
			p.Logger.Error("audit_publish_failed", "action", action, "error", err)
		}
		go p.retry(action, actor, resource, resourceID, state)
		return
	}
	if resp != nil {
		_ = resp.Body.Close()
	}
}

func (p Peers) retry(action, actor, resource, resourceID string, state map[string]any) {
	time.Sleep(200 * time.Millisecond)
	p.Publish(context.Background(), action, actor, resource, resourceID, state)
}
