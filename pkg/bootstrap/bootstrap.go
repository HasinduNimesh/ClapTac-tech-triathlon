package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/auth"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/config"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/httpx"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/logging"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/telemetry"
	"github.com/go-chi/chi/v5"
)

type App struct {
	Config        config.Config
	Logger        *slog.Logger
	Authenticator auth.Authenticator
}

func Setup(serviceName, schema string) (*App, error) {
	cfg := config.Load(serviceName, schema)
	if err := auth.ValidateDisabledGuard(cfg.Environment, cfg.AuthDisabled); err != nil {
		return nil, err
	}
	logger := logging.New(serviceName, cfg.LogLevel)
	var authenticator auth.Authenticator
	if cfg.AuthDisabled {
		logger.Warn("authentication_disabled", "environment", cfg.Environment)
		authenticator = auth.DisabledAuthenticator{}
	} else {
		authenticator = auth.NewJWTAuthenticator(cfg.OIDCIssuer, cfg.OIDCAudience, cfg.OIDCJWKSURL)
	}
	return &App{Config: cfg, Logger: logger, Authenticator: authenticator}, nil
}

func (a *App) Run(register func(r chi.Router)) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	shutdown, err := telemetry.Init(ctx, a.Config.ServiceName, a.Config.OTLPEndpoint)
	if err != nil {
		return fmt.Errorf("telemetry: %w", err)
	}
	defer func() { _ = shutdown(context.Background()) }()

	r := httpx.NewRouter(httpx.Options{Service: a.Config.ServiceName, Logger: a.Logger})
	register(r)
	return httpx.ListenAndServe(ctx, a.Config.HTTPAddr, r, a.Logger)
}
