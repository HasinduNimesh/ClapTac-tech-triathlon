package notify

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// GatewayIDPrefix marks a provider message id as one issued by our own cellular gateway, so a status update
// can tell it from a Twilio SID. The rest is the gateway's request id.
const GatewayIDPrefix = "cg:"

var (
	gatewaySelector  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)
	gatewayRequestID = regexp.MustCompile(`^[A-Za-z0-9_-]{4,72}$`)
)

// GatewayConfig is how Waypoint reaches the cellular gateway (Android phones used as SMS endpoints).
type GatewayConfig struct {
	// BaseURL is the gateway's origin, for example https://gateway.waypoint.claptac.dev. It must be HTTPS,
	// except for a gateway on the same machine during development.
	BaseURL string
	// APIKey is an API key created in the gateway dashboard (cgk_...). It acts with its creator's role.
	APIKey string
	// Group or Device pick the phone(s) to send from. Neither means any enabled device. Not both.
	Group  string
	Device string
	// Queue holds a message until a phone is online instead of failing with 409, so a text is not lost
	// while the phone is away.
	Queue bool
}

// Gateway sends SMS through the cellular gateway's REST API.
type Gateway struct {
	config   GatewayConfig
	client   *http.Client
	endpoint string
}

func isLocalHost(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "[::1]" || host == "::1"
}

func NewGateway(config GatewayConfig, client *http.Client) (*Gateway, error) {
	config.BaseURL = strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	config.APIKey = strings.TrimSpace(config.APIKey)
	config.Group = strings.TrimSpace(config.Group)
	config.Device = strings.TrimSpace(config.Device)
	if config.BaseURL == "" || config.APIKey == "" {
		return nil, fmt.Errorf("gateway URL and API key are required")
	}
	base, err := url.Parse(config.BaseURL)
	if err != nil || base.Host == "" || base.User != nil || base.RawQuery != "" || base.Fragment != "" {
		return nil, fmt.Errorf("gateway URL must be an absolute URL with no credentials")
	}
	if base.Scheme != "https" && !(base.Scheme == "http" && isLocalHost(base.Hostname())) {
		return nil, fmt.Errorf("gateway URL must use HTTPS")
	}
	if !strings.HasPrefix(config.APIKey, "cgk_") || len(config.APIKey) < 16 {
		return nil, fmt.Errorf("gateway API key must be a cgk_ key")
	}
	if config.Group != "" && config.Device != "" {
		return nil, fmt.Errorf("configure a gateway group or a device, not both")
	}
	for _, selector := range []string{config.Group, config.Device} {
		if selector != "" && !gatewaySelector.MatchString(selector) {
			return nil, fmt.Errorf("gateway group or device is invalid")
		}
	}
	if client == nil {
		client = &http.Client{Timeout: 12 * time.Second}
	}
	return &Gateway{config: config, client: client, endpoint: config.BaseURL + "/api/v1/messages"}, nil
}

// Send asks the gateway to send one SMS and returns "cg:" plus the gateway's request id, which the gateway's
// SMS_SENT and SMS_FAILED webhooks carry back. Like the Twilio provider it does not retry: the create call has no
// idempotency key, so a timeout after acceptance would send the text twice.
func (g *Gateway) Send(ctx context.Context, sms SMS) (string, error) {
	sms.To = strings.TrimSpace(sms.To)
	sms.Body = strings.TrimSpace(sms.Body)
	if !e164.MatchString(sms.To) || sms.Body == "" || len([]rune(sms.Body)) > 1600 {
		return "", fmt.Errorf("SMS recipient or body is invalid")
	}
	payload := map[string]any{"to": sms.To, "message": sms.Body, "queue": g.config.Queue}
	if g.config.Group != "" {
		payload["group"] = g.config.Group
	}
	if g.config.Device != "" {
		payload["device"] = g.config.Device
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("X-API-Key", g.config.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := g.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("SMS provider request outcome is unknown: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("SMS provider rejected request with HTTP %d", resp.StatusCode)
	}
	var result struct {
		RequestID string `json:"request_id"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result); err != nil || !gatewayRequestID.MatchString(result.RequestID) {
		return "", fmt.Errorf("SMS provider accepted request without a usable message ID; outcome requires reconciliation")
	}
	return GatewayIDPrefix + result.RequestID, nil
}

// VerifyGatewaySignature checks the X-CG-Signature header ("sha256=<hex>", the HMAC-SHA256 of the raw body with
// the webhook secret) in constant time.
func VerifyGatewaySignature(secret string, body []byte, header string) bool {
	if secret == "" {
		return false
	}
	header = strings.TrimSpace(header)
	if !strings.HasPrefix(header, "sha256=") {
		return false
	}
	got, err := hex.DecodeString(strings.TrimPrefix(header, "sha256="))
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hmac.Equal(got, mac.Sum(nil))
}

// GatewayEvent is a webhook delivery from the gateway.
type GatewayEvent struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Data struct {
		RequestID string `json:"request_id"`
		Error     string `json:"error"`
	} `json:"data"`
}

func ParseGatewayEvent(body []byte) (GatewayEvent, error) {
	var event GatewayEvent
	if err := json.Unmarshal(body, &event); err != nil {
		return GatewayEvent{}, fmt.Errorf("invalid gateway event")
	}
	return event, nil
}

// StatusUpdate turns the events that say what happened to an outbound text into a status update. Other event
// types (inbound SMS, calls, device presence) have nothing to say about a notification and give false.
func (e GatewayEvent) StatusUpdate() (StatusUpdate, bool) {
	if !gatewayRequestID.MatchString(e.Data.RequestID) {
		return StatusUpdate{}, false
	}
	switch e.Type {
	case "SMS_SENT":
		return StatusUpdate{MessageSID: GatewayIDPrefix + e.Data.RequestID, Status: "sent"}, true
	case "SMS_FAILED":
		return StatusUpdate{MessageSID: GatewayIDPrefix + e.Data.RequestID, Status: "failed", ErrorCode: "GATEWAY_SMS_FAILED"}, true
	}
	return StatusUpdate{}, false
}
