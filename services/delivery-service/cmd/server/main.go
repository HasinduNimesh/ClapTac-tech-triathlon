package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/bootstrap"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/db"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/oauth"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/objectstore"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/delivery-service/internal/client"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/delivery-service/internal/handler"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/delivery-service/internal/retention"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/delivery-service/internal/service"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/delivery-service/internal/store"
)

func main() {
	app, err := bootstrap.Setup("delivery-service", "delivery")
	if err != nil {
		log.Fatal(err)
	}
	pool, err := db.Open(context.Background(), app.Config.DatabaseURL, "delivery")
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	peers := client.Peers{
		LoadingURL: getenv("LOADING_SERVICE_URL", "http://loading-service:8080"),
		SharedURL:  getenv("SHARED_SERVICE_URL", "http://shared-service:8080"),
		Logger:     app.Logger,
		M2M: &oauth.TokenSource{
			TokenURL:     getenv("OIDC_TOKEN_URL", "http://thunderid:8090/oauth2/token"),
			ClientID:     os.Getenv("M2M_CLIENT_ID"),
			ClientSecret: os.Getenv("M2M_CLIENT_SECRET"),
			Scope:        "loading:read-internal outlets:read-internal audit:write",
		},
	}
	objects := objectstore.Store(objectstore.S3{
		Endpoint:  os.Getenv("MINIO_ENDPOINT"),
		Bucket:    getenv("MINIO_BUCKET", "waypoint-proof"),
		AccessKey: getenv("MINIO_ACCESS_KEY", "s3mock"),
		SecretKey: getenv("MINIO_SECRET_KEY", "s3mock"),
		Region:    getenv("MINIO_REGION", "us-east-1"),
	})
	if os.Getenv("MINIO_ENDPOINT") == "" {
		objects = &objectstore.Memory{}
	}
	retentionEnabled, retentionDays, err := deliveryProofRetentionConfig(os.Getenv("DELIVERY_PROOF_RETENTION_ENABLED"), os.Getenv("DELIVERY_PROOF_RETENTION_DAYS"))
	if err != nil {
		log.Fatal(err)
	}
	if retentionEnabled {
		go (retention.Runner{Store: store.Postgres{Pool: pool}, Files: objects, Days: retentionDays, Log: app.Logger}).Run(context.Background())
		app.Logger.Warn("delivery_proof_retention_enabled", "retention_days", retentionDays, "policy_version", "delivery-proof-retention-v1")
	}
	h := handler.Handler{
		Authn:    app.Authenticator,
		Profiles: client.Profiles{Shared: peers},
		Service:  service.Service{Repo: store.Postgres{Pool: pool}, Peers: peers, Objects: objects},
	}
	if err := app.Run(func(r chi.Router) { h.Routes(r) }); err != nil {
		log.Fatal(err)
	}
}

func getenv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func deliveryProofRetentionConfig(enabledValue, daysValue string) (bool, int, error) {
	enabledValue = strings.ToLower(strings.TrimSpace(enabledValue))
	enabled := false
	if enabledValue != "" {
		if enabledValue != "true" && enabledValue != "false" {
			return false, 0, fmt.Errorf("DELIVERY_PROOF_RETENTION_ENABLED must be true or false")
		}
		enabled = enabledValue == "true"
	}
	if strings.TrimSpace(daysValue) == "" {
		daysValue = "180"
	}
	days, err := strconv.Atoi(strings.TrimSpace(daysValue))
	if err != nil || days < 1 || days > 3650 {
		return false, 0, fmt.Errorf("DELIVERY_PROOF_RETENTION_DAYS must be between 1 and 3650")
	}
	return enabled, days, nil
}
