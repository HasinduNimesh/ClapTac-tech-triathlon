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
	"strings"
	"time"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/authorization"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/order-service/internal/domain"
)

type Shared struct {
	BaseURL string
	HTTP    *http.Client
	M2M     interface {
		Token(context.Context) (string, error)
	}
	Logger *slog.Logger
}

func (s Shared) http() *http.Client {
	if s.HTTP != nil {
		return s.HTTP
	}
	return http.DefaultClient
}

func (s Shared) ResolveProfile(subject, bearer string) (*authorization.Profile, error) {
	req, err := http.NewRequest(http.MethodGet, s.BaseURL+"/api/v1/shared/profiles/me", nil)
	if err != nil {
		return nil, err
	}
	if bearer != "" {
		req.Header.Set("Authorization", bearer)
	}
	resp, err := s.http().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("profile status %d", resp.StatusCode)
	}
	var out struct {
		Profile authorization.Profile `json:"profile"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if subject != "" && out.Profile.Subject != "" && out.Profile.Subject != subject {
		return nil, fmt.Errorf("profile subject mismatch")
	}
	return &out.Profile, nil
}

func (s Shared) Resolve(subject, bearer string) (string, []string, []string, error) {
	profile, err := s.ResolveProfile(subject, bearer)
	if err != nil {
		return "", nil, nil, err
	}
	return profile.UserID, profile.Roles, profile.OutletIDs, nil
}

func (s Shared) Outlet(id, bearer string) (domain.Outlet, error) {
	req, err := http.NewRequest(http.MethodGet, s.BaseURL+"/api/v1/shared/outlets/"+id, nil)
	if err != nil {
		return domain.Outlet{}, err
	}
	if bearer != "" {
		req.Header.Set("Authorization", bearer)
	}
	resp, err := s.http().Do(req)
	if err != nil {
		return domain.Outlet{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return domain.Outlet{}, fmt.Errorf("outlet status %d", resp.StatusCode)
	}
	var out struct {
		Outlet domain.Outlet `json:"outlet"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return domain.Outlet{}, err
	}
	return out.Outlet, nil
}

// Products reads catalog products by id, with the caller's own token so shared-service applies its access rules.
func (s Shared) Products(ids []string, bearer string) ([]domain.Product, error) {
	req, err := http.NewRequest(http.MethodGet, s.BaseURL+"/api/v1/shared/products?ids="+url.QueryEscape(strings.Join(ids, ",")), nil)
	if err != nil {
		return nil, err
	}
	if bearer != "" {
		req.Header.Set("Authorization", bearer)
	}
	resp, err := s.http().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("products status %d", resp.StatusCode)
	}
	var out struct {
		Items []domain.Product `json:"items"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&out); err != nil {
		return nil, err
	}
	return out.Items, nil
}

func (s Shared) CutoffLocalTime() (string, error) {
	token := ""
	if s.M2M != nil {
		t, err := s.M2M.Token(context.Background())
		if err != nil {
			return "", err
		}
		token = t
	}
	req, err := http.NewRequest(http.MethodGet, s.BaseURL+"/api/v1/shared/policies/current", nil)
	if err != nil {
		return "", err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := s.http().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("policy status %d", resp.StatusCode)
	}
	var out struct {
		Policy struct {
			CutoffLocalTime string `json:"cutoffLocalTime"`
		} `json:"policy"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return out.Policy.CutoffLocalTime, nil
}

func (s Shared) OperatingDays(from, to string) ([]domain.OperatingDay, error) {
	token := ""
	if s.M2M != nil {
		t, err := s.M2M.Token(context.Background())
		if err != nil {
			return nil, err
		}
		token = t
	}
	req, err := http.NewRequest(http.MethodGet, s.BaseURL+"/api/v1/shared/calendar?from="+from+"&to="+to, nil)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := s.http().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("calendar status %d", resp.StatusCode)
	}
	var out struct {
		Items []domain.OperatingDay `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out.Items, nil
}

func (s Shared) PublishCreated(correlationID, actorID, orderRef string, state map[string]any) error {
	token := ""
	if s.M2M != nil {
		t, err := s.M2M.Token(context.Background())
		if err != nil {
			s.log("audit_token_failed", err, orderRef)
			go s.retry(correlationID, actorID, orderRef, state)
			return err
		}
		token = "Bearer " + t
	}
	if err := s.postAudit(token, correlationID, actorID, orderRef, state); err != nil {
		s.log("audit_publish_failed", err, orderRef)
		go s.retry(correlationID, actorID, orderRef, state)
		return err
	}
	return nil
}

func (s Shared) PublishReceipt(action, actorID, orderRef string, state map[string]any) error {
	token := ""
	if s.M2M != nil {
		t, err := s.M2M.Token(context.Background())
		if err != nil {
			return err
		}
		token = "Bearer " + t
	}
	body, _ := json.Marshal(map[string]any{"action": action, "actorId": actorID, "actorType": "user", "resourceType": "ORDER", "resourceId": orderRef, "newState": state, "source": "ORDER_SERVICE"})
	req, err := http.NewRequest(http.MethodPost, s.BaseURL+"/api/v1/shared/audit-events", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", token)
	}
	resp, err := s.http().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("audit status %d", resp.StatusCode)
	}
	return nil
}

func (s Shared) retry(correlationID, actorID, orderRef string, state map[string]any) {
	for i := 0; i < 3; i++ {
		time.Sleep(time.Duration(i+1) * 200 * time.Millisecond)
		token := ""
		if s.M2M != nil {
			t, err := s.M2M.Token(context.Background())
			if err != nil {
				continue
			}
			token = "Bearer " + t
		}
		if err := s.postAudit(token, correlationID, actorID, orderRef, state); err == nil {
			return
		}
	}
	s.log("audit_retry_exhausted", fmt.Errorf("giving up"), orderRef)
}

func (s Shared) postAudit(token, correlationID, actorID, orderRef string, state map[string]any) error {
	body, _ := json.Marshal(map[string]any{
		"action":        "ORDER_CREATED",
		"actorId":       actorID,
		"actorType":     "user",
		"resourceType":  "ORDER",
		"resourceId":    orderRef,
		"correlationId": correlationID,
		"newState":      state,
		"source":        "WEB",
	})
	req, err := http.NewRequest(http.MethodPost, s.BaseURL+"/api/v1/shared/audit-events", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", token)
	}
	resp, err := s.http().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("audit status %d", resp.StatusCode)
	}
	return nil
}

func (s Shared) log(msg string, err error, orderRef string) {
	if s.Logger == nil {
		return
	}
	s.Logger.Error(msg, "order_id", orderRef, "error", err)
}
