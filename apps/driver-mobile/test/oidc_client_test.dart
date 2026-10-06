import 'package:flutter/services.dart';
import 'package:flutter_appauth/flutter_appauth.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:waypoint_driver/auth/auth_config.dart';
import 'package:waypoint_driver/auth/auth_failure.dart';
import 'package:waypoint_driver/auth/oidc_client.dart';

final _now = DateTime.utc(2026, 10, 3, 9);

class FakeAppAuth extends FlutterAppAuth {
  FakeAppAuth();

  AuthorizationTokenRequest? request;
  Object? error;
  AuthorizationTokenResponse? response = AuthorizationTokenResponse(
    'access-123',
    null,
    _now.add(const Duration(hours: 1)),
    'id-token',
    'Bearer',
    const ['openid', 'profile'],
    null,
    null,
  );

  @override
  Future<AuthorizationTokenResponse> authorizeAndExchangeCode(AuthorizationTokenRequest request) async {
    this.request = request;
    if (error != null) throw error!;
    return response!;
  }
}

const _local = AuthConfig(issuer: 'http://localhost:8090', apiBaseUrl: 'http://localhost:18081', releaseMode: false);
const _production = AuthConfig(
  issuer: 'https://id.waypoint.claptac.dev',
  apiBaseUrl: 'https://waypoint.claptac.dev',
  resource: 'https://waypoint.claptac.dev/api/v1',
  releaseMode: true,
);

FlutterAppAuthPlatformException _serverError(String? oauthError) => FlutterAppAuthPlatformException(
      code: 'token_failed',
      message: 'Token request failed',
      platformErrorDetails: FlutterAppAuthPlatformErrorDetails(error: oauthError),
    );

void main() {
  test('asks the identity provider for a PKCE sign-in with a fresh login for this app', () async {
    final appAuth = FakeAppAuth();
    final tokens = await AppAuthOidcClient(_local, appAuth: appAuth, clock: () => _now).signIn();

    final request = appAuth.request!;
    expect(request.clientId, 'waypoint-driver');
    expect(request.redirectUrl, 'dev.claptac.waypointdriver:/oauth2redirect');
    expect(request.discoveryUrl, 'http://localhost:8090/.well-known/openid-configuration');
    expect(request.scopes, ['openid', 'profile']);
    expect(request.promptValues, ['login']);
    expect(request.additionalParameters, isNull);
    expect(tokens?.accessToken, 'access-123');
    expect(tokens?.expiresAt, _now.add(const Duration(hours: 1)));
  });

  test('plain HTTP is allowed only for a local identity server in a debug build', () async {
    final local = FakeAppAuth();
    await AppAuthOidcClient(_local, appAuth: local).signIn();
    expect(local.request!.allowInsecureConnections, isTrue);

    final production = FakeAppAuth();
    await AppAuthOidcClient(_production, appAuth: production).signIn();
    expect(production.request!.allowInsecureConnections, isFalse);
  });

  test('the production API audience is requested as a resource', () async {
    final appAuth = FakeAppAuth();
    await AppAuthOidcClient(_production, appAuth: appAuth).signIn();
    expect(appAuth.request!.additionalParameters, {'resource': 'https://waypoint.claptac.dev/api/v1'});
    expect(appAuth.request!.discoveryUrl, 'https://id.waypoint.claptac.dev/.well-known/openid-configuration');
  });

  test('backing out of the browser returns null instead of an error', () async {
    final appAuth = FakeAppAuth()
      ..error = FlutterAppAuthUserCancelledException(
        code: 'authorize_and_exchange_code_failed',
        platformErrorDetails: FlutterAppAuthPlatformErrorDetails(),
      );
    expect(await AppAuthOidcClient(_local, appAuth: appAuth).signIn(), isNull);
  });

  test('a server that refuses the sign-in is reported as not accepted', () async {
    for (final oauthError in ['invalid_grant', 'access_denied', 'invalid_client', 'unauthorized_client', 'INVALID_GRANT', 'invalid_request', 'invalid_scope', 'unsupported_grant_type', 'invalid_token', 'something_new']) {
      final appAuth = FakeAppAuth()..error = _serverError(oauthError);
      await expectLater(
        AppAuthOidcClient(_local, appAuth: appAuth).signIn(),
        throwsA(isA<AuthFailure>().having((f) => f.kind, 'kind', AuthFailureKind.unauthorized)),
        reason: oauthError,
      );
    }
  });

  test('other platform failures are reported as unreachable without exposing details', () async {
    for (final error in [_serverError(null), _serverError(''), _serverError('server_error'), _serverError('Temporarily_Unavailable'), PlatformException(code: 'boom', message: 'secret internals')]) {
      final appAuth = FakeAppAuth()..error = error;
      await expectLater(
        AppAuthOidcClient(_local, appAuth: appAuth).signIn(),
        throwsA(isA<AuthFailure>().having((f) => f.kind, 'kind', AuthFailureKind.unavailable)),
      );
    }
  });

  test('a response without an access token is treated as a failed sign-in', () async {
    final appAuth = FakeAppAuth()..response = AuthorizationTokenResponse(null, null, null, 'id-token', null, null, null, null);
    await expectLater(
      AppAuthOidcClient(_local, appAuth: appAuth).signIn(),
      throwsA(isA<AuthFailure>().having((f) => f.kind, 'kind', AuthFailureKind.unavailable)),
    );
  });

  test('a missing expiry falls back to a short lifetime rather than a long-lived token', () async {
    final appAuth = FakeAppAuth()..response = AuthorizationTokenResponse('access-123', null, null, null, null, null, null, null);
    final tokens = await AppAuthOidcClient(_local, appAuth: appAuth, clock: () => _now).signIn();
    expect(tokens!.expiresAt, _now.add(const Duration(minutes: 30)));
  });
}
