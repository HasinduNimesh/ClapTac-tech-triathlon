package client

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type staticToken string

func (t staticToken) Token(context.Context) (string, error) { return string(t), nil }

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestAuditPublisherSendsM2MEvent(t *testing.T) {
	transport := transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "http://shared.test/api/v1/shared/audit-events" {
			t.Fatalf("unexpected audit URL %s", r.URL)
		}
		if r.Header.Get("Authorization") != "Bearer fleet-token" {
			t.Fatal("missing audit service authorization")
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "MASTER_DATA_VEHICLE_UPDATED") {
			t.Fatalf("unexpected audit payload %s", body)
		}
		return &http.Response{StatusCode: http.StatusAccepted, Body: io.NopCloser(strings.NewReader(`{"status":"ingested"}`)), Request: r}, nil
	})
	p := AuditPublisher{SharedURL: "http://shared.test", M2M: staticToken("fleet-token"), HTTP: &http.Client{Transport: transport}}
	if err := p.Publish(context.Background(), []byte(`{"action":"MASTER_DATA_VEHICLE_UPDATED"}`)); err != nil {
		t.Fatal(err)
	}
}
