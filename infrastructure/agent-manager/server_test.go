package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/agenttrace"
)

func sample(id, agent, status string) agenttrace.Trace {
	now := time.Now().UTC()
	return agenttrace.Trace{ID: id, CorrelationID: "corr-" + id, Agent: agent, Operation: "chat", ActorID: "u1", StartedAt: now, EndedAt: now.Add(40 * time.Millisecond), Status: status,
		Steps: []agenttrace.Step{{Kind: agenttrace.KindTool, Name: "GetOrders", Reason: "needed the caller's open orders"}}}
}

func do(h http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestIngestThenListAndGet(t *testing.T) {
	store, _ := NewStore(10, "")
	h := Server{Store: store, IngestToken: "in", ViewToken: "view"}.Handler()

	body, _ := json.Marshal(sample("a1", "orchestrator", agenttrace.StatusOK))
	if rec := do(h, "POST", "/api/v1/traces", "in", string(body)); rec.Code != http.StatusAccepted {
		t.Fatalf("ingest: %d %s", rec.Code, rec.Body)
	}
	rec := do(h, "GET", "/api/v1/traces?agent=orchestrator", "view", "")
	var list struct {
		Items  []Summary `json:"items"`
		Agents []string  `json:"agents"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if rec.Code != 200 || len(list.Items) != 1 || list.Items[0].Steps != 1 || list.Agents[0] != "orchestrator" {
		t.Fatalf("list: %d %s", rec.Code, rec.Body)
	}
	rec = do(h, "GET", "/api/v1/traces/a1", "view", "")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "needed the caller's open orders") {
		t.Fatalf("get: %d %s", rec.Code, rec.Body)
	}
	if do(h, "GET", "/api/v1/traces/nope", "view", "").Code != http.StatusNotFound {
		t.Fatal("unknown id should be 404")
	}
}

func TestTokensAreSeparateAndRequired(t *testing.T) {
	store, _ := NewStore(10, "")
	h := Server{Store: store, IngestToken: "in", ViewToken: "view"}.Handler()
	body, _ := json.Marshal(sample("a1", "orchestrator", "ok"))

	if do(h, "POST", "/api/v1/traces", "", string(body)).Code != 401 || do(h, "POST", "/api/v1/traces", "view", string(body)).Code != 401 {
		t.Fatal("ingest must need the ingest token (the view token is not enough)")
	}
	if do(h, "GET", "/api/v1/traces", "", "").Code != 401 || do(h, "GET", "/api/v1/traces", "in", "").Code != 401 {
		t.Fatal("reads must need the view token (the ingest token is not enough)")
	}
	if do(h, "GET", "/health/live", "", "").Code != 200 || do(h, "GET", "/", "", "").Code != 200 {
		t.Fatal("health and the static page stay open")
	}
}

func TestIngestRejectsBadInputAndSanitizes(t *testing.T) {
	store, _ := NewStore(10, "")
	h := Server{Store: store}.Handler()
	for _, bad := range []string{`{`, `{"agent":"","operation":"x"}`, `{"agent":"a","operation":"o"} {}`, `{"agent":"a","operation":"o","extra":`} {
		if rec := do(h, "POST", "/api/v1/traces", "", bad); rec.Code != http.StatusBadRequest {
			t.Fatalf("%q should be 400, got %d", bad, rec.Code)
		}
	}
	tr := sample("", "orchestrator", "ok")
	tr.Steps[0].Detail = map[string]any{"bearerToken": "abc", "tool": "GetOrders"}
	body, _ := json.Marshal(tr)
	if rec := do(h, "POST", "/api/v1/traces", "", string(body)); rec.Code != http.StatusAccepted {
		t.Fatalf("ingest without token locally: %d", rec.Code)
	}
	items := store.List(Filter{})
	got, _ := store.Get(items[0].ID)
	if _, leaked := got.Steps[0].Detail["bearerToken"]; leaked || got.Steps[0].Detail["tool"] != "GetOrders" {
		t.Fatalf("secret-looking detail key was stored: %+v", got.Steps[0].Detail)
	}
	if strings.Contains(do(h, "GET", "/api/v1/traces", "", "").Body.String(), "abc") {
		t.Fatal("secret value leaked through the API")
	}
}

func TestIngestDefaultsMissingTimes(t *testing.T) {
	store, _ := NewStore(10, "")
	h := Server{Store: store}.Handler()
	if rec := do(h, "POST", "/api/v1/traces", "", `{"id":"t","agent":"a","operation":"o","status":"ok"}`); rec.Code != http.StatusAccepted {
		t.Fatalf("ingest: %d", rec.Code)
	}
	got, _ := store.Get("t")
	if got.StartedAt.IsZero() || got.EndedAt.Before(got.StartedAt) {
		t.Fatalf("times not defaulted: %+v", got)
	}
}

func TestViewerSendsLockedDownHeaders(t *testing.T) {
	store, _ := NewStore(10, "")
	rec := do(Server{Store: store}.Handler(), "GET", "/", "", "")
	if !strings.Contains(rec.Header().Get("Content-Security-Policy"), "default-src 'none'") || rec.Header().Get("X-Frame-Options") != "DENY" {
		t.Fatalf("missing security headers: %v", rec.Header())
	}
}

func TestStoreEvictsOldestAndFilters(t *testing.T) {
	store, _ := NewStore(3, "")
	for _, id := range []string{"t1", "t2", "t3", "t4"} {
		agent, status := "orchestrator", "ok"
		if id == "t4" {
			agent, status = "order-assistant", "error"
		}
		_ = store.Add(sample(id, agent, status))
		time.Sleep(2 * time.Millisecond)
	}
	if _, ok := store.Get("t1"); ok {
		t.Fatal("oldest trace should have been evicted")
	}
	if got := store.List(Filter{}); len(got) != 3 || got[0].ID != "t4" {
		t.Fatalf("expected newest first, got %+v", got)
	}
	if got := store.List(Filter{Agent: "order-assistant"}); len(got) != 1 || got[0].ID != "t4" {
		t.Fatalf("agent filter: %+v", got)
	}
	if got := store.List(Filter{Status: "error"}); len(got) != 1 {
		t.Fatalf("status filter: %+v", got)
	}
	if got := store.List(Filter{Query: "OPEN ORDERS"}); len(got) != 3 {
		t.Fatalf("search should look in reasons, case-insensitively: %+v", got)
	}
}

func TestStorePersistsAndCompacts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "traces.jsonl")
	store, err := NewStore(2, path)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"a", "b", "c", "d", "e"} {
		if err := store.Add(sample(id, "orchestrator", "ok")); err != nil {
			t.Fatal(err)
		}
	}
	reopened, err := NewStore(2, path)
	if err != nil {
		t.Fatal(err)
	}
	got := reopened.List(Filter{})
	if len(got) != 2 || got[0].ID != "e" || got[1].ID != "d" {
		t.Fatalf("reopened store should keep the newest two: %+v", got)
	}
	raw, _ := os.ReadFile(path)
	if lines := strings.Count(string(raw), "\n"); lines != 2 {
		t.Fatalf("file should be compacted to 2 lines at startup, has %d", lines)
	}
}
