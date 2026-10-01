package approvals

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

type Status string

const (
	StatusPending  Status = "pending"
	StatusApproved Status = "approved"
	StatusRejected Status = "rejected"
)

var ErrNotFound = errors.New("approval not found")
var ErrExpired = errors.New("approval expired")
var ErrNotOwner = errors.New("approval belongs to another user")
var ErrAlreadyDecided = errors.New("approval is no longer pending")
var ErrCapacity = errors.New("approval capacity reached")

type Request struct {
	ID        string         `json:"id"`
	Tool      string         `json:"tool"`
	Args      map[string]any `json:"args"`
	ArgsHash  string         `json:"args_hash"`
	ActorID   string         `json:"actor_sub"`
	Status    Status         `json:"status"`
	Reason    string         `json:"reason,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	ExpiresAt time.Time      `json:"expires_at"`
}

type Service interface {
	Propose(ctx context.Context, req Request) (Request, error)
	Decide(ctx context.Context, id, actor string, approved bool, reason string) (Request, error)
	Get(ctx context.Context, id, actor string) (Request, error)
}

type Memory struct {
	mu    sync.Mutex
	items map[string]Request
	now   func() time.Time
}

func NewMemory() *Memory { return &Memory{items: map[string]Request{}, now: time.Now} }

func (m *Memory) Propose(_ context.Context, req Request) (Request, error) {
	if req.ActorID == "" || req.Tool == "" || len(req.Args) == 0 {
		return Request{}, errors.New("invalid approval proposal")
	}
	args, err := json.Marshal(req.Args)
	if err != nil {
		return Request{}, fmt.Errorf("invalid approval arguments: %w", err)
	}
	if req.ID == "" {
		b := make([]byte, 16)
		if _, err := rand.Read(b); err != nil {
			return Request{}, err
		}
		req.ID = hex.EncodeToString(b)
	}
	if req.ArgsHash == "" {
		h := sha256.Sum256(args)
		req.ArgsHash = hex.EncodeToString(h[:])
	}
	now := m.now().UTC()
	req.Status, req.CreatedAt, req.ExpiresAt = StatusPending, now, now.Add(5*time.Minute)
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, item := range m.items {
		if !now.Before(item.ExpiresAt) {
			delete(m.items, id)
		}
	}
	if len(m.items) >= 1000 {
		return Request{}, ErrCapacity
	}
	if _, exists := m.items[req.ID]; exists {
		return Request{}, errors.New("approval id collision")
	}
	m.items[req.ID] = req
	return req, nil
}

func (m *Memory) Decide(_ context.Context, id, actor string, approved bool, reason string) (Request, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	req, ok := m.items[id]
	if !ok {
		return Request{}, ErrNotFound
	}
	if req.ActorID != actor {
		return Request{}, ErrNotOwner
	}
	if !m.now().Before(req.ExpiresAt) {
		delete(m.items, id)
		return Request{}, ErrExpired
	}
	if req.Status != StatusPending {
		return Request{}, ErrAlreadyDecided
	}
	if approved {
		req.Status = StatusApproved
	} else {
		req.Status = StatusRejected
	}
	req.Reason = reason
	m.items[id] = req
	return req, nil
}

func (m *Memory) Get(_ context.Context, id, actor string) (Request, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	req, ok := m.items[id]
	if !ok {
		return Request{}, ErrNotFound
	}
	if req.ActorID != actor {
		return Request{}, ErrNotOwner
	}
	if !m.now().Before(req.ExpiresAt) {
		delete(m.items, id)
		return Request{}, ErrExpired
	}
	return req, nil
}
