package agentctx

import (
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/auth"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/authorization"
)

type Assembly struct {
	Principal *auth.Principal
	Profile   *authorization.Profile
	Goal      string
}

func Assemble(principal *auth.Principal, profile *authorization.Profile, goal string) Assembly {
	return Assembly{Principal: principal, Profile: profile, Goal: goal}
}
