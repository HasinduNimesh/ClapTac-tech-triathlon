import 'package:flutter/foundation.dart';

/// Where to sign in and where the Waypoint API lives. Values come from `--dart-define`
/// so nothing environment-specific is baked into the source.
///
///   --dart-define=OIDC_ISSUER=https://id.example.com
///   --dart-define=API_BASE_URL=https://waypoint.example.com
///   --dart-define=OIDC_CLIENT_ID=waypoint-driver                          (default)
///   --dart-define=OIDC_REDIRECT_URI=dev.claptac.waypointdriver:/oauth2redirect   (default)
///   --dart-define=OIDC_RESOURCE=https://waypoint.claptac.dev/api/v1       (optional, see [resource])
///
/// The redirect scheme must also match `appAuthRedirectScheme` in android/app/build.gradle.kts.
/// URL schemes may only contain letters, digits, `+`, `-` and `.`: an underscore makes the
/// redirect an invalid URL, and the browser then treats it as a relative path.
class AuthConfig {
  const AuthConfig({
    required this.issuer,
    required this.apiBaseUrl,
    this.clientId = 'waypoint-driver',
    this.redirectUri = 'dev.claptac.waypointdriver:/oauth2redirect',
    this.resource = '',
    this.scopes = const ['openid', 'profile'],
    this.releaseMode = kReleaseMode,
  });

  factory AuthConfig.fromEnvironment() => const AuthConfig(
        issuer: String.fromEnvironment('OIDC_ISSUER'),
        apiBaseUrl: String.fromEnvironment('API_BASE_URL'),
        clientId: String.fromEnvironment('OIDC_CLIENT_ID', defaultValue: 'waypoint-driver'),
        redirectUri: String.fromEnvironment('OIDC_REDIRECT_URI', defaultValue: 'dev.claptac.waypointdriver:/oauth2redirect'),
        resource: String.fromEnvironment('OIDC_RESOURCE'),
      );

  final String issuer;
  final String apiBaseUrl;
  final String clientId;
  final String redirectUri;

  /// The API the access token is for (RFC 8707 `resource`), an absolute URI such as
  /// `https://waypoint.claptac.dev/api/v1`. ThunderID needs it to issue a token the Waypoint
  /// services accept. Empty means no `resource` parameter is sent (the local dev server).
  final String resource;
  final List<String> scopes;
  final bool releaseMode;

  /// Plain HTTP is only allowed in debug and profile builds, for a local identity server.
  bool get usesInsecureTransport => issuer.startsWith('http://') || apiBaseUrl.startsWith('http://');

  /// Sign-in is only offered when both URLs are set, and never over plain HTTP in a release build.
  bool get isConfigured {
    if (issuer.isEmpty || apiBaseUrl.isEmpty) return false;
    if (releaseMode && usesInsecureTransport) return false;
    return true;
  }

  String get discoveryUrl => '${issuer.replaceAll(RegExp(r'/+$'), '')}/.well-known/openid-configuration';

  String get apiRoot => apiBaseUrl.replaceAll(RegExp(r'/+$'), '');
}
