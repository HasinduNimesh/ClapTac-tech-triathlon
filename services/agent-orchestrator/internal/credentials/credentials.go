package credentials

import (
	"context"
	"errors"
	"net/http"
)

// Provider issues a delegated authenticated context for a target service.
// Milestone 1 may forward the inbound user token. An agent service
// credential is never a superuser grant.
type Provider interface {
	CredentialForService(ctx context.Context, target string) (string, error)
}

type inboundKey struct{}

func WithInboundToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, inboundKey{}, token)
}

func InboundToken(ctx context.Context) string {
	v, _ := ctx.Value(inboundKey{}).(string)
	return v
}

type InboundForwarder struct{}

func (InboundForwarder) CredentialForService(ctx context.Context, _ string) (string, error) {
	token := InboundToken(ctx)
	if token == "" {
		return "", errors.New("no inbound user credential to delegate")
	}
	return token, nil
}

func BearerFromRequest(r *http.Request) string {
	return r.Header.Get("Authorization")
}
