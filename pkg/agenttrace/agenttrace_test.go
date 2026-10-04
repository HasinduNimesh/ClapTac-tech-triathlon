package agenttrace

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type memSink struct{ got []Trace }

func (m *memSink) Submit(t Trace) { m.got = append(m.got, t) }

func TestRecorderCollectsStepsAndFinishesOnce(t *testing.T) {
	sink := &memSink{}
	rec := Start(sink, "orchestrator", "chat", "u1", "corr-1")
	rec.Input("abc", 12)
	rec.Add(KindDecision, "authorize", "role allows tools", "ok", map[string]any{"tools": 3})
	rec.AddSince(time.Now().Add(-5*time.Millisecond), KindTool, "GetOrders", "needed current orders", "ok", nil)
	rec.Finish(StatusOK)
	rec.Finish(StatusError)
	rec.Add(KindError, "late", "ignored after finish", "", nil)

	if len(sink.got) != 1 {
		t.Fatalf("want one submitted trace, got %d", len(sink.got))
	}
	tr := sink.got[0]
	if tr.Status != StatusOK || tr.CorrelationID != "corr-1" || tr.ID == "" || tr.InputSize != 12 || len(tr.Steps) != 2 {
		t.Fatalf("unexpected trace: %+v", tr)
	}
	if tr.Steps[0].Seq != 1 || tr.Steps[1].Seq != 2 || tr.Steps[1].DurationMS < 5 {
		t.Fatalf("unexpected steps: %+v", tr.Steps)
	}
}

func TestNilRecorderIsSafe(t *testing.T) {
	rec := Start(nil, "a", "b", "c", "d")
	rec.Input("h", 1)
	rec.SetActor("x")
	rec.SetRef("y")
	rec.Add(KindDecision, "n", "r", "", nil)
	rec.Finish(StatusOK)
}

func TestSanitizeBoundsAndDropsSecrets(t *testing.T) {
	long := strings.Repeat("é", 1000)
	steps := make([]Step, MaxSteps+20)
	steps[0] = Step{Kind: KindTool, Name: "x", Reason: long, Detail: map[string]any{
		"bearerToken": "secret", "Authorization": "Bearer x", "apiKey": "k", "ok": long, "n": 3, "nested": map[string]any{"a": 1},
	}}
	got := Sanitize(Trace{Steps: steps})
	if len(got.Steps) != MaxSteps {
		t.Fatalf("steps not capped: %d", len(got.Steps))
	}
	s := got.Steps[0]
	if len([]rune(s.Reason)) != MaxReasonChars {
		t.Fatalf("reason not clipped: %d", len([]rune(s.Reason)))
	}
	for _, k := range []string{"bearerToken", "Authorization", "apiKey"} {
		if _, ok := s.Detail[k]; ok {
			t.Fatalf("%s should have been dropped", k)
		}
	}
	if len([]rune(s.Detail["ok"].(string))) != MaxDetailString || s.Detail["n"] != 3 {
		t.Fatalf("detail not bounded: %+v", s.Detail)
	}
	if _, ok := s.Detail["nested"].(string); !ok {
		t.Fatalf("nested values should be flattened to a bounded string")
	}
}

func TestHTTPSinkPostsWithBearer(t *testing.T) {
	received := make(chan Trace, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/traces" || r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var tr Trace
		_ = json.NewDecoder(r.Body).Decode(&tr)
		received <- tr
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	sink := NewHTTPSink(srv.URL+"/", "tok")
	rec := Start(sink, "orchestrator", "chat", "u1", "corr-9")
	rec.Add(KindDecision, "n", "r", "", nil)
	rec.Finish(StatusOK)

	select {
	case tr := <-received:
		if tr.CorrelationID != "corr-9" || len(tr.Steps) != 1 {
			t.Fatalf("unexpected trace: %+v", tr)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("trace was not delivered")
	}
}

func TestNewHTTPSinkDisabledWithoutURL(t *testing.T) {
	if NewHTTPSink("  ", "") != nil {
		t.Fatal("empty URL must disable tracing")
	}
}
