package notify

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func fakeClient(f roundTripFunc) *http.Client                             { return &http.Client{Transport: f} }
func response(r *http.Request, status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}
}
func config() TwilioConfig {
	return TwilioConfig{AccountSID: "AC0123456789abcdef0123456789abcdef", AuthToken: "test-secret", From: "+94770000000"}
}

func TestNewTwilioRequiresSingleValidSender(t *testing.T) {
	for name, cfg := range map[string]TwilioConfig{"missing credentials": {}, "no sender": {AccountSID: config().AccountSID, AuthToken: "token"}, "two senders": {AccountSID: config().AccountSID, AuthToken: "token", From: "+94770000000", MessagingServiceSID: "MG0123456789abcdef0123456789abcdef"}, "bad source number": {AccountSID: config().AccountSID, AuthToken: "token", From: "0770000000"}} {
		t.Run(name, func(t *testing.T) {
			if _, err := NewTwilio(cfg, nil); err == nil {
				t.Fatal("expected invalid configuration")
			}
		})
	}
}

func TestTwilioSendsFormEncodedSMSAndReturnsProviderSID(t *testing.T) {
	client := fakeClient(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost || r.URL.Path != "/2010-04-01/Accounts/AC0123456789abcdef0123456789abcdef/Messages.json" {
			return nil, fmt.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		user, pass, ok := r.BasicAuth()
		if !ok || user != config().AccountSID || pass != config().AuthToken {
			return nil, fmt.Errorf("basic auth missing")
		}
		b, _ := io.ReadAll(r.Body)
		form, _ := url.ParseQuery(string(b))
		if form.Get("To") != "+94771112222" || form.Get("From") != "+94770000000" || form.Get("Body") != "Waypoint delivery update" || form.Get("StatusCallback") != "https://notify.example.test/api/v1/integrations/notifications/status" {
			return nil, fmt.Errorf("bad form: %v", form)
		}
		return response(r, http.StatusCreated, `{"sid":"SM0123456789abcdef0123456789abcdef","status":"queued"}`), nil
	})
	cfg := config()
	cfg.StatusCallbackURL = "https://notify.example.test/api/v1/integrations/notifications/status"
	adapter, err := NewTwilio(cfg, client)
	if err != nil {
		t.Fatal(err)
	}
	sid, err := adapter.Send(context.Background(), SMS{To: "+94771112222", Body: "Waypoint delivery update"})
	if err != nil || sid != "SM0123456789abcdef0123456789abcdef" {
		t.Fatalf("Send()=%q,%v", sid, err)
	}
}

func TestTwilioUsesMessagingServiceSender(t *testing.T) {
	client := fakeClient(func(r *http.Request) (*http.Response, error) {
		b, _ := io.ReadAll(r.Body)
		form, _ := url.ParseQuery(string(b))
		if form.Get("MessagingServiceSid") != "MG0123456789abcdef0123456789abcdef" || form.Has("From") {
			return nil, fmt.Errorf("unexpected sender form: %v", form)
		}
		return response(r, http.StatusCreated, `{"sid":"SM0123456789abcdef0123456789abcdef"}`), nil
	})
	cfg := TwilioConfig{AccountSID: config().AccountSID, AuthToken: "token", MessagingServiceSID: "MG0123456789abcdef0123456789abcdef"}
	adapter, err := NewTwilio(cfg, client)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Send(context.Background(), SMS{To: "+94771112222", Body: "Update"}); err != nil {
		t.Fatal(err)
	}
}

func TestTwilioRejectsInvalidSMSAndProviderRejection(t *testing.T) {
	client := fakeClient(func(r *http.Request) (*http.Response, error) { return response(r, http.StatusGatewayTimeout, ""), nil })
	adapter, err := NewTwilio(config(), client)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = adapter.Send(context.Background(), SMS{To: "0770000000", Body: "bad"}); err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("invalid number: %v", err)
	}
	if _, err = adapter.Send(context.Background(), SMS{To: "+94771112222", Body: "test"}); err == nil || !strings.Contains(err.Error(), "HTTP 504") {
		t.Fatalf("provider failure: %v", err)
	}
}
