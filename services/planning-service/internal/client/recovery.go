package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/recovery"
	"net/http"
	"net/url"
)

func (p Peers) RecoveryIncidents(ctx context.Context) ([]recovery.Incident, error) {
	tok, err := p.m2m(ctx)
	if err != nil {
		return nil, err
	}
	var out struct {
		Items []recovery.Incident `json:"items"`
	}
	err = p.getJSON(ctx, p.FleetURL+"/api/v1/fleet/incidents?openOnly=true", tok, &out)
	return out.Items, err
}

// Unlike general planning's legacy fallback, recovery must fail closed if the
// availability service is unavailable. A missing row means the fleet default.
func (p Peers) RecoveryAvailability(ctx context.Context, date string) (map[string]string, error) {
	tok, err := p.m2m(ctx)
	if err != nil {
		return nil, err
	}
	var out struct {
		Items []struct {
			VehicleID string `json:"vehicleId"`
			Status    string `json:"status"`
		} `json:"items"`
	}
	if err = p.getJSON(ctx, p.FleetURL+"/api/v1/fleet/availability?date="+url.QueryEscape(date), tok, &out); err != nil {
		return nil, err
	}
	result := map[string]string{}
	for _, v := range out.Items {
		result[v.VehicleID] = v.Status
	}
	return result, nil
}

func (p Peers) RecoveryDelivery(ctx context.Context, orderID string) (recovery.DeliveryState, error) {
	var out struct {
		Delivery recovery.DeliveryState `json:"delivery"`
	}
	tok, err := p.m2m(ctx)
	if err != nil {
		return out.Delivery, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.DeliveryURL+"/api/v1/delivery/internal/orders/"+url.PathEscape(orderID), nil)
	if err != nil {
		return out.Delivery, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	res, err := p.http().Do(req)
	if err != nil {
		return out.Delivery, err
	}
	defer res.Body.Close()
	// 404 explicitly means delivery has not been prepared yet, not a timeout.
	if res.StatusCode == http.StatusNotFound {
		return out.Delivery, nil
	}
	if res.StatusCode != http.StatusOK {
		return out.Delivery, fmt.Errorf("delivery progress unavailable: HTTP %d", res.StatusCode)
	}
	err = json.NewDecoder(res.Body).Decode(&out)
	return out.Delivery, err
}

func (p Peers) SuspendRecovery(ctx context.Context, caseID string, moves []recovery.Move) error {
	stops := []map[string]string{}
	for _, m := range moves {
		stops = append(stops, map[string]string{"tripId": m.SourceTripID, "orderId": m.OrderID})
	}
	body, _ := json.Marshal(map[string]any{"caseId": caseID, "stops": stops})
	tok, err := p.m2m(ctx)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.DeliveryURL+"/api/v1/delivery/internal/recovery/suspend", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	res, err := p.http().Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusConflict {
		return fmt.Errorf("conflict: a delivery changed; generate new recovery options")
	}
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("recovery suspension unavailable: HTTP %d", res.StatusCode)
	}
	return nil
}
