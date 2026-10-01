package notifyworker

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/shared-service/internal/store"
)

type testQueue struct {
	item                  *store.PendingNotification
	claimed               bool
	status, message, code string
	finishErr             error
}

func (q *testQueue) ClaimNotification(context.Context) (*store.PendingNotification, error) {
	q.claimed = true
	if q.item == nil {
		return nil, nil
	}
	n := q.item
	q.item = nil
	return n, nil
}
func (q *testQueue) FinishNotification(_ context.Context, _ int64, status, message, code string) error {
	q.status = status
	q.message = message
	q.code = code
	return q.finishErr
}
func (q *testQueue) RecoverSendingNotifications(context.Context) error      { return nil }
func (q *testQueue) PurgeExpiredNotificationPayloads(context.Context) error { return nil }
func (q *testQueue) NotificationOutboxStats(context.Context) (store.NotificationOutboxStats, error) {
	return store.NotificationOutboxStats{}, nil
}

type testTokens struct {
	token string
	err   error
}

func (t testTokens) Token(context.Context) (string, error) { return t.token, t.err }

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func resp(r *http.Request, status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header), Request: r}
}

func TestProcessOneRecordsQueuedSIDAndBearerBoundary(t *testing.T) {
	q := &testQueue{item: &store.PendingNotification{ID: 9, Phone: "+94771112222", Body: "hello"}}
	r := Runner{Store: q, Tokens: testTokens{token: "m2m"}, IntegrationURL: "http://integration", HTTP: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("Authorization") != "Bearer m2m" || req.URL.Path != "/api/v1/integrations/notifications/send" {
			t.Fatalf("unexpected request: %s %s", req.URL, req.Header.Get("Authorization"))
		}
		return resp(req, http.StatusAccepted, `{"status":"queued","providerMessageId":"SM123"}`), nil
	})}}
	worked, err := r.ProcessOne(context.Background())
	if !worked || err != nil || q.status != "QUEUED" || q.message != "SM123" {
		t.Fatalf("process result worked=%v err=%v queue=%+v", worked, err, q)
	}
}

func TestProcessOneNeverRetriesAmbiguousTransportFailure(t *testing.T) {
	q := &testQueue{item: &store.PendingNotification{ID: 10, Phone: "+94771112222", Body: "hello"}}
	r := Runner{Store: q, Tokens: testTokens{token: "m2m"}, IntegrationURL: "http://integration", HTTP: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("connection reset") })}}
	worked, err := r.ProcessOne(context.Background())
	if !worked || err == nil || q.status != "UNKNOWN" || q.code != "PROVIDER_OUTCOME_UNKNOWN" {
		t.Fatalf("ambiguous result worked=%v err=%v queue=%+v", worked, err, q)
	}
}

func TestProcessOneDoesNotClaimUntilM2MTokenIsAvailable(t *testing.T) {
	q := &testQueue{item: &store.PendingNotification{ID: 11, Phone: "+94771112222", Body: "hello"}}
	r := Runner{Store: q, Tokens: testTokens{err: errors.New("oidc unavailable")}, IntegrationURL: "http://integration"}
	if worked, err := r.ProcessOne(context.Background()); worked || err == nil || q.claimed {
		t.Fatalf("token failure should preserve pending item: worked=%v err=%v claimed=%v", worked, err, q.claimed)
	}
}
