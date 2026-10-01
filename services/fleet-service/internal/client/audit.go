package client

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"time"
)

type TokenSource interface {
	Token(context.Context) (string, error)
}
type AuditPublisher struct {
	SharedURL string
	M2M       TokenSource
	HTTP      *http.Client
}

func (p AuditPublisher) Publish(ctx context.Context, payload []byte) error {
	if p.M2M == nil {
		return fmt.Errorf("audit token source is not configured")
	}
	token, err := p.M2M.Token(ctx)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.SharedURL+"/api/v1/shared/audit-events", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	httpClient := p.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("audit event rejected with status %d", resp.StatusCode)
	}
	return nil
}
