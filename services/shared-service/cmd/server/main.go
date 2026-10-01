package main

import (
	"context"
	"log"
	"os"

	"github.com/go-chi/chi/v5"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/bootstrap"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/db"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/oauth"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/shared-service/internal/handler"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/shared-service/internal/notifyworker"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/shared-service/internal/store"
)

func main() {
	app, err := bootstrap.Setup("shared-service", "shared")
	if err != nil {
		log.Fatal(err)
	}
	pool, err := db.Open(context.Background(), app.Config.DatabaseURL, "shared")
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	sharedStore := store.Store{Pool: pool}
	tokens := &oauth.TokenSource{TokenURL: getenv("OIDC_TOKEN_URL", "http://thunderid:8090/oauth2/token"), ClientID: getenv("M2M_CLIENT_ID", "waypoint-shared-service"), ClientSecret: os.Getenv("M2M_CLIENT_SECRET"), Scope: "notifications:send"}
	workerCtx, cancelWorker := context.WithCancel(context.Background())
	defer cancelWorker()
	go (notifyworker.Runner{Store: sharedStore, Tokens: tokens, IntegrationURL: getenv("INTEGRATION_SERVICE_URL", "http://integration-service:8080"), Logger: app.Logger}).Run(workerCtx)
	h := handler.Handler{Authn: app.Authenticator, Store: sharedStore}
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
