package handler

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
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

func gatewaySignature(secret, body string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = io.WriteString(mac, body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func postGatewayEvent(h Handler, body, signature string) *httptest.ResponseRecorder {
	router := chi.NewRouter()
	h.Routes(router)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/integrations/notifications/gateway", strings.NewReader(body))
	req.Header.Set("X-CG-Signature", signature)
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	return res
}

func TestGatewayWebhookVerifiesSignatureAndForwardsOutboundStatus(t *testing.T) {
	const secret = "whsec_test"
	const sent = `{"id":"d1","type":"SMS_SENT","data":{"request_id":"req_abcd1234"}}`
	sink := &fixedStatusSink{}
	h := Handler{GatewayWebhookSecret: secret, StatusSink: sink}

	if res := postGatewayEvent(h, sent, gatewaySignature("other", sent)); res.Code != http.StatusForbidden {
		t.Fatalf("bad signature: %d", res.Code)
	}
	if sink.got.MessageSID != "" {
		t.Fatal("an unsigned event reached the status sink")
	}
	if res := postGatewayEvent(h, sent, gatewaySignature(secret, sent)); res.Code != http.StatusNoContent {
		t.Fatalf("valid event: %d %s", res.Code, res.Body.String())
	}
	if sink.got.MessageSID != "cg:req_abcd1234" || sink.got.Status != "sent" {
		t.Fatalf("forwarded %+v", sink.got)
	}

	const failed = `{"id":"d2","type":"SMS_FAILED","data":{"request_id":"req_abcd1234"}}`
	if res := postGatewayEvent(h, failed, gatewaySignature(secret, failed)); res.Code != http.StatusNoContent {
		t.Fatalf("failed event: %d", res.Code)
	}
	if sink.got.Status != "failed" || sink.got.ErrorCode != "GATEWAY_SMS_FAILED" {
		t.Fatalf("forwarded %+v", sink.got)
	}
}

func TestGatewayWebhookAcknowledgesUnrelatedEventsAndRetriesStorageFailures(t *testing.T) {
	const secret = "whsec_test"
	sink := &fixedStatusSink{}
	h := Handler{GatewayWebhookSecret: secret, StatusSink: sink}

	const inbound = `{"id":"d3","type":"SMS_RECEIVED","data":{"from":"+94771112222"}}`
	if res := postGatewayEvent(h, inbound, gatewaySignature(secret, inbound)); res.Code != http.StatusNoContent || sink.got.MessageSID != "" {
		t.Fatalf("an inbound SMS must be acknowledged and ignored: %d %+v", res.Code, sink.got)
	}
	const garbage = "not json"
	if res := postGatewayEvent(h, garbage, gatewaySignature(secret, garbage)); res.Code != http.StatusBadRequest {
		t.Fatalf("garbage: %d", res.Code)
	}
	const sent = `{"id":"d1","type":"SMS_SENT","data":{"request_id":"req_abcd1234"}}`
	h.StatusSink = &fixedStatusSink{err: io.ErrUnexpectedEOF}
	if res := postGatewayEvent(h, sent, gatewaySignature(secret, sent)); res.Code != http.StatusInternalServerError {
		t.Fatalf("a storage failure must answer 500 so the gateway retries: %d", res.Code)
	}
}

func TestGatewayWebhookIsOffUntilConfigured(t *testing.T) {
	const sent = `{"id":"d1","type":"SMS_SENT","data":{"request_id":"req_abcd1234"}}`
	if res := postGatewayEvent(Handler{StatusSink: &fixedStatusSink{}}, sent, gatewaySignature("", sent)); res.Code != http.StatusNotImplemented {
		t.Fatalf("no secret: %d", res.Code)
	}
	if res := postGatewayEvent(Handler{GatewayWebhookSecret: "s"}, sent, gatewaySignature("s", sent)); res.Code != http.StatusNotImplemented {
		t.Fatalf("no sink: %d", res.Code)
	}
}
