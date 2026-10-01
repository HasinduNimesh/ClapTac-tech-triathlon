package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type StatusUpdate struct {
	MessageSID string `json:"messageSid"`
	Status     string `json:"status"`
	ErrorCode  string `json:"errorCode,omitempty"`
}
type TokenSource interface {
	Token(context.Context) (string, error)
}

type SharedStatusSink struct {
	SharedURL string
	Tokens    TokenSource
	HTTP      *http.Client
}

func (s SharedStatusSink) UpdateStatus(ctx context.Context, u StatusUpdate) error {
	if s.Tokens == nil || s.SharedURL == "" {
		return fmt.Errorf("shared notification status sink is not configured")
	}
	token, err := s.Tokens.Token(ctx)
	if err != nil {
		return err
	}
	body, _ := json.Marshal(u)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.SharedURL+"/api/v1/shared/internal/notifications/status", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	client := s.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("shared notification status returned HTTP %d: %s", resp.StatusCode, string(b))
	}
	return nil
}
