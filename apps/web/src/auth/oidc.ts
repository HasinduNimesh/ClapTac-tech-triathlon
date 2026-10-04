export type OidcConfig = {
  issuer: string;
  clientId: string;
  redirectUri: string;
  resource: string;
};

export function oidcConfig(): OidcConfig {
  return {
    issuer: import.meta.env.VITE_OIDC_ISSUER || "http://localhost:8090",
    clientId: import.meta.env.VITE_OIDC_CLIENT_ID ?? "waypoint-web",
    redirectUri: import.meta.env.VITE_OIDC_REDIRECT_URI ?? "http://localhost/auth/callback",
    resource: import.meta.env.VITE_OIDC_AUDIENCE ?? "waypoint-api",
  };
}
