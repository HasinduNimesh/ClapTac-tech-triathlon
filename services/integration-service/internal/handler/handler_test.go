package handler

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"testing"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/auth"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/integration-service/internal/notify"
	"github.com/go-chi/chi/v5"
)

type acceptedAuth struct{}

func (acceptedAuth) Authenticate(*http.Request) (*auth.Principal, error) {
	return &auth.Principal{Subject: "dispatcher", Scopes: []string{"notifications:send"}}, nil
}

type fixedSMS struct {
	id       string
	err      error
	to, body string
}

type fixedStatusSink struct {
	got notify.StatusUpdate
	err error
}

func (s *fixedStatusSink) UpdateStatus(_ context.Context, u notify.StatusUpdate) error {
	s.got = u
	return s.err
}

func testTwilioSignature(token, callback string, values url.Values) string {
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	data := callback
	for _, k := range keys {
		data += k + values.Get(k)
	}
	mac := hmac.New(sha1.New, []byte(token))
	_, _ = io.WriteString(mac, data)
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func (s *fixedSMS) Send(_ context.Context, m notify.SMS) (string, error) {
	s.to = m.To
	s.body = m.Body
	return s.id, s.err
}

func TestUnconfiguredAdaptersReturnProblemNotFakeSuccess(t *testing.T) {
	router := chi.NewRouter()
	(Handler{Authn: acceptedAuth{}}).Routes(router)
	for _, path := range []string{"/api/v1/integrations/routing/directions", "/api/v1/integrations/storage/presign", "/api/v1/integrations/notifications/send"} {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if res.Code != http.StatusNotImplemented {
			t.Errorf("%s: expected 501, got %d", path, res.Code)
		}
		if got := res.Header().Get("Content-Type"); got != "application/problem+json" {
			t.Errorf("%s: expected problem+json, got %q", path, got)
		}
	}
}

func TestNotificationSendRequiresScopeAndReturnsProviderStatus(t *testing.T) {
	provider := &fixedSMS{id: "SM0123456789abcdef0123456789abcdef"}
	router := chi.NewRouter()
	(Handler{Authn: acceptedAuth{}, SMS: provider}).Routes(router)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/integrations/notifications/send", strings.NewReader(`{"to":"+94771112222","body":"Delay update"}`))
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusAccepted || !strings.Contains(res.Body.String(), provider.id) || provider.to != "+94771112222" {
		t.Fatalf("send response=%d %s provider=%+v", res.Code, res.Body.String(), provider)
	}
}

type noScopeAuth struct{}

func (noScopeAuth) Authenticate(*http.Request) (*auth.Principal, error) {
	return &auth.Principal{Subject: "user"}, nil
}

func TestNotificationSendRejectsUserTokenWithoutServiceScope(t *testing.T) {
	router := chi.NewRouter()
	(Handler{Authn: noScopeAuth{}, SMS: &fixedSMS{id: "unused"}}).Routes(router)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/integrations/notifications/send", strings.NewReader(`{"to":"+94771112222","body":"message"}`))
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", res.Code, res.Body.String())
	}
}

func TestTwilioStatusCallbackRequiresValidSignatureAndUpdatesSharedStatus(t *testing.T) {
	const callback = "https://notify.example.test/api/v1/integrations/notifications/status"
	form := url.Values{"MessageSid": {"SM0123456789abcdef0123456789abcdef"}, "MessageStatus": {"delivered"}, "ErrorCode": {""}}
	sink := &fixedStatusSink{}
	router := chi.NewRouter()
	(Handler{CallbackURL: callback, CallbackAuthToken: "secret", StatusSink: sink}).Routes(router)
	makeReq := func(signature string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/integrations/notifications/status", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("X-Twilio-Signature", signature)
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		return res
	}
	if res := makeReq("invalid"); res.Code != http.StatusForbidden {
		t.Fatalf("invalid signature accepted: %d", res.Code)
	}
	if sink.got.MessageSID != "" {
		t.Fatal("invalid signature reached shared status sink")
	}
	signature := testTwilioSignature("secret", callback, form)
	if res := makeReq(signature); res.Code != http.StatusNoContent {
		t.Fatalf("valid callback failed: %d %s", res.Code, res.Body.String())
	}
	if sink.got.MessageSID != form.Get("MessageSid") || sink.got.Status != "delivered" {
		t.Fatalf("wrong callback forwarded: %+v", sink.got)
	}
}
