package client

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type rt func(*http.Request) (*http.Response, error)

func (f rt) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestQueueShortfallNoticePayload(t *testing.T) {
	tr := rt(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/api/v1/shared/internal/notifications/enqueue" {
			t.Fatal(r.URL)
		}
		b, _ := io.ReadAll(r.Body)
		for _, w := range []string{`"eventKey":"shortfall:I1:HOLD"`, `"type":"LOAD_SHORTFALL"`, `"units":4`, `"reason":"HOLD"`, `"outletId":"OUT1"`} {
			if !strings.Contains(string(b), w) {
				t.Fatalf("missing %s in %s", w, b)
			}
		}
		return &http.Response{StatusCode: 202, Body: io.NopCloser(strings.NewReader(`{}`)), Request: r}, nil
	})
	p := Peers{SharedURL: "http://shared.test", HTTP: &http.Client{Transport: tr}}
	if err := p.QueueShortfallNotice(context.Background(), "shortfall:I1:HOLD", "OUT1", "ORD-1", "HOLD", 4); err != nil {
		t.Fatal(err)
	}
}
