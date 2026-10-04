package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/auth"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/authorization"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/telemetry"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/delivery-service/internal/domain"
)

type TokenSource interface {
	Token(ctx context.Context) (string, error)
}

type Peers struct {
	LoadingURL string
    OrdersURL string
	SharedURL  string
	M2M        TokenSource
	HTTP       *http.Client
	Logger     *slog.Logger
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

func (p Peers) ReadyTrips(ctx context.Context, date, vehicleID string) ([]domain.LoadingTrip, error) {
	tok, err := p.m2m(ctx)
	if err != nil {
		return nil, err
	}
	q := "?date=" + date
	if vehicleID != "" {
		q += "&vehicleId=" + vehicleID
	}
	var out struct {
		Items []domain.LoadingTrip `json:"items"`
	}
	if err := p.getJSON(ctx, p.LoadingURL+"/api/v1/loading/internal/trips"+q, tok, &out); err != nil {
		return nil, err
	}
	if out.Items == nil {
		out.Items = []domain.LoadingTrip{}
	}
	return out.Items, nil
}

func (p Peers) ReadyTrip(ctx context.Context, tripID string) (domain.LoadingTrip, error) {
	tok, err := p.m2m(ctx)
	if err != nil {
		return domain.LoadingTrip{}, err
	}
	var trip domain.LoadingTrip
	if err := p.getJSON(ctx, p.LoadingURL+"/api/v1/loading/internal/trips/"+tripID, tok, &trip); err != nil {
		return domain.LoadingTrip{}, err
	}
	return trip, nil
}

func (p Peers) QueueArrivalChange(ctx context.Context, eventKey, outletID, orderRef string, oldETA, newETA time.Time) (string, error) {
	tok, err := p.m2m(ctx)
	if err != nil { return "", err }
	body, err := json.Marshal(map[string]any{
		"eventKey": eventKey, "outletId": outletID, "orderRef": orderRef,
		"type": "ARRIVAL_CHANGE", "oldArrivalAt": oldETA, "newArrivalAt": newETA,
	})
	if err != nil { return "", err }
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.SharedURL+"/api/v1/shared/internal/notifications/enqueue", bytes.NewReader(body))
	if err != nil { return "", err }
	req.Header.Set("Content-Type", "application/json")
	if tok != "" { req.Header.Set("Authorization", "Bearer "+tok) }
	resp, err := p.http().Do(req)
	if err != nil { return "", err }
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("arrival notification enqueue returned HTTP %d: %s", resp.StatusCode, message)
	}
	var result struct { Notification struct { Status string `json:"status"` } `json:"notification"` }
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&result); err != nil { return "", err }
	return result.Notification.Status, nil
}

func (p Peers) Outlet(ctx context.Context, id string) (domain.Outlet, error) {
	tok, err := p.m2m(ctx)
	if err != nil {
		return domain.Outlet{}, err
	}
	var out struct {
		Outlet domain.Outlet `json:"outlet"`
	}
	if err := p.getJSON(ctx, p.SharedURL+"/api/v1/shared/outlets/"+id, tok, &out); err != nil {
		return domain.Outlet{}, err
	}
	return out.Outlet, nil
}

// outletPositions keeps the outlet list for a short time so serving trips does not ask shared-service for
// every outlet on every request. A corrected position reaches drivers within the TTL.
var outletPositions struct {
	sync.Mutex
	url  string
	at   time.Time
	byID map[string]domain.Outlet
}

const outletPositionsTTL = 30 * time.Second

// Outlets returns every outlet by id, including each one's position.
func (p Peers) Outlets(ctx context.Context) (map[string]domain.Outlet, error) {
	outletPositions.Lock()
	defer outletPositions.Unlock()
	if outletPositions.byID != nil && outletPositions.url == p.SharedURL && time.Since(outletPositions.at) < outletPositionsTTL {
		return outletPositions.byID, nil
	}
	tok, err := p.m2m(ctx)
	if err != nil {
		return nil, err
	}
	var out struct {
		Items []domain.Outlet `json:"items"`
	}
	if err := p.getJSON(ctx, p.SharedURL+"/api/v1/shared/outlets", tok, &out); err != nil {
		return nil, err
	}
	byID := make(map[string]domain.Outlet, len(out.Items))
	for _, o := range out.Items {
		byID[o.ID] = o
	}
	outletPositions.url, outletPositions.at, outletPositions.byID = p.SharedURL, time.Now(), byID
	return byID, nil
}

type Profiles struct {
	Shared Peers
}

func (p Profiles) Resolve(ctx context.Context, subject string) (*authorization.Profile, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.Shared.SharedURL+"/api/v1/shared/profiles/me", nil)
	if err != nil {
		return nil, err
	}
	if b := auth.BearerFrom(ctx); b != "" {
		req.Header.Set("Authorization", b)
	}
	resp, err := p.Shared.http().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return &authorization.Profile{Subject: subject}, nil
	}
	var out struct {
		Profile authorization.Profile `json:"profile"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if out.Profile.Subject == "" {
		out.Profile.Subject = subject
	}
	return &out.Profile, nil
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
		"resourceType": resource, "resourceId": resourceID, "newState": state, "source": "delivery-service",
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
		if resp != nil {
			_ = resp.Body.Close()
		}
		telemetry.AuditPublishFailures.Inc()
		if p.Logger != nil {
			p.Logger.Error("audit_publish_failed", "action", action, "error", err)
		}
		go func() {
			time.Sleep(200 * time.Millisecond)
			p.publishOnce(context.Background(), action, actor, resource, resourceID, state)
		}()
		return
	}
	_ = resp.Body.Close()
}

func (p Peers) publishOnce(ctx context.Context, action, actor, resource, resourceID string, state map[string]any) {
	tok, err := p.m2m(ctx)
	if err != nil {
		telemetry.AuditPublishFailures.Inc()
		return
	}
	body, _ := json.Marshal(map[string]any{
		"action": action, "actorId": actor, "actorType": "user",
		"resourceType": resource, "resourceId": resourceID, "newState": state, "source": "delivery-service",
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
	if err != nil || resp == nil || resp.StatusCode >= 300 {
		telemetry.AuditPublishFailures.Inc()
		if resp != nil {
			_ = resp.Body.Close()
		}
		return
	}
	_ = resp.Body.Close()
}


// CreateDeliveryFollowup asks order-service to clone the original confirmed
// order into the next operating run. stopID is its idempotency key.
func (p Peers) CreateDeliveryFollowup(ctx context.Context, orderID, stopID, tripDate string, units int, resolution string) (string, string, string, error) {
    token, err := p.m2m(ctx)
    if err != nil { return "", "", "", err }
    body, err := json.Marshal(map[string]any{
        "sourceOrderId": orderID, "stopId": stopID, "tripDate": tripDate,
        "units": units, "resolution": resolution,
    })
    if err != nil { return "", "", "", err }
    target := p.OrdersURL
    if target == "" { target = "http://order-service:8080" }
    req, err := http.NewRequestWithContext(ctx, http.MethodPost, target+"/api/v1/orders/internal/delivery-followups", bytes.NewReader(body))
    if err != nil { return "", "", "", err }
    req.Header.Set("Content-Type", "application/json")
    if token != "" { req.Header.Set("Authorization", "Bearer "+token) }
    resp, err := p.http().Do(req)
    if err != nil { return "", "", "", err }
    defer resp.Body.Close()
    if resp.StatusCode >= 300 {
        message, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
        return "", "", "", fmt.Errorf("follow-up order returned HTTP %d: %s", resp.StatusCode, message)
    }
    var result struct { Order struct {
        ID string `json:"id"`
        OrderRef string `json:"orderRef"`
        RequestedDeliveryDate string `json:"requestedDeliveryDate"`
    } `json:"order"` }
    if err := json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&result); err != nil { return "", "", "", err }
    if result.Order.ID == "" { return "", "", "", fmt.Errorf("follow-up order missing ID") }
    return result.Order.ID, result.Order.OrderRef, result.Order.RequestedDeliveryDate, nil
}


func (p Peers) QueueReturnedGoodsNotice(ctx context.Context, item domain.ReturnedGoods) error {
    token, err := p.m2m(ctx)
    if err != nil { return err }
    body, err := json.Marshal(map[string]any{
        "eventKey": "returned-goods:"+item.StopID, "outletId": item.OutletID,
        "type": "DELIVERY_REJECTED", "orderRef": item.OrderRef,
        "goods": item.Goods, "units": item.Units, "reason": item.Reason,
        "resolution": item.Resolution, "followupDate": item.FollowupDate,
    })
    if err != nil { return err }
    req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.SharedURL+"/api/v1/shared/internal/notifications/enqueue", bytes.NewReader(body))
    if err != nil { return err }
    req.Header.Set("Content-Type", "application/json")
    if token != "" { req.Header.Set("Authorization", "Bearer "+token) }
    resp, err := p.http().Do(req)
    if err != nil { return err }
    defer resp.Body.Close()
    if resp.StatusCode >= 300 {
        message, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
        return fmt.Errorf("return notification returned HTTP %d: %s", resp.StatusCode, message)
    }
    return nil
}
