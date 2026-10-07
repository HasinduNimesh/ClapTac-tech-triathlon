export type OidcConfig = {
  issuer: string;
  clientId: string;
  redirectUri: string;
  resource: string;
  /** What the sign-in asks for. Add offline_access (and allow it for this app on the identity server) to let it renew itself. */
  scope: string;
};

export function oidcConfig(): OidcConfig {
  return {
    issuer: import.meta.env.VITE_OIDC_ISSUER || "http://localhost:8090",
    clientId: import.meta.env.VITE_OIDC_CLIENT_ID ?? "waypoint-web",
    redirectUri: import.meta.env.VITE_OIDC_REDIRECT_URI ?? "http://localhost/auth/callback",
    resource: import.meta.env.VITE_OIDC_AUDIENCE ?? "waypoint-api",
    scope: (import.meta.env.VITE_OIDC_SCOPE as string | undefined)?.trim() || "openid profile",
  };
}
