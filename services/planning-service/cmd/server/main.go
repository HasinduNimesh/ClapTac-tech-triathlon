package main

import (
	"context"
	"log"
	"os"

	"github.com/go-chi/chi/v5"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/bootstrap"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/db"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/oauth"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/client"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/handler"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/service"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/store"
)

func main() {
	app, err := bootstrap.Setup("planning-service", "planning")
	if err != nil {
		log.Fatal(err)
	}
	pool, err := db.Open(context.Background(), app.Config.DatabaseURL, "planning")
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	peers := client.Peers{
		OrdersURL:   getenv("ORDER_SERVICE_URL", "http://order-service:8080"),
		FleetURL:    getenv("FLEET_SERVICE_URL", "http://fleet-service:8080"),
		SharedURL:   getenv("SHARED_SERVICE_URL", "http://shared-service:8080"),
		DeliveryURL: getenv("DELIVERY_SERVICE_URL", "http://delivery-service:8080"),
		Logger:      app.Logger,
		M2M: &oauth.TokenSource{
			TokenURL:     getenv("OIDC_TOKEN_URL", "http://thunderid:8090/oauth2/token"),
			ClientID:     os.Getenv("M2M_CLIENT_ID"),
			ClientSecret: os.Getenv("M2M_CLIENT_SECRET"),
			Scope:        "orders:read-internal fleet:read-internal outlets:read-internal deliveries:read-internal audit:write policy:read-internal",
			Resource:     getenv("OIDC_AUDIENCE", "waypoint-api"),
		},
	}
	h := handler.Handler{
		Authn:    app.Authenticator,
		Profiles: client.Profiles{Shared: peers},
		Service:  service.Service{Repo: store.Postgres{Pool: pool}, Peers: peers},
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
