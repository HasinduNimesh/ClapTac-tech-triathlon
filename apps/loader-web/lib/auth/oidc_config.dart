/// Where the app is served, the same base path the production build and NGINX use.
const loaderBasePath = '/loader-app/';

/// The path the identity server sends the browser back to after sign-in. It is
/// served by the app itself (NGINX falls back to index.html under /loader-app/).
const loaderCallbackPath = '/loader-app/auth/callback';

/// Which identity server and which registered client the app signs in with.
///
/// Sign-in is the standard OIDC authorization code flow with PKCE for a public web
/// client: the browser is sent to the identity server, the app never sees a password,
/// and the redirect URI is `<origin>/loader-app/auth/callback`. Local Compose and
/// production differ only in these build settings (`--dart-define`):
///
///  * `OIDC_ISSUER` — the identity server's origin (for example `https://id.waypoint.claptac.dev`;
///    `http://localhost:8090` for local Compose, which runs the dev identity server there).
///  * `OIDC_CLIENT_ID` — the public client registered for the loader (default `waypoint-loader`).
///  * `OIDC_SCOPES` — requested scopes; `offline_access` is what lets the session renew itself
///    instead of sending the loader back to sign in every time the access token expires.
class OidcConfig {
  const OidcConfig({required this.issuer, this.clientId = 'waypoint-loader', this.scopes = 'openid profile offline_access'});

  factory OidcConfig.fromEnvironment() => const OidcConfig(
        issuer: String.fromEnvironment('OIDC_ISSUER'),
        clientId: String.fromEnvironment('OIDC_CLIENT_ID', defaultValue: 'waypoint-loader'),
        scopes: String.fromEnvironment('OIDC_SCOPES', defaultValue: 'openid profile offline_access'),
      );

  final String issuer;
  final String clientId;
  final String scopes;

  bool get configured => issuer.trim().isNotEmpty;

  /// The issuer without a trailing slash, so endpoint paths join cleanly.
  String get issuerBase => issuer.trim().replaceFirst(RegExp(r'/+$'), '');

  String redirectUri(String origin) => '$origin$loaderCallbackPath';
  String postLogoutUri(String origin) => '$origin$loaderBasePath';
}
