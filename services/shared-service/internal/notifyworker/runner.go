package notifyworker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/telemetry"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/shared-service/internal/store"
)

type TokenSource interface {
	Token(context.Context) (string, error)
}

type QueueStore interface {
	ClaimNotification(context.Context) (*store.PendingNotification, error)
	FinishNotification(context.Context, int64, string, string, string) error
	RecoverSendingNotifications(context.Context) error
	PurgeExpiredNotificationPayloads(context.Context) error
	NotificationOutboxStats(context.Context) (store.NotificationOutboxStats, error)
}

type Runner struct {
	Store          QueueStore
	Tokens         TokenSource
	IntegrationURL string
	HTTP           *http.Client
	Logger         *slog.Logger
}

func (r Runner) client() *http.Client {
	if r.HTTP != nil {
		return r.HTTP
	}
	return &http.Client{Timeout: 20 * time.Second}
}

func (r Runner) Run(ctx context.Context) {
	delay := time.Second
	nextPurge := time.Now()
	nextStats := time.Now()
	for {
		if time.Now().After(nextPurge) {
			if err := r.Store.PurgeExpiredNotificationPayloads(ctx); err != nil && r.Logger != nil {
				r.Logger.Error("notification_payload_purge_failed", "error", err)
			}
			nextPurge = time.Now().Add(time.Hour)
		}
		if time.Now().After(nextStats) {
			if stats, err := r.Store.NotificationOutboxStats(ctx); err != nil {
				if r.Logger != nil {
					r.Logger.Error("notification_metrics_failed", "error", err)
				}
			} else {
				telemetry.NotificationOutboxPending.Set(float64(stats.Pending))
				telemetry.NotificationOutboxOldest.Set(stats.OldestAgeSeconds)
				telemetry.NotificationsAwaitingStatus.Set(float64(stats.AwaitingStatus))
				telemetry.NotificationsAwaitingOldest.Set(stats.AwaitingOldestSeconds)
			}
			nextStats = time.Now().Add(30 * time.Second)
		}
		worked, err := r.ProcessOne(ctx)
		if worked {
			delay = time.Second
			continue
		}
		if err != nil {
			if r.Logger != nil {
				r.Logger.Error("notification_worker_error", "error", err)
			}
			if delay < 30*time.Second {
				delay *= 2
			}
		} else {
			delay = time.Second
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (r Runner) ProcessOne(ctx context.Context) (bool, error) {
	if r.Tokens == nil || r.IntegrationURL == "" {
		return false, fmt.Errorf("notification worker is not configured")
	}
	if err := r.Store.RecoverSendingNotifications(ctx); err != nil {
		return false, err
	}
	// Acquire credentials before claiming so token-service failure leaves the
	// durable event pending instead of consuming its one provider attempt.
	token, err := r.Tokens.Token(ctx)
	if err != nil {
		return false, err
	}
	n, err := r.Store.ClaimNotification(ctx)
	if err != nil || n == nil {
		return false, err
	}
	body, _ := json.Marshal(map[string]string{"to": n.Phone, "body": n.Body})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.IntegrationURL+"/api/v1/integrations/notifications/send", bytes.NewReader(body))
	if err != nil {
		_ = r.finish(ctx, n, "FAILED", "", "REQUEST_BUILD_FAILED")
		return true, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := r.client().Do(req)
	if err != nil {
		markErr := r.finish(ctx, n, "UNKNOWN", "", "PROVIDER_OUTCOME_UNKNOWN")
		return true, join(err, markErr)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode == http.StatusAccepted {
		var result struct {
			Status    string `json:"status"`
			MessageID string `json:"providerMessageId"`
		}
		if err := json.Unmarshal(b, &result); err != nil {
			return true, r.finish(ctx, n, "UNKNOWN", "", "INVALID_PROVIDER_RESPONSE")
		}
		if result.Status == "queued" && result.MessageID != "" {
			return true, r.finish(ctx, n, "QUEUED", result.MessageID, "")
		}
		if result.Status == "unknown" {
			return true, r.finish(ctx, n, "UNKNOWN", "", "PROVIDER_OUTCOME_UNKNOWN")
		}
		return true, r.finish(ctx, n, "UNKNOWN", "", "INVALID_PROVIDER_STATUS")
	}
	markErr := r.finish(ctx, n, "FAILED", "", fmt.Sprintf("INTEGRATION_HTTP_%d", resp.StatusCode))
	return true, join(fmt.Errorf("notification integration returned HTTP %d", resp.StatusCode), markErr)
}

func (r Runner) finish(ctx context.Context, n *store.PendingNotification, status, messageID, errorCode string) error {
	err := r.Store.FinishNotification(ctx, n.ID, status, messageID, errorCode)
	if err == nil {
		telemetry.NotificationEvents.WithLabelValues(n.EventType, status).Inc()
	}
	return err
}

func join(first, second error) error {
	if second != nil {
		return fmt.Errorf("%v; recording outcome failed: %w", first, second)
	}
	return first
}
