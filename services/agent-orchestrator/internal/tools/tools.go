package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/telemetry"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/agent-orchestrator/internal/credentials"
)

const MaxResultBytes = 64 << 10

var safeID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,80}$`)

type HTTPError struct{ Code int }

func (e HTTPError) Error() string   { return fmt.Sprintf("business service returned HTTP %d", e.Code) }
func (e HTTPError) StatusCode() int { return e.Code }

type Property struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}
type Definition struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  struct {
		Type                 string              `json:"type"`
		Properties           map[string]Property `json:"properties"`
		Required             []string            `json:"required,omitempty"`
		AdditionalProperties bool                `json:"additionalProperties"`
	} `json:"parameters"`
}

type Tool interface {
	Name() string
	Description() string
	Sensitive() bool
	Definition() Definition
	Validate(map[string]any) error
	Call(context.Context, map[string]any) (map[string]any, error)
}

type HTTPTool struct {
	name, description, baseURL, method string
	sensitive                          bool
	properties                         map[string]Property
	required                           []string
	build                              func(map[string]any) (string, string, map[string]any, error)
	creds                              credentials.Provider
	client                             *http.Client
}

func (t *HTTPTool) Name() string        { return t.name }
func (t *HTTPTool) Description() string { return t.description }
func (t *HTTPTool) Sensitive() bool     { return t.sensitive }
func (t *HTTPTool) Definition() Definition {
	d := Definition{Name: t.name, Description: t.description}
	d.Parameters.Type = "object"
	d.Parameters.Properties = t.properties
	d.Parameters.Required = append([]string(nil), t.required...)
	d.Parameters.AdditionalProperties = false
	return d
}
func (t *HTTPTool) Validate(args map[string]any) error {
	if args == nil {
		args = map[string]any{}
	}
	for key := range args {
		if _, ok := t.properties[key]; !ok {
			return fmt.Errorf("unexpected argument %q", key)
		}
	}
	for _, key := range t.required {
		if v, ok := args[key]; !ok || v == nil || v == "" {
			return fmt.Errorf("argument %q is required", key)
		}
	}
	for key, value := range args {
		if value == nil {
			return fmt.Errorf("argument %q is invalid", key)
		}
		switch t.properties[key].Type {
		case "string":
			if _, ok := value.(string); !ok {
				return fmt.Errorf("argument %q must be a string", key)
			}
		case "integer":
			if _, ok := numberInt(value); !ok {
				return fmt.Errorf("argument %q must be an integer", key)
			}
		case "number":
			if _, ok := value.(float64); !ok {
				if _, ok := value.(int); !ok {
					return fmt.Errorf("argument %q must be a number", key)
				}
			}
		}
		if strings.HasSuffix(key, "Id") {
			s, _ := value.(string)
			if !safeID.MatchString(s) {
				return fmt.Errorf("argument %q is invalid", key)
			}
		}
		if key == "date" || key == "requestedDeliveryDate" {
			s, _ := value.(string)
			if _, err := time.Parse("2006-01-02", s); err != nil {
				return fmt.Errorf("argument %q must be YYYY-MM-DD", key)
			}
		}
	}
	if t.name == "CreateOrder" {
		units, ok := numberInt(args["orderUnits"])
		if !ok || units <= 0 {
			return errors.New("orderUnits must be positive")
		}
		weight := numeric(args["orderWeightKg"])
		volume := numeric(args["orderVolumeM3"])
		temp, _ := args["temperatureRequirement"].(string)
		if weight <= 0 || volume <= 0 || (strings.ToLower(temp) != "ambient" && strings.ToLower(temp) != "chilled") {
			return errors.New("order quantities or temperature are invalid")
		}
	}
	return nil
}
func (t *HTTPTool) Call(ctx context.Context, args map[string]any) (map[string]any, error) {
	started := time.Now()
	telemetry.AgentToolCalls.Inc()
	telemetry.AgentToolCallsByName.WithLabelValues(t.name).Inc()
	fail := func(err error) (map[string]any, error) {
		telemetry.AgentToolFailures.Inc()
		telemetry.AgentFailures.WithLabelValues("tool").Inc()
		return nil, err
	}
	if err := t.Validate(args); err != nil {
		return fail(err)
	}
	if t.build == nil {
		return fail(errors.New("tool is not configured"))
	}
	path, method, body, err := t.build(args)
	if err != nil {
		return fail(err)
	}
	token, err := t.creds.CredentialForService(ctx, t.name)
	if err != nil {
		return fail(err)
	}
	var reader io.Reader
	if body != nil {
		raw, e := json.Marshal(body)
		if e != nil {
			return fail(e)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(t.baseURL, "/")+path, reader)
	if err != nil {
		return fail(err)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", token)
	}
	resp, err := t.client.Do(req)
	if err != nil {
		return fail(err)
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(resp.Body, MaxResultBytes+1))
	if err != nil {
		return fail(err)
	}
	if len(payload) > MaxResultBytes {
		return fail(errors.New("tool response exceeds size limit"))
	}
	if resp.StatusCode >= 400 {
		return fail(HTTPError{Code: resp.StatusCode})
	}
	out := map[string]any{}
	if err := json.Unmarshal(payload, &out); err != nil {
		return fail(errors.New("tool returned invalid JSON"))
	}
	telemetry.AgentLatency.Observe(time.Since(started).Seconds())
	return out, nil
}

type spec struct {
	name, description, service, method string
	sensitive                          bool
	properties                         map[string]Property
	required                           []string
	build                              func(map[string]any) (string, string, map[string]any, error)
}

func str(a map[string]any, k string) string    { v, _ := a[k].(string); return v }
func pathID(a map[string]any, k string) string { return url.PathEscape(str(a, k)) }
func intArg(a map[string]any, k string) (int, error) {
	n, ok := numberInt(a[k])
	if !ok || n < 1 || n > 2 {
		return 0, fmt.Errorf("%s must be 1 or 2", k)
	}
	return n, nil
}
func numeric(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	}
	return 0
}

func numberInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case float64:
		i := int(n)
		return i, float64(i) == n
	}
	return 0, false
}
func query(servicePath, key string, args map[string]any) (string, string, map[string]any, error) {
	v := str(args, key)
	if v == "" {
		return "", "", nil, fmt.Errorf("%s required", key)
	}
	return servicePath + "?" + key + "=" + url.QueryEscape(v), http.MethodGet, nil, nil
}

func Catalog(creds credentials.Provider) []Tool {
	order := envURL("ORDER_SERVICE_URL", "http://order-service:8080")
	planning := envURL("PLANNING_SERVICE_URL", "http://planning-service:8080")
	fleet := envURL("FLEET_SERVICE_URL", "http://fleet-service:8080")
	loading := envURL("LOADING_SERVICE_URL", "http://loading-service:8080")
	delivery := envURL("DELIVERY_SERVICE_URL", "http://delivery-service:8080")
	shared := envURL("SHARED_SERVICE_URL", "http://shared-service:8080")
	date := map[string]Property{"date": {Type: "string", Description: "Delivery date in YYYY-MM-DD format"}}
	orderID := map[string]Property{"orderId": {Type: "string", Description: "Order identifier"}}
	planID := map[string]Property{"planId": {Type: "string", Description: "Planning identifier"}}
	toolspecs := []spec{
		{name: "GetMyProfile", description: "Get the signed-in application profile and role", service: shared, method: http.MethodGet, build: func(map[string]any) (string, string, map[string]any, error) {
			return "/api/v1/shared/profiles/me", http.MethodGet, nil, nil
		}},
		{name: "GetOrders", description: "List orders visible to the signed-in user", service: order, method: http.MethodGet, properties: map[string]Property{"status": {Type: "string", Description: "Optional order status"}, "brand": {Type: "string", Description: "Optional brand"}, "requested_delivery_date": date["date"]}, build: func(a map[string]any) (string, string, map[string]any, error) {
			q := url.Values{}
			for _, k := range []string{"status", "brand", "requested_delivery_date"} {
				if v := str(a, k); v != "" {
					q.Set(k, v)
				}
			}
			p := "/api/v1/orders"
			if len(q) > 0 {
				p += "?" + q.Encode()
			}
			return p, http.MethodGet, nil, nil
		}},
		{name: "GetOrderTracking", description: "Get status, planned ETA, delivery outcome and receipt for an order the user can access", service: order, method: http.MethodGet, properties: orderID, required: []string{"orderId"}, build: func(a map[string]any) (string, string, map[string]any, error) {
			return "/api/v1/orders/" + pathID(a, "orderId") + "/tracking", http.MethodGet, nil, nil
		}},
		{name: "GetPlan", description: "Get a plan by delivery date", service: planning, method: http.MethodGet, properties: date, required: []string{"date"}, build: func(a map[string]any) (string, string, map[string]any, error) {
			return query("/api/v1/planning/plans", "date", a)
		}},
		{name: "GetConstraintFailures", description: "Get deterministic constraint outcomes recorded for a plan date", service: planning, method: http.MethodGet, properties: date, required: []string{"date"}, build: func(a map[string]any) (string, string, map[string]any, error) {
			return query("/api/v1/planning/plans", "date", a)
		}},
		{name: "SimulatePlan", description: "Run the deterministic planning engine without persisting assignments", service: planning, method: http.MethodPost, properties: planID, required: []string{"planId"}, build: func(a map[string]any) (string, string, map[string]any, error) {
			return "/api/v1/planning/plans/" + pathID(a, "planId") + "/simulate", http.MethodPost, map[string]any{}, nil
		}},
		{name: "GetFleetSummary", description: "List fleet vehicles visible to the signed-in user", service: fleet, method: http.MethodGet, build: func(map[string]any) (string, string, map[string]any, error) {
			return "/api/v1/fleet/vehicles", http.MethodGet, nil, nil
		}},
		{name: "GetLoadingStatus", description: "Get loading status for a delivery date", service: loading, method: http.MethodGet, properties: date, required: []string{"date"}, build: func(a map[string]any) (string, string, map[string]any, error) {
			return query("/api/v1/loading/trips", "date", a)
		}},
		{name: "GetDeliveryStatus", description: "Get delivery status for a delivery date", service: delivery, method: http.MethodGet, properties: date, required: []string{"date"}, build: func(a map[string]any) (string, string, map[string]any, error) {
			return query("/api/v1/delivery/trips", "date", a)
		}},
		{name: "GetReceiptStatus", description: "Get receipts awaiting confirmation for the signed-in Store Manager", service: order, method: http.MethodGet, build: func(map[string]any) (string, string, map[string]any, error) {
			return "/api/v1/orders/receipts/pending", http.MethodGet, nil, nil
		}},
		{name: "CreateOrder", description: "Propose a new store order; requires human approval before submission", service: order, method: http.MethodPost, sensitive: true, properties: map[string]Property{"requestedDeliveryDate": {Type: "string", Description: "Needed delivery date YYYY-MM-DD"}, "orderUnits": {Type: "integer", Description: "Requested units"}, "orderWeightKg": {Type: "number", Description: "Total order weight kg"}, "orderVolumeM3": {Type: "number", Description: "Total order volume m3"}, "temperatureRequirement": {Type: "string", Description: "AMBIENT or CHILLED"}}, required: []string{"requestedDeliveryDate", "orderUnits", "orderWeightKg", "orderVolumeM3", "temperatureRequirement"}, build: func(a map[string]any) (string, string, map[string]any, error) {
			return "/api/v1/orders", http.MethodPost, a, nil
		}},
		{name: "DeferOrder", description: "Propose deferring an order; requires human approval", service: planning, method: http.MethodPost, sensitive: true, properties: map[string]Property{"planId": planID["planId"], "orderId": orderID["orderId"], "reasonCode": {Type: "string", Description: "Structured planning deferral reason"}, "comment": {Type: "string", Description: "Deferral explanation"}}, required: []string{"planId", "orderId", "reasonCode"}, build: func(a map[string]any) (string, string, map[string]any, error) {
			return "/api/v1/planning/plans/" + pathID(a, "planId") + "/deferrals", http.MethodPost, map[string]any{"orderId": a["orderId"], "reasonCode": a["reasonCode"], "comment": a["comment"]}, nil
		}},
		{name: "ReassignAllocation", description: "Propose moving an order allocation to a different vehicle and trip; requires human approval", service: planning, method: http.MethodPut, sensitive: true, properties: map[string]Property{"planId": planID["planId"], "allocationId": {Type: "string", Description: "Allocation identifier"}, "vehicleId": {Type: "string", Description: "Replacement vehicle identifier"}, "tripNumber": {Type: "integer", Description: "Trip number 1 or 2"}, "reason": {Type: "string", Description: "Why this manual reassignment is being made"}}, required: []string{"planId", "allocationId", "vehicleId", "tripNumber", "reason"}, build: func(a map[string]any) (string, string, map[string]any, error) {
			n, err := intArg(a, "tripNumber")
			if err != nil {
				return "", "", nil, err
			}
			return "/api/v1/planning/plans/" + pathID(a, "planId") + "/allocations/" + pathID(a, "allocationId"), http.MethodPut, map[string]any{"vehicleId": a["vehicleId"], "tripNumber": n, "reason": a["reason"]}, nil
		}},
		{name: "ConfirmPlan", description: "Propose confirming the current plan; requires human approval", service: planning, method: http.MethodPost, sensitive: true, properties: planID, required: []string{"planId"}, build: func(a map[string]any) (string, string, map[string]any, error) {
			return "/api/v1/planning/plans/" + pathID(a, "planId") + "/confirm", http.MethodPost, nil, nil
		}},
	}
	out := make([]Tool, 0, len(toolspecs))
	for _, s := range toolspecs {
		out = append(out, &HTTPTool{name: s.name, description: s.description, baseURL: s.service, method: s.method, sensitive: s.sensitive, properties: s.properties, required: s.required, build: s.build, creds: creds, client: &http.Client{Timeout: 8 * time.Second}})
	}
	return out
}

func envURL(key, fallback string) string {
	if v := lookup(key, fallback); v != "" {
		return strings.TrimRight(v, "/")
	}
	return fallback
}
