import 'package:flutter/services.dart';
import 'package:flutter_appauth/flutter_appauth.dart';

import 'auth_config.dart';
import 'auth_failure.dart';
import 'oidc_tokens.dart';

/// Runs the browser sign-in. Returns null if the person backed out.
abstract class OidcClient {
  Future<OidcTokens?> signIn();

  /// Trades the refresh token for a new access token without opening the browser. Throws
  /// [AuthFailure]: `unauthorized` when the provider refuses the refresh token (revoked, used,
  /// expired), so the driver has to sign in again; `unavailable` for anything else, such as no
  /// signal, where the refresh token is still good and the attempt should simply be repeated later.
  Future<OidcTokens> refresh(OidcTokens current);
}

/// Authorization Code + PKCE through the system browser (AppAuth). The app never sees the
/// username or password; it only receives the tokens.
class AppAuthOidcClient implements OidcClient {
  AppAuthOidcClient(this._config, {FlutterAppAuth? appAuth, DateTime Function()? clock})
      : _appAuth = appAuth ?? const FlutterAppAuth(),
        _clock = clock ?? DateTime.now;

  final AuthConfig _config;
  final FlutterAppAuth _appAuth;
  final DateTime Function() _clock;

  @override
  Future<OidcTokens?> signIn() async {
    try {
      final result = await _appAuth.authorizeAndExchangeCode(
        AuthorizationTokenRequest(
          _config.clientId,
          _config.redirectUri,
          discoveryUrl: _config.discoveryUrl,
          scopes: _config.scopes,
          // Always ask for credentials: a phone or tablet may be shared between drivers.
          promptValues: const ['login'],
          additionalParameters: _config.resource.isEmpty ? null : {'resource': _config.resource},
          allowInsecureConnections: !_config.releaseMode && _config.usesInsecureTransport,
        ),
      );
      final accessToken = result.accessToken;
      if (accessToken == null || accessToken.isEmpty) {
        throw const AuthFailure(AuthFailureKind.unavailable);
      }
      final expiresAt = result.accessTokenExpirationDateTime ?? _clock().add(const Duration(minutes: 30));
      return OidcTokens(accessToken: accessToken, expiresAt: expiresAt.toUtc(), refreshToken: result.refreshToken);
    } on FlutterAppAuthUserCancelledException {
      return null;
    } on FlutterAppAuthPlatformException catch (error) {
      throw _failure(error);
    } on PlatformException {
      throw const AuthFailure(AuthFailureKind.unavailable);
    }
  }

  @override
  Future<OidcTokens> refresh(OidcTokens current) async {
    final refreshToken = current.refreshToken;
    if (refreshToken == null || refreshToken.isEmpty) throw const AuthFailure(AuthFailureKind.unauthorized);
    try {
      final result = await _appAuth.token(
        TokenRequest(
          _config.clientId,
          _config.redirectUri,
          discoveryUrl: _config.discoveryUrl,
          refreshToken: refreshToken,
          grantType: 'refresh_token',
          scopes: _config.scopes,
          additionalParameters: _config.resource.isEmpty ? null : {'resource': _config.resource},
          allowInsecureConnections: !_config.releaseMode && _config.usesInsecureTransport,
        ),
      );
      final accessToken = result.accessToken;
      if (accessToken == null || accessToken.isEmpty) throw const AuthFailure(AuthFailureKind.unavailable);
      final expiresAt = result.accessTokenExpirationDateTime ?? _clock().add(const Duration(minutes: 30));
      return OidcTokens(
        accessToken: accessToken,
        expiresAt: expiresAt.toUtc(),
        // A provider that rotates refresh tokens returns a new one; if it does not, the old one stays valid.
        refreshToken: (result.refreshToken == null || result.refreshToken!.isEmpty) ? refreshToken : result.refreshToken,
      );
    } on FlutterAppAuthPlatformException catch (error) {
      throw _failure(error);
    } on PlatformException {
      throw const AuthFailure(AuthFailureKind.unavailable);
    }
  }

  /// The authorization server said no (bad or revoked grant, access denied): the sign-in or refresh
  /// was not accepted. Anything else (no network, unreadable discovery document, a failed browser
  /// round trip) is the provider being unreachable. Internals are never shown to the driver.
  static AuthFailure _failure(FlutterAppAuthPlatformException error) {
    const rejected = {'invalid_grant', 'access_denied', 'invalid_client', 'unauthorized_client'};
    if (rejected.contains(error.platformErrorDetails.error?.toLowerCase())) {
      return const AuthFailure(AuthFailureKind.unauthorized);
    }
    return const AuthFailure(AuthFailureKind.unavailable);
  }
}
