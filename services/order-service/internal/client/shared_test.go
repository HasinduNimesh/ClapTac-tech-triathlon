package client

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type testToken string

func (t testToken) Token(context.Context) (string, error) { return string(t), nil }

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSharedCutoffPolicyUsesScopedInternalEndpoint(t *testing.T) {
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/api/v1/shared/policies/current" {
			t.Fatalf("unexpected URL %s", r.URL)
		}
		if r.Header.Get("Authorization") != "Bearer policy-token" {
			t.Fatal("policy read must be authenticated with the machine credential")
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"policy":{"cutoffLocalTime":"15:30:00"}}`)), Request: r}, nil
	})
	s := Shared{BaseURL: "http://shared.test", M2M: testToken("policy-token"), HTTP: &http.Client{Transport: transport}}
	value, err := s.CutoffLocalTime()
	if err != nil || value != "15:30:00" {
		t.Fatalf("cutoff=%q err=%v", value, err)
	}
}

func TestSharedOperatingCalendarReadsVersionedDays(t *testing.T) {
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/api/v1/shared/calendar" || r.URL.Query().Get("from") != "2026-09-28" || r.URL.Query().Get("to") != "2026-10-04" {
			t.Fatalf("unexpected calendar request %s", r.URL)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"items":[{"date":"2026-09-28","isOperating":true},{"date":"2026-10-04","isOperating":false}]}`)), Request: r}, nil
	})
	s := Shared{BaseURL: "http://shared.test", M2M: testToken("policy-token"), HTTP: &http.Client{Transport: transport}}
	days, err := s.OperatingDays("2026-09-28", "2026-10-04")
	if err != nil || len(days) != 2 || !days[0].IsOperating || days[1].IsOperating {
		t.Fatalf("calendar=%+v err=%v", days, err)
	}
}
