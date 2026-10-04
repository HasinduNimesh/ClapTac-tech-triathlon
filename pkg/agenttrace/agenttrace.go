// Package agenttrace records what an agent did during one request and why.
//
// A Trace is a short list of Steps. Each Step carries a Reason: either a rule the
// code applied ("sensitive tool, so held for approval") or the rationale the model
// stated. Traces are sent best-effort to the agent manager (infrastructure/agent-manager);
// a missing, slow or failing manager never affects the agent request.
//
// Traces hold bounded facts only: names, counts, hashes, short reasons. Callers
// must not put bearer tokens, provider keys or raw request text in them, and
// Sanitize drops secret-looking keys and truncates everything as a second line of defence.
package agenttrace

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	MaxSteps        = 100
	MaxReasonChars  = 400
	MaxNameChars    = 120
	MaxDetailKeys   = 16
	MaxDetailString = 200
)

// Step kinds.
const (
	KindDecision  = "decision"  // a rule the code applied
	KindLLM       = "llm"       // a model call
	KindTool      = "tool"      // a business-service call
	KindGuardrail = "guardrail" // validation or authorization check
	KindApproval  = "approval"  // human approval gate
	KindError     = "error"
)

// Trace statuses.
const (
	StatusOK              = "ok"
	StatusError           = "error"
	StatusDenied          = "denied"
	StatusPendingApproval = "pending_approval"
)

type Step struct {
	Seq        int            `json:"seq"`
	At         time.Time      `json:"at"`
	Kind       string         `json:"kind"`
	Name       string         `json:"name"`
	Reason     string         `json:"reason"`
	Outcome    string         `json:"outcome,omitempty"`
	DurationMS int64          `json:"duration_ms,omitempty"`
	Detail     map[string]any `json:"detail,omitempty"`
}

type Trace struct {
	ID            string    `json:"id"`             // unique per trace
	CorrelationID string    `json:"correlation_id"` // X-Correlation-ID of the request; may repeat across traces
	Agent         string    `json:"agent"`          // e.g. orchestrator, order-assistant
	Operation     string    `json:"operation"`      // e.g. chat, approval.decide
	ActorID       string    `json:"actor_id"`
	Ref           string    `json:"ref,omitempty"` // related id, e.g. an approval id
	StartedAt     time.Time `json:"started_at"`
	EndedAt       time.Time `json:"ended_at"`
	Status        string    `json:"status"`
	InputHash     string    `json:"input_hash,omitempty"`
	InputSize     int       `json:"input_chars,omitempty"`
	Steps         []Step    `json:"steps"`
}

// Sink receives finished traces. Implementations must not block.
type Sink interface {
	Submit(Trace)
}

// Recorder collects the steps of one request. All methods are safe on a nil
// *Recorder, so call sites need no checks when tracing is off.
type Recorder struct {
	mu    sync.Mutex
	sink  Sink
	trace Trace
	done  bool
}

// Start begins a trace. It returns nil when sink is nil.
func Start(sink Sink, agent, operation, actorID, correlationID string) *Recorder {
	if sink == nil {
		return nil
	}
	return &Recorder{sink: sink, trace: Trace{ID: NewID(), CorrelationID: correlationID, Agent: agent, Operation: operation, ActorID: actorID, StartedAt: time.Now().UTC(), Status: StatusError}}
}

// NewID returns a random 16-hex-character trace id.
func NewID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Input records a size and hash of the request text, never the text itself.
func (r *Recorder) Input(hash string, chars int) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.trace.InputHash, r.trace.InputSize = hash, chars
	r.mu.Unlock()
}

func (r *Recorder) SetActor(actorID string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.trace.ActorID = actorID
	r.mu.Unlock()
}

func (r *Recorder) SetRef(ref string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.trace.Ref = ref
	r.mu.Unlock()
}

// Add records a step that took no measurable time.
func (r *Recorder) Add(kind, name, reason, outcome string, detail map[string]any) {
	r.add(Step{Kind: kind, Name: name, Reason: reason, Outcome: outcome, Detail: detail})
}

// AddSince records a step and how long it took since start.
func (r *Recorder) AddSince(start time.Time, kind, name, reason, outcome string, detail map[string]any) {
	r.add(Step{Kind: kind, Name: name, Reason: reason, Outcome: outcome, Detail: detail, DurationMS: time.Since(start).Milliseconds()})
}

func (r *Recorder) add(s Step) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.done || len(r.trace.Steps) >= MaxSteps {
		return
	}
	s.Seq = len(r.trace.Steps) + 1
	s.At = time.Now().UTC()
	r.trace.Steps = append(r.trace.Steps, s)
}

// Finish ends the trace and hands it to the sink. Only the first call counts.
func (r *Recorder) Finish(status string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	if r.done {
		r.mu.Unlock()
		return
	}
	r.done = true
	r.trace.Status = status
	r.trace.EndedAt = time.Now().UTC()
	t := r.trace
	r.mu.Unlock()
	r.sink.Submit(t)
}

// Sanitize bounds a trace and removes secret-looking detail keys. The agent
// manager runs it on everything it receives; clients run it before sending.
func Sanitize(t Trace) Trace {
	t.ID = clip(t.ID, MaxNameChars)
	t.CorrelationID = clip(t.CorrelationID, MaxNameChars)
	t.Agent = clip(t.Agent, MaxNameChars)
	t.Operation = clip(t.Operation, MaxNameChars)
	t.ActorID = clip(t.ActorID, MaxNameChars)
	t.Ref = clip(t.Ref, MaxNameChars)
	t.Status = clip(t.Status, 32)
	t.InputHash = clip(t.InputHash, 64)
	if len(t.Steps) > MaxSteps {
		t.Steps = t.Steps[:MaxSteps]
	}
	steps := make([]Step, len(t.Steps))
	for i, s := range t.Steps {
		s.Seq = i + 1
		s.Kind = clip(s.Kind, 32)
		s.Name = clip(s.Name, MaxNameChars)
		s.Reason = clip(s.Reason, MaxReasonChars)
		s.Outcome = clip(s.Outcome, MaxNameChars)
		s.Detail = sanitizeDetail(s.Detail)
		steps[i] = s
	}
	t.Steps = steps
	return t
}

func sanitizeDetail(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		if len(out) >= MaxDetailKeys {
			break
		}
		if secretKey(k) {
			continue
		}
		switch x := v.(type) {
		case nil, bool, int, int64, float64:
			out[clip(k, MaxNameChars)] = x
		case string:
			out[clip(k, MaxNameChars)] = clip(x, MaxDetailString)
		default:
			b, err := json.Marshal(x)
			if err != nil {
				b = []byte(fmt.Sprint(x))
			}
			out[clip(k, MaxNameChars)] = clip(string(b), MaxDetailString)
		}
	}
	return out
}

func secretKey(k string) bool {
	k = strings.ToLower(k)
	for _, bad := range []string{"token", "secret", "password", "authorization", "apikey", "api_key", "accesskey", "signedurl"} {
		if strings.Contains(k, bad) {
			return true
		}
	}
	return false
}

// clip shortens s to at most n runes.
func clip(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

// HTTPSink posts traces to the agent manager from one background goroutine.
// The queue is bounded; when it is full the trace is dropped.
type HTTPSink struct {
	url    string
	token  string
	client *http.Client
	queue  chan Trace
}

// NewHTTPSink returns a nil Sink when baseURL is empty, which turns tracing off.
func NewHTTPSink(baseURL, token string) Sink {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil
	}
	s := &HTTPSink{url: baseURL + "/api/v1/traces", token: token, client: &http.Client{Timeout: 3 * time.Second}, queue: make(chan Trace, 256)}
	go s.run()
	return s
}

func (s *HTTPSink) Submit(t Trace) {
	select {
	case s.queue <- Sanitize(t):
	default: // full: drop rather than slow the agent down
	}
}

func (s *HTTPSink) run() {
	for t := range s.queue {
		body, err := json.Marshal(t)
		if err != nil {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url, bytes.NewReader(body))
		if err == nil {
			req.Header.Set("Content-Type", "application/json")
			if s.token != "" {
				req.Header.Set("Authorization", "Bearer "+s.token)
			}
			if resp, err := s.client.Do(req); err == nil {
				resp.Body.Close()
			}
		}
		cancel()
	}
}
