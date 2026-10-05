package notify

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func gatewayConfig() GatewayConfig {
	return GatewayConfig{BaseURL: "https://gateway.example.test", APIKey: "cgk_0123456789abcdef", Queue: true}
}

func TestNewGatewayValidatesItsSettings(t *testing.T) {
	cases := map[string]GatewayConfig{
		"no url":                   {APIKey: "cgk_0123456789abcdef"},
		"no key":                   {BaseURL: "https://gateway.example.test"},
		"plain http to the world":  {BaseURL: "http://gateway.example.test", APIKey: "cgk_0123456789abcdef"},
		"credentials in the url":   {BaseURL: "https://user:pass@gateway.example.test", APIKey: "cgk_0123456789abcdef"},
		"a query in the url":       {BaseURL: "https://gateway.example.test?x=1", APIKey: "cgk_0123456789abcdef"},
		"not a gateway key":        {BaseURL: "https://gateway.example.test", APIKey: "whsec_0123456789abcdef"},
		"key too short":            {BaseURL: "https://gateway.example.test", APIKey: "cgk_1"},
		"group and device":         {BaseURL: "https://gateway.example.test", APIKey: "cgk_0123456789abcdef", Group: "primary", Device: "redmi"},
		"a group with a bad shape": {BaseURL: "https://gateway.example.test", APIKey: "cgk_0123456789abcdef", Group: "../../x"},
	}
	for name, cfg := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := NewGateway(cfg, nil); err == nil {
				t.Fatal("expected an invalid configuration")
			}
		})
	}
	for _, ok := range []GatewayConfig{
		gatewayConfig(),
		{BaseURL: "http://localhost:18000/", APIKey: "cgk_0123456789abcdef"},
		{BaseURL: "http://127.0.0.1:18000", APIKey: "cgk_0123456789abcdef", Group: "primary"},
		{BaseURL: "https://gateway.example.test/", APIKey: "cgk_0123456789abcdef", Device: "redmi"},
	} {
		if _, err := NewGateway(ok, nil); err != nil {
			t.Errorf("%+v should be valid: %v", ok, err)
		}
	}
}

func TestGatewaySendsOneJSONMessageAndReturnsAPrefixedRequestID(t *testing.T) {
	cfg := gatewayConfig()
	cfg.Group = "primary"
	client := fakeClient(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost || r.URL.String() != "https://gateway.example.test/api/v1/messages" {
			return nil, fmt.Errorf("unexpected request %s %s", r.Method, r.URL)
		}
		if r.Header.Get("X-API-Key") != cfg.APIKey || r.Header.Get("Content-Type") != "application/json" {
			return nil, fmt.Errorf("headers: %v", r.Header)
		}
		raw, _ := io.ReadAll(r.Body)
		var got map[string]any
		if err := json.Unmarshal(raw, &got); err != nil {
			return nil, err
		}
		if got["to"] != "+94771112222" || got["message"] != "Waypoint delivery update" || got["queue"] != true || got["group"] != "primary" {
			return nil, fmt.Errorf("bad body: %s", raw)
		}
		if _, hasDevice := got["device"]; hasDevice {
			return nil, fmt.Errorf("a group send must not name a device: %s", raw)
		}
		return response(r, http.StatusCreated, `{"id":"9705aed0-2075-49c0-9826-1b10b6eaf8f6","request_id":"req_Hr93eRmiqYu0Ed2nWsSdaQ","status":"QUEUED"}`), nil
	})
	g, err := NewGateway(cfg, client)
	if err != nil {
		t.Fatal(err)
	}
	id, err := g.Send(context.Background(), SMS{To: " +94771112222 ", Body: " Waypoint delivery update "})
	if err != nil {
		t.Fatal(err)
	}
	if id != "cg:req_Hr93eRmiqYu0Ed2nWsSdaQ" {
		t.Fatalf("id = %q", id)
	}
}

func TestGatewayCanTargetOneDeviceAndOptOutOfQueueing(t *testing.T) {
	cfg := gatewayConfig()
	cfg.Device = "redmi"
	cfg.Queue = false
	client := fakeClient(func(r *http.Request) (*http.Response, error) {
		raw, _ := io.ReadAll(r.Body)
		var got map[string]any
		_ = json.Unmarshal(raw, &got)
		if got["device"] != "redmi" || got["queue"] != false {
			return nil, fmt.Errorf("bad body: %s", raw)
		}
		return response(r, http.StatusCreated, `{"request_id":"req_abcd1234"}`), nil
	})
	g, _ := NewGateway(cfg, client)
	if _, err := g.Send(context.Background(), SMS{To: "+94771112222", Body: "hello"}); err != nil {
		t.Fatal(err)
	}
}

func TestGatewayRefusesBadInputBeforeAnyRequest(t *testing.T) {
	calls := 0
	g, _ := NewGateway(gatewayConfig(), fakeClient(func(r *http.Request) (*http.Response, error) {
		calls++
		return response(r, http.StatusCreated, `{"request_id":"req_abcd1234"}`), nil
	}))
	for name, sms := range map[string]SMS{
		"not E.164":  {To: "0771112222", Body: "hello"},
		"empty body": {To: "+94771112222", Body: "  "},
		"too long":   {To: "+94771112222", Body: strings.Repeat("ක", 1601)},
	} {
		if _, err := g.Send(context.Background(), sms); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	if calls != 0 {
		t.Fatalf("%d request(s) were made for invalid input", calls)
	}
}

func TestGatewayReportsRefusalsAndAmbiguousOutcomesDifferently(t *testing.T) {
	send := func(status int, body string) error {
		g, _ := NewGateway(gatewayConfig(), fakeClient(func(r *http.Request) (*http.Response, error) { return response(r, status, body), nil }))
		_, err := g.Send(context.Background(), SMS{To: "+94771112222", Body: "hello"})
		return err
	}
	for _, status := range []int{400, 401, 403, 409, 422, 429, 500} {
		err := send(status, `{"detail":"secret detail must not leak"}`)
		if err == nil || strings.Contains(err.Error(), "outcome is unknown") || strings.Contains(err.Error(), "secret detail") {
			t.Errorf("HTTP %d: want a plain refusal, got %v", status, err)
		}
	}
	if err := send(201, `{"status":"QUEUED"}`); err == nil || !strings.Contains(err.Error(), "reconciliation") {
		t.Errorf("accepted without an id should need reconciliation, got %v", err)
	}
	if err := send(201, `{"request_id":"has spaces and ?"}`); err == nil {
		t.Error("an unusable request id must be refused")
	}
	down := fakeClient(func(r *http.Request) (*http.Response, error) { return nil, fmt.Errorf("connection reset") })
	g, _ := NewGateway(gatewayConfig(), down)
	if _, err := g.Send(context.Background(), SMS{To: "+94771112222", Body: "hello"}); err == nil || !strings.Contains(err.Error(), "outcome is unknown") {
		t.Errorf("a transport failure leaves the outcome unknown, got %v", err)
	}
}

func sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestVerifyGatewaySignature(t *testing.T) {
	body := []byte(`{"id":"d1","type":"SMS_SENT","data":{"request_id":"req_abcd1234"}}`)
	good := sign("whsec_test", body)
	if !VerifyGatewaySignature("whsec_test", body, good) {
		t.Fatal("a correct signature must verify")
	}
	for name, c := range map[string]struct {
		secret, header string
		body           []byte
	}{
		"wrong secret":    {"whsec_other", good, body},
		"tampered body":   {"whsec_test", good, append(append([]byte{}, body...), ' ')},
		"no prefix":       {"whsec_test", strings.TrimPrefix(good, "sha256="), body},
		"not hex":         {"whsec_test", "sha256=zz", body},
		"empty header":    {"whsec_test", "", body},
		"no secret set":   {"", good, body},
		"other algorithm": {"whsec_test", "sha1=" + strings.TrimPrefix(good, "sha256="), body},
	} {
		if VerifyGatewaySignature(c.secret, c.body, c.header) {
			t.Errorf("%s must not verify", name)
		}
	}
}

func TestGatewayEventsMapToStatusUpdates(t *testing.T) {
	parse := func(raw string) GatewayEvent {
		e, err := ParseGatewayEvent([]byte(raw))
		if err != nil {
			t.Fatal(err)
		}
		return e
	}
	sent, ok := parse(`{"id":"d1","type":"SMS_SENT","data":{"request_id":"req_abcd1234","result":"ok"}}`).StatusUpdate()
	if !ok || sent.MessageSID != "cg:req_abcd1234" || sent.Status != "sent" || sent.ErrorCode != "" {
		t.Errorf("SMS_SENT -> %+v %v", sent, ok)
	}
	failed, ok := parse(`{"id":"d2","type":"SMS_FAILED","data":{"request_id":"req_abcd1234","error":"No SIM"}}`).StatusUpdate()
	if !ok || failed.Status != "failed" || failed.ErrorCode != "GATEWAY_SMS_FAILED" || strings.Contains(failed.ErrorCode, "SIM") {
		t.Errorf("SMS_FAILED -> %+v %v", failed, ok)
	}
	for _, raw := range []string{
		`{"type":"SMS_RECEIVED","data":{"request_id":"req_abcd1234"}}`,
		`{"type":"DEVICE_ONLINE","data":{}}`,
		`{"type":"SMS_SENT","data":{}}`,
		`{"type":"SMS_SENT","data":{"request_id":"bad id!"}}`,
	} {
		if _, ok := parse(raw).StatusUpdate(); ok {
			t.Errorf("%s says nothing about a notification", raw)
		}
	}
	if _, err := ParseGatewayEvent([]byte("not json")); err == nil {
		t.Error("garbage must be refused")
	}
}
