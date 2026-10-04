package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/auth"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/authorization"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/telemetry"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/loading-service/internal/domain"
)

type TokenSource interface {
	Token(ctx context.Context) (string, error)
}

type Peers struct {
	PlanningURL string
	OrdersURL   string
	SharedURL   string
	M2M         TokenSource
	HTTP        *http.Client
	Logger      *slog.Logger
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

func (p Peers) ConfirmedTrips(ctx context.Context, date, depot string) ([]domain.PlanningTrip, error) {
	tok, err := p.m2m(ctx)
	if err != nil {
		return nil, err
	}
	q := "?date=" + date
	if depot != "" {
		q += "&depot=" + depot
	}
	var out struct {
		Items []domain.PlanningTrip `json:"items"`
	}
	if err := p.getJSON(ctx, p.PlanningURL+"/api/v1/planning/internal/trips"+q, tok, &out); err != nil {
		return nil, err
	}
	if out.Items == nil {
		out.Items = []domain.PlanningTrip{}
	}
	return out.Items, nil
}

func (p Peers) ConfirmedTrip(ctx context.Context, tripID string) (domain.PlanningTrip, error) {
	tok, err := p.m2m(ctx)
	if err != nil {
		return domain.PlanningTrip{}, err
	}
	var out struct {
		Trip domain.PlanningTrip `json:"trip"`
	}
	if err := p.getJSON(ctx, p.PlanningURL+"/api/v1/planning/internal/trips/"+tripID, tok, &out); err != nil {
		return domain.PlanningTrip{}, err
	}
	return out.Trip, nil
}

func (p Peers) Order(ctx context.Context, id string) (domain.OrderDetail, error) {
	tok, err := p.m2m(ctx)
	if err != nil {
		return domain.OrderDetail{}, err
	}
	var out struct {
		Order domain.OrderDetail `json:"order"`
	}
	if err := p.getJSON(ctx, p.OrdersURL+"/api/v1/orders/"+id, tok, &out); err != nil {
		return domain.OrderDetail{}, err
	}
	return out.Order, nil
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
		"resourceType": resource, "resourceId": resourceID, "newState": state, "source": "loading-service",
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
		"resourceType": resource, "resourceId": resourceID, "newState": state, "source": "loading-service",
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

// QueueShortfallNotice asks shared-service to enqueue the store notice for a
// dispatcher shortfall decision. eventKey makes it idempotent per issue+decision.
func (p Peers) QueueShortfallNotice(ctx context.Context, eventKey, outletID, orderRef, decision string, unitsShort int) error {
	tok, err := p.m2m(ctx)
	if err != nil {
		return err
	}
	body, _ := json.Marshal(map[string]any{"eventKey": eventKey, "outletId": outletID, "type": "LOAD_SHORTFALL", "orderRef": orderRef, "reason": decision, "units": unitsShort})
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
