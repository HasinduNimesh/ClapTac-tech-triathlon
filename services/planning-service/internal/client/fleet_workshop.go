package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// SetVehicleWorkshop marks a vehicle in_workshop for the date through
// fleet-service so the planner no longer treats it as available.
func (p Peers) SetVehicleWorkshop(ctx context.Context, vehicleID, date, reason string) error {
	tok, err := p.m2m(ctx)
	if err != nil {
		return err
	}
	body, _ := json.Marshal(map[string]string{"date": date, "status": "in_workshop", "reason": reason})
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, p.FleetURL+"/api/v1/fleet/vehicles/"+url.PathEscape(vehicleID)+"/availability", bytes.NewReader(body))
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
		return fmt.Errorf("fleet availability update returned HTTP %d: %s", resp.StatusCode, string(b))
	}
	return nil
}
