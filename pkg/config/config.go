package config

import (
	"fmt"
	"os"
	"strings"
)

type Config struct {
	ServiceName    string
	Environment    string
	HTTPAddr       string
	AuthDisabled   bool
	OIDCIssuer     string
	OIDCJWKSURL    string
	OIDCAudience   string
	DatabaseURL    string
	DatabaseSchema string
	RedisURL       string
	OTLPEndpoint   string
	LogLevel       string
}

func Load(serviceName, defaultSchema string) Config {
	return Config{
		ServiceName:    serviceName,
		Environment:    env("ENVIRONMENT", "local"),
		HTTPAddr:       env("HTTP_ADDR", ":8080"),
		AuthDisabled:   envBool("AUTH_DISABLED", false),
		OIDCIssuer:     env("OIDC_ISSUER", ""),
		OIDCJWKSURL:    env("OIDC_JWKS_URL", ""),
		OIDCAudience:   env("OIDC_AUDIENCE", "waypoint-api"),
		DatabaseURL:    env("DATABASE_URL", ""),
		DatabaseSchema: env("DATABASE_SCHEMA", defaultSchema),
		RedisURL:       env("REDIS_URL", ""),
		OTLPEndpoint:   env("OTEL_EXPORTER_OTLP_ENDPOINT", ""),
		LogLevel:       env("LOG_LEVEL", "info"),
	}
}

func (c Config) IsLocal() bool {
	return strings.EqualFold(c.Environment, "local")
}

func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	switch strings.ToLower(v) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func (c Config) SchemaDSN() string {
	if c.DatabaseURL == "" || c.DatabaseSchema == "" {
		return c.DatabaseURL
	}
	sep := "?"
	if strings.Contains(c.DatabaseURL, "?") {
		sep = "&"
	}
	return fmt.Sprintf("%s%ssearch_path=%s", c.DatabaseURL, sep, c.DatabaseSchema)
}
