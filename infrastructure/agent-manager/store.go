package main

import (
	"bufio"
	"encoding/json"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/agenttrace"
)

// Store keeps the newest `max` traces in memory and, when a path is set, appends
// each one to a JSON-lines file so they survive a restart. Nothing fancier than that.
type Store struct {
	mu       sync.RWMutex
	max      int
	path     string
	order    []string // oldest first
	byID     map[string]agenttrace.Trace
	appended int // lines written since the file was last compacted
}

type Summary struct {
	ID            string    `json:"id"`
	CorrelationID string    `json:"correlation_id"`
	Agent         string    `json:"agent"`
	Operation     string    `json:"operation"`
	ActorID       string    `json:"actor_id"`
	Ref           string    `json:"ref,omitempty"`
	StartedAt     time.Time `json:"started_at"`
	DurationMS    int64     `json:"duration_ms"`
	Status        string    `json:"status"`
	Steps         int       `json:"steps"`
}

type Filter struct {
	Agent, Status, Query string
	Limit                int
}

func NewStore(max int, path string) (*Store, error) {
	if max < 1 {
		max = 1000
	}
	s := &Store{max: max, path: path, byID: map[string]agenttrace.Trace{}}
	if path == "" {
		return s, nil
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, s.compact()
}

func (s *Store) load() error {
	f, err := os.Open(s.path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for sc.Scan() {
		var t agenttrace.Trace
		if json.Unmarshal(sc.Bytes(), &t) == nil && t.ID != "" {
			s.insert(agenttrace.Sanitize(t))
		}
	}
	return sc.Err()
}

// insert adds or replaces a trace in memory and evicts the oldest over the cap.
func (s *Store) insert(t agenttrace.Trace) {
	if _, exists := s.byID[t.ID]; !exists {
		s.order = append(s.order, t.ID)
	}
	s.byID[t.ID] = t
	for len(s.order) > s.max {
		delete(s.byID, s.order[0])
		s.order = s.order[1:]
	}
}

func (s *Store) Add(t agenttrace.Trace) error {
	t = agenttrace.Sanitize(t)
	if t.ID == "" {
		t.ID = agenttrace.NewID()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.insert(t)
	if s.path == "" {
		return nil
	}
	if s.appended >= s.max {
		return s.compactLocked()
	}
	f, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	b, err := json.Marshal(t)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		return err
	}
	s.appended++
	return nil
}

func (s *Store) compact() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.compactLocked()
}

// compactLocked rewrites the file with only the traces still held in memory.
func (s *Store) compactLocked() error {
	tmp := s.path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(f)
	for _, id := range s.order {
		b, err := json.Marshal(s.byID[id])
		if err != nil {
			f.Close()
			return err
		}
		w.Write(b)
		w.WriteByte('\n')
	}
	if err := w.Flush(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	s.appended = 0
	return os.Rename(tmp, s.path)
}

func (s *Store) Get(id string) (agenttrace.Trace, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.byID[id]
	return t, ok
}

// List returns summaries newest first.
func (s *Store) List(f Filter) []Summary {
	if f.Limit < 1 || f.Limit > 500 {
		f.Limit = 100
	}
	q := strings.ToLower(strings.TrimSpace(f.Query))
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Summary, 0, f.Limit)
	for i := len(s.order) - 1; i >= 0 && len(out) < f.Limit; i-- {
		t := s.byID[s.order[i]]
		if f.Agent != "" && t.Agent != f.Agent {
			continue
		}
		if f.Status != "" && t.Status != f.Status {
			continue
		}
		if q != "" && !matches(t, q) {
			continue
		}
		out = append(out, Summary{ID: t.ID, CorrelationID: t.CorrelationID, Agent: t.Agent, Operation: t.Operation, ActorID: t.ActorID, Ref: t.Ref,
			StartedAt: t.StartedAt, DurationMS: t.EndedAt.Sub(t.StartedAt).Milliseconds(), Status: t.Status, Steps: len(t.Steps)})
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].StartedAt.After(out[b].StartedAt) })
	return out
}

// Agents lists the distinct agent names seen, for the viewer's filter.
func (s *Store) Agents() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	seen := map[string]bool{}
	for _, t := range s.byID {
		seen[t.Agent] = true
	}
	out := make([]string, 0, len(seen))
	for a := range seen {
		out = append(out, a)
	}
	sort.Strings(out)
	return out
}

func matches(t agenttrace.Trace, q string) bool {
	for _, v := range []string{t.ID, t.CorrelationID, t.ActorID, t.Ref, t.Operation, t.Agent} {
		if strings.Contains(strings.ToLower(v), q) {
			return true
		}
	}
	for _, st := range t.Steps {
		if strings.Contains(strings.ToLower(st.Name), q) || strings.Contains(strings.ToLower(st.Reason), q) {
			return true
		}
	}
	return false
}
