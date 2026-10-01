package main

import (
	"context"
	"log"
	"os"

	"github.com/go-chi/chi/v5"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/bootstrap"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/db"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/oauth"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/order-service/internal/client"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/order-service/internal/cutoff"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/order-service/internal/handler"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/order-service/internal/service"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/order-service/internal/store"
)

func main() {
	app, err := bootstrap.Setup("order-service", "orders")
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()
	pool, err := db.Open(ctx, app.Config.DatabaseURL, "orders")
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	calPath := os.Getenv("CALENDAR_PATH")
	if calPath == "" {
		calPath = "/data/calendar.csv"
	}
	cal, err := cutoff.FromCSV(calPath)
	if err != nil {
		app.Logger.Warn("calendar_unavailable", "error", err)
		cal = cutoff.Load(nil)
	}

	shared := client.Shared{
		BaseURL: getenv("SHARED_SERVICE_URL", "http://shared-service:8080"),
		Logger:  app.Logger,
		M2M: &oauth.TokenSource{
			TokenURL:     getenv("OIDC_TOKEN_URL", "http://thunderid:8090/oauth2/token"),
			ClientID:     os.Getenv("M2M_CLIENT_ID"),
			ClientSecret: os.Getenv("M2M_CLIENT_SECRET"),
			Scope:        "plans:read-internal deliveries:read-internal outlets:read-internal audit:write policy:read-internal",
		},
	}
	h := handler.Handler{
		Authn:    app.Authenticator,
		Profiles: client.Resolver{Shared: shared},
		Service: service.Service{
			Repo:     store.Postgres{Pool: pool},
			Outlets:  shared,
			Audit:    shared,
			Cutoff:   cal,
			Policy:   shared,
			Calendar: shared,
			Planning: client.PlanningTrackingPeer{BaseURL: getenv("PLANNING_SERVICE_URL", "http://planning-service:8080"), M2M: shared.M2M},
			Delivery: client.DeliveryTrackingPeer{BaseURL: getenv("DELIVERY_SERVICE_URL", "http://delivery-service:8080"), M2M: shared.M2M},
		},
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
