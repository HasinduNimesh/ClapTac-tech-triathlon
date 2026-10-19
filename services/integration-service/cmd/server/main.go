package main

import (
	"log"
	"os"

	"github.com/go-chi/chi/v5"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/bootstrap"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/oauth"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/integration-service/internal/handler"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/integration-service/internal/notify"
)

func main() {
	app, err := bootstrap.Setup("integration-service", "")
	if err != nil {
		log.Fatal(err)
	}
	h := handler.Handler{Authn: app.Authenticator, CallbackURL: os.Getenv("TWILIO_STATUS_CALLBACK_URL"), CallbackAuthToken: os.Getenv("TWILIO_AUTH_TOKEN"), StatusSink: notify.SharedStatusSink{SharedURL: getenv("SHARED_SERVICE_URL", "http://shared-service:8080"), Tokens: &oauth.TokenSource{TokenURL: getenv("OIDC_TOKEN_URL", "http://thunderid:8090/oauth2/token"), ClientID: getenv("M2M_CLIENT_ID", "waypoint-integration-service"), ClientSecret: os.Getenv("M2M_CLIENT_SECRET"), Scope: "notifications:write", Resource: getenv("OIDC_AUDIENCE", "waypoint-api")}}}
	// The SMS provider is our own cellular gateway when it is configured, otherwise Twilio (see ProviderFromEnv).
	h.GatewayWebhookSecret = os.Getenv("GATEWAY_WEBHOOK_SECRET")
	if sms, name, perr := notify.ProviderFromEnv(os.Getenv); perr == nil {
		h.SMS = sms
		app.Logger.Info("sms_provider", "provider", name)
	} else {
		app.Logger.Warn("sms_provider_unavailable", "provider", name, "reason", perr)
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
