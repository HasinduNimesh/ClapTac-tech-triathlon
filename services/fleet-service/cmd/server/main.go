package main

import (
	"context"
	"log"
	"log/slog"
	"os"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/bootstrap"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/db"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/oauth"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/telemetry"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/fleet-service/internal/client"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/fleet-service/internal/handler"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/fleet-service/internal/store"
)

func main() {
	app, err := bootstrap.Setup("fleet-service", "fleet")
	if err != nil {
		log.Fatal(err)
	}
	pool, err := db.Open(context.Background(), app.Config.DatabaseURL, "fleet")
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	sharedURL := os.Getenv("SHARED_SERVICE_URL")
	if sharedURL == "" {
		sharedURL = "http://shared-service:8080"
	}
	repo := store.Store{Pool: pool}
	publisher := client.AuditPublisher{SharedURL: sharedURL, M2M: &oauth.TokenSource{TokenURL: getenv("OIDC_TOKEN_URL", "http://thunderid:8090/oauth2/token"), ClientID: os.Getenv("M2M_CLIENT_ID"), ClientSecret: os.Getenv("M2M_CLIENT_SECRET"), Scope: "audit:write", Resource: getenv("OIDC_AUDIENCE", "waypoint-api")}}
	go flushAuditOutbox(context.Background(), repo, publisher, app.Logger)
	h := handler.Handler{
		Authn:    app.Authenticator,
		Profiles: client.Profiles{SharedURL: sharedURL},
		Store:    repo,
	}
	if err := app.Run(func(r chi.Router) { h.Routes(r) }); err != nil {
		log.Fatal(err)
	}
}

func flushAuditOutbox(ctx context.Context, repo store.Store, publisher client.AuditPublisher, logger *slog.Logger) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pending, oldest, healthErr := repo.AuditOutboxHealth(ctx)
			if healthErr != nil {
				logger.Warn("audit_outbox_health_read_failed", "error", healthErr)
			} else {
				telemetry.FleetAuditOutboxPending.Set(float64(pending))
				telemetry.FleetAuditOutboxOldestSeconds.Set(oldest)
			}
			items, err := repo.ClaimPendingAudit(ctx, 2)
			if err != nil {
				logger.Warn("audit_outbox_read_failed", "error", err)
				continue
			}
			for _, item := range items {
				publishCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
				err := publisher.Publish(publishCtx, item.Payload)
				cancel()
				message := ""
				if err != nil {
					message = err.Error()
					logger.Warn("audit_outbox_publish_failed", "eventId", item.EventID, "error", err)
				}
				if markErr := repo.MarkAudit(ctx, item.EventID, message); markErr != nil {
					logger.Error("audit_outbox_ack_failed", "eventId", item.EventID, "error", markErr)
				}
			}
		}
	}
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
