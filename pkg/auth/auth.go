package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

type contextKey string

const principalKey contextKey = "waypoint.principal"
const bearerKey contextKey = "waypoint.bearer"

// Principal is the identity extracted from a ThunderID access token.
// Tokens carry subject, roles, and scopes only. Outlet and trip
// assignments live in application data and are resolved later.
type Principal struct {
	Subject string
	Roles   []string
	Scopes  []string
}

type Authenticator interface {
	Authenticate(r *http.Request) (*Principal, error)
}

func ValidateDisabledGuard(environment string, authDisabled bool) error {
	if authDisabled && !strings.EqualFold(environment, "local") {
		return errors.New("AUTH_DISABLED is only allowed when ENVIRONMENT=local")
	}
	return nil
}

type DisabledAuthenticator struct {
	Default Principal
}

func (a DisabledAuthenticator) Authenticate(r *http.Request) (*Principal, error) {
	p := a.Default
	if p.Subject == "" {
		p.Subject = headerOr(r, "X-Waypoint-Subject", "local-user")
	}
	if roles := r.Header.Get("X-Waypoint-Roles"); roles != "" {
		p.Roles = splitCSV(roles)
	}
	if scopes := r.Header.Get("X-Waypoint-Scopes"); scopes != "" {
		p.Scopes = splitCSV(scopes)
	}
	if len(p.Roles) == 0 {
		p.Roles = []string{"DISPATCHER"}
	}
	return &p, nil
}

type JWTAuthenticator struct {
	Issuer   string
	Audience string
	JWKSURL  string
	KeyFunc  jwt.Keyfunc
}

func (a JWTAuthenticator) Authenticate(r *http.Request) (*Principal, error) {
	raw := bearerToken(r.Header.Get("Authorization"))
	if raw == "" {
		return nil, errors.New("missing bearer token")
	}
	keyFunc := a.KeyFunc
	if keyFunc == nil {
		keyFunc = func(token *jwt.Token) (any, error) {
			return nil, errors.New("JWKS keyfunc is not configured")
		}
	}
	claims := jwt.MapClaims{}
	opts := []jwt.ParserOption{jwt.WithValidMethods([]string{"RS256"}), jwt.WithExpirationRequired()}
	if a.Audience != "" {
		opts = append(opts, jwt.WithAudience(a.Audience))
	}
	if a.Issuer != "" {
		opts = append(opts, jwt.WithIssuer(a.Issuer))
	}
	token, err := jwt.ParseWithClaims(raw, claims, keyFunc, opts...)
	if err != nil || !token.Valid {
		return nil, fmt.Errorf("invalid access token: %w", err)
	}
	sub, _ := claims["sub"].(string)
	if sub == "" {
		return nil, errors.New("token missing sub")
	}
	return &Principal{
		Subject: sub,
		Roles:   claimStrings(claims, "roles", "role"),
		Scopes:  claimStrings(claims, "scope", "scp", "scopes"),
	}, nil
}

func WithPrincipal(ctx context.Context, p *Principal) context.Context {
	return context.WithValue(ctx, principalKey, p)
}

func PrincipalFrom(ctx context.Context) (*Principal, bool) {
	p, ok := ctx.Value(principalKey).(*Principal)
	return p, ok
}

func WithBearer(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, bearerKey, token)
}

func BearerFrom(ctx context.Context) string {
	v, _ := ctx.Value(bearerKey).(string)
	return v
}

func bearerToken(header string) string {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return ""
	}
	return strings.TrimSpace(header[len(prefix):])
}

func headerOr(r *http.Request, key, fallback string) string {
	if v := strings.TrimSpace(r.Header.Get(key)); v != "" {
		return v
	}
	return fallback
}

func splitCSV(v string) []string {
	parts := strings.FieldsFunc(v, func(r rune) bool {
		return r == ',' || r == ' '
	})
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func claimStrings(claims jwt.MapClaims, keys ...string) []string {
	for _, key := range keys {
		switch v := claims[key].(type) {
		case string:
			return splitCSV(v)
		case []any:
			out := make([]string, 0, len(v))
			for _, item := range v {
				if s, ok := item.(string); ok && s != "" {
					out = append(out, s)
				}
			}
			return out
		}
	}
	return nil
}
