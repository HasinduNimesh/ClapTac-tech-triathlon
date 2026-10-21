package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/auth"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/authorization"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/cache"
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
	// Cache is never nil. It is disabled (and harmless) when REDIS_URL is empty, and fails open when Redis is down.
	Cache *cache.Cache
}

func Setup(serviceName, schema string) (*App, error) {
	cfg := config.Load(serviceName, schema)
	if err := auth.ValidateDisabledGuard(cfg.Environment, cfg.AuthDisabled); err != nil {
		return nil, err
	}
	logger := logging.New(serviceName, cfg.LogLevel)
	// Code that logs through the package-level slog functions then writes the same JSON lines, tagged with the service.
	slog.SetDefault(logger)
	var authenticator auth.Authenticator
	if cfg.AuthDisabled {
		logger.Warn("authentication_disabled", "environment", cfg.Environment)
		authenticator = auth.DisabledAuthenticator{}
	} else {
		authenticator = auth.NewJWTAuthenticator(cfg.OIDCIssuer, cfg.OIDCAudience, cfg.OIDCJWKSURL)
	}
	return &App{Config: cfg, Logger: logger, Authenticator: authenticator, Cache: cache.New(cfg.RedisURL, logger)}, nil
}

func (a *App) Run(register func(r chi.Router)) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	shutdown, err := telemetry.Init(ctx, a.Config.ServiceName, a.Config.OTLPEndpoint)
	if err != nil {
		return fmt.Errorf("telemetry: %w", err)
	}
	defer func() { _ = shutdown(context.Background()) }()

	defer a.Cache.Close()

	opts := httpx.Options{Service: a.Config.ServiceName, Logger: a.Logger}
	if a.Cache.Enabled() {
		opts.RateLimit = &httpx.RateLimit{
			Limiter:      a.Cache,
			Authn:        a.Authenticator,
			PerUser:      envInt("RATE_LIMIT_USER_PER_MIN", 1200),
			PerAnonymous: envInt("RATE_LIMIT_ANON_PER_MIN", 300),
		}
	}
	r := httpx.NewRouter(opts)
	register(r)
	return httpx.ListenAndServe(ctx, a.Config.HTTPAddr, r, a.Logger)
}

func envInt(key string, fallback int) int {
	if n, err := strconv.Atoi(os.Getenv(key)); err == nil && n > 0 {
		return n
	}
	return fallback
}

// CachedProfiles wraps a service's profile lookup with the shared Redis cache. PROFILE_CACHE_TTL_SECONDS sets how
// long a profile is remembered (default 30; 0 turns it off). Without Redis it returns the lookup unchanged.
func (a *App) CachedProfiles(inner authorization.ProfileResolver) authorization.ProfileResolver {
	ttl := 30
	if v, err := strconv.Atoi(os.Getenv("PROFILE_CACHE_TTL_SECONDS")); err == nil && v >= 0 {
		ttl = v
	}
	if !a.Cache.Enabled() || ttl == 0 {
		return inner
	}
	return authorization.CachedResolver{Inner: inner, Cache: a.Cache, TTL: time.Duration(ttl) * time.Second}
}
