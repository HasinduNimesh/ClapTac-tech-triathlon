package client

import (
	"context"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/auth"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/authorization"
)

type Resolver struct {
	Shared Shared
}

func (r Resolver) Resolve(ctx context.Context, subject string) (*authorization.Profile, error) {
	profile, err := r.Shared.ResolveProfile(subject, auth.BearerFrom(ctx))
	if err != nil {
		// M2M subjects are not in shared.users; empty roles + JWT scopes authorize internals.
		return &authorization.Profile{Subject: subject}, nil
	}
	return profile, nil
}
