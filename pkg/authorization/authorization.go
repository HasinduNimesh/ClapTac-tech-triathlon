package authorization

import (
	"context"
	"net/http"
	"strings"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/auth"
	apierrors "github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/errors"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/telemetry"
)

type contextKey string

const profileKey contextKey = "waypoint.profile"

// Profile is application-owned identity mapping. Outlet membership and
// driver assignments are resolved here, not from JWT claims.
type Profile struct {
	UserID      string   `json:"userId"`
	Subject     string   `json:"subject"`
	DisplayName string   `json:"displayName"`
	Roles       []string `json:"roles"`
	OutletIDs   []string `json:"outletIds,omitempty"`
	DriverID    string   `json:"driverId,omitempty"`
	Depot       string   `json:"depot,omitempty"`
	VehicleID   string   `json:"vehicleId,omitempty"`
}

type ProfileResolver interface {
	Resolve(ctx context.Context, subject string) (*Profile, error)
}

type StaticProfileResolver struct {
	Profiles map[string]Profile
}

func (s StaticProfileResolver) Resolve(_ context.Context, subject string) (*Profile, error) {
	if s.Profiles != nil {
		if p, ok := s.Profiles[subject]; ok {
			cp := p
			return &cp, nil
		}
	}
	return &Profile{Subject: subject}, nil
}

type ResourceRef struct {
	Type      string
	ID        string
	OutletID  string
	DriverID  string
	TripID    string
	Depot     string
	VehicleID string
}

type ResourceAuthorizer interface {
	Authorize(ctx context.Context, principal *auth.Principal, profile *Profile, resource ResourceRef) error
}

type BusinessRuleValidator interface {
	Validate(ctx context.Context, principal *auth.Principal, profile *Profile, resource ResourceRef) error
}

type AllowResourceAuthorizer struct{}

func (AllowResourceAuthorizer) Authorize(context.Context, *auth.Principal, *Profile, ResourceRef) error {
	return nil
}

type AllowBusinessRules struct{}

func (AllowBusinessRules) Validate(context.Context, *auth.Principal, *Profile, ResourceRef) error {
	return nil
}

type Middleware struct {
	Authenticator   auth.Authenticator
	Profiles        ProfileResolver
	Resources       ResourceAuthorizer
	Rules           BusinessRuleValidator
	Permission      string
	AnyPermission   []string
	ResourceFromReq func(*http.Request) ResourceRef
}

func (m Middleware) Handler(next http.Handler) http.Handler {
	if m.Profiles == nil {
		m.Profiles = StaticProfileResolver{}
	}
	if m.Resources == nil {
		m.Resources = AllowResourceAuthorizer{}
	}
	if m.Rules == nil {
		m.Rules = AllowBusinessRules{}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, err := m.Authenticator.Authenticate(r)
		if err != nil {
			telemetry.AgentAuthorizationDenied.Inc()
			apierrors.Unauthorized(w, err.Error())
			return
		}
		needed := m.AnyPermission
		if m.Permission != "" {
			needed = append(needed, m.Permission)
		}
		ctx := auth.WithBearer(r.Context(), r.Header.Get("Authorization"))
		profile, err := m.Profiles.Resolve(ctx, principal.Subject)
		if err != nil {
			apierrors.Forbidden(w, "profile lookup failed")
			return
		}
		// Application DB role is authoritative. JWT roles are not used for RBAC.
		if len(profile.Roles) == 0 && len(principal.Scopes) == 0 {
			telemetry.AgentAuthorizationDenied.Inc()
			apierrors.Forbidden(w, "no application role for subject")
			return
		}
		if len(needed) > 0 && !hasAnyPermission(profile.Roles, principal.Scopes, needed) {
			telemetry.AgentAuthorizationDenied.Inc()
			apierrors.Forbidden(w, "missing permission "+needed[0])
			return
		}
		resource := ResourceRef{}
		if m.ResourceFromReq != nil {
			resource = m.ResourceFromReq(r)
		}
		if err := m.Resources.Authorize(r.Context(), principal, profile, resource); err != nil {
			telemetry.AgentAuthorizationDenied.Inc()
			apierrors.Forbidden(w, err.Error())
			return
		}
		if err := m.Rules.Validate(r.Context(), principal, profile, resource); err != nil {
			apierrors.Forbidden(w, err.Error())
			return
		}
		ctx = auth.WithPrincipal(ctx, principal)
		ctx = context.WithValue(ctx, profileKey, profile)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func Require(authn auth.Authenticator, permission string) func(http.Handler) http.Handler {
	return Middleware{Authenticator: authn, Permission: permission}.Handler
}

func RequireAny(authn auth.Authenticator, permissions ...string) func(http.Handler) http.Handler {
	return Middleware{Authenticator: authn, AnyPermission: permissions}.Handler
}

func RequireWith(authn auth.Authenticator, profiles ProfileResolver, permission string) func(http.Handler) http.Handler {
	return Middleware{Authenticator: authn, Profiles: profiles, Permission: permission}.Handler
}

func RequireAnyWith(authn auth.Authenticator, profiles ProfileResolver, permissions ...string) func(http.Handler) http.Handler {
	return Middleware{Authenticator: authn, Profiles: profiles, AnyPermission: permissions}.Handler
}

func hasAnyPermission(roles, scopes, permissions []string) bool {
	for _, permission := range permissions {
		if HasPermission(roles, permission) || hasScope(scopes, permission) {
			return true
		}
	}
	return false
}

func ProfileFrom(ctx context.Context) (*Profile, bool) {
	p, ok := ctx.Value(profileKey).(*Profile)
	return p, ok
}

// WithProfile adds a profile resolved from the application identity store to a
// context. It is used by handlers that intentionally share middleware's RBAC
// identity without depending on the HTTP middleware itself.
func WithProfile(ctx context.Context, profile *Profile) context.Context {
	return context.WithValue(ctx, profileKey, profile)
}

func hasScope(scopes []string, permission string) bool {
	for _, s := range scopes {
		if s == permission {
			return true
		}
	}
	return false
}

// OutletScoped rejects store-manager access unless the resource outlet is
// in the resolved profile. Dispatchers are unrestricted at this layer.
type OutletScoped struct{}

func (OutletScoped) Authorize(_ context.Context, _ *auth.Principal, profile *Profile, resource ResourceRef) error {
	if profile == nil {
		return errForbidden("profile required")
	}
	if HasPermission(profile.Roles, PermOrderViewAll) {
		return nil
	}
	if resource.OutletID == "" {
		return nil
	}
	for _, id := range profile.OutletIDs {
		if id == resource.OutletID {
			return nil
		}
	}
	return errForbidden("store manager may only access their authorized outlet")
}

// DepotScoped rejects loader access unless the trip depot matches the profile.
// loading:view-all skips the check.
type DepotScoped struct{}

func (DepotScoped) Authorize(_ context.Context, _ *auth.Principal, profile *Profile, resource ResourceRef) error {
	if profile == nil {
		return errForbidden("profile required")
	}
	if HasPermission(profile.Roles, PermLoadingViewAll) {
		return nil
	}
	if resource.Depot == "" {
		return nil
	}
	if profile.Depot == "" || !strings.EqualFold(profile.Depot, resource.Depot) {
		return errForbidden("loader may only access trips for their assigned depot")
	}
	return nil
}

// DriverScoped rejects driver access unless the resource vehicle matches the profile.
// delivery:view-all skips the check.
type DriverScoped struct{}

func (DriverScoped) Authorize(_ context.Context, _ *auth.Principal, profile *Profile, resource ResourceRef) error {
	if profile == nil {
		return errForbidden("profile required")
	}
	if HasPermission(profile.Roles, PermDeliveryViewAll) {
		return nil
	}
	if resource.VehicleID == "" {
		return nil
	}
	if profile.VehicleID == "" || !strings.EqualFold(profile.VehicleID, resource.VehicleID) {
		return errForbidden("driver may only access trips for their assigned vehicle")
	}
	return nil
}

type forbid string

func (f forbid) Error() string { return string(f) }

func errForbidden(msg string) error { return forbid(msg) }
