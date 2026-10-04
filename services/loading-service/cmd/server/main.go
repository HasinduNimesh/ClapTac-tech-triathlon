package main

import (
	"context"
	"log"
	"os"

	"github.com/go-chi/chi/v5"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/bootstrap"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/db"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/oauth"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/objectstore"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/loading-service/internal/client"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/loading-service/internal/handler"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/loading-service/internal/service"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/loading-service/internal/store"
)

func main() {
	app, err := bootstrap.Setup("loading-service", "loading")
	if err != nil {
		log.Fatal(err)
	}
	pool, err := db.Open(context.Background(), app.Config.DatabaseURL, "loading")
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	peers := client.Peers{
		PlanningURL: getenv("PLANNING_SERVICE_URL", "http://planning-service:8080"),
		OrdersURL:   getenv("ORDER_SERVICE_URL", "http://order-service:8080"),
		SharedURL:   getenv("SHARED_SERVICE_URL", "http://shared-service:8080"),
		Logger:      app.Logger,
		M2M: &oauth.TokenSource{
			TokenURL:     getenv("OIDC_TOKEN_URL", "http://thunderid:8090/oauth2/token"),
			ClientID:     os.Getenv("M2M_CLIENT_ID"),
			ClientSecret: os.Getenv("M2M_CLIENT_SECRET"),
			Scope:        "plans:read-internal orders:read-internal audit:write",
		},
	}
	// Shortfall photos share the proof object store; without MINIO_ENDPOINT
	// they are kept in memory (local development only).
	objects := objectstore.Reader(objectstore.S3{
		Endpoint:  os.Getenv("MINIO_ENDPOINT"),
		Bucket:    getenv("MINIO_BUCKET", "waypoint-proof"),
		AccessKey: getenv("MINIO_ACCESS_KEY", "s3mock"),
		SecretKey: getenv("MINIO_SECRET_KEY", "s3mock"),
		Region:    getenv("MINIO_REGION", "us-east-1"),
	})
	if os.Getenv("MINIO_ENDPOINT") == "" {
		if err := objectstore.RequireDurable(app.Config.IsLocal(), ""); err != nil {
			log.Fatal(err)
		}
		objects = &objectstore.Memory{}
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
