package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/order-service/internal/domain"
)

type PlanningTrackingPeer struct {
	BaseURL string
	M2M     interface {
		Token(context.Context) (string, error)
	}
	HTTP *http.Client
}
type DeliveryTrackingPeer struct {
	BaseURL string
	M2M     interface {
		Token(context.Context) (string, error)
	}
	HTTP *http.Client
}

func httpClient(c *http.Client) *http.Client {
	if c != nil {
		return c
	}
	return http.DefaultClient
}
func get(ctx context.Context, c *http.Client, m2m interface {
	Token(context.Context) (string, error)
}, base, path string, dest any) error {
	if m2m == nil {
		return fmt.Errorf("internal token unavailable")
	}
	token, err := m2m.Token(ctx)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := httpClient(c).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("not found: %s", path)
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("internal read status %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(dest)
}
func (p PlanningTrackingPeer) ByOrder(id string) (domain.PlanningTracking, error) {
	var out struct {
		Tracking domain.PlanningTracking `json:"tracking"`
	}
	err := get(context.Background(), p.HTTP, p.M2M, p.BaseURL, "/api/v1/planning/internal/orders/"+url.PathEscape(id), &out)
	return out.Tracking, err
}
func (p DeliveryTrackingPeer) ByOrder(id string) (domain.DeliveryTracking, error) {
	var out struct {
		Delivery domain.DeliveryTracking `json:"delivery"`
	}
	err := get(context.Background(), p.HTTP, p.M2M, p.BaseURL, "/api/v1/delivery/internal/orders/"+url.PathEscape(id), &out)
	return out.Delivery, err
}
