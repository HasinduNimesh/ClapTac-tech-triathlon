import 'dart:convert';
import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:waypoint_driver/auth/auth_config.dart';
import 'package:waypoint_driver/auth/auth_failure.dart';
import 'package:waypoint_driver/auth/auth_gateway.dart';
import 'package:waypoint_driver/auth/auth_store.dart';
import 'package:waypoint_driver/auth/oidc_client.dart';
import 'package:waypoint_driver/auth/oidc_tokens.dart';
import 'package:waypoint_driver/auth/profile_api.dart';

final _now = DateTime.utc(2026, 10, 3, 9);

class FakeOidcClient implements OidcClient {
  FakeOidcClient(this.result);
  OidcTokens? result;
  Object? error;
  int calls = 0;

  @override
  Future<OidcTokens?> signIn() async {
    calls++;
    if (error != null) throw error!;
    return result;
  }

  @override
  Future<OidcTokens> refresh(OidcTokens current) async => throw UnimplementedError('refresh is covered in token_refresh_test.dart');
}

OidcTokens _tokens({Duration life = const Duration(hours: 1)}) => OidcTokens(accessToken: 'token-abc', expiresAt: _now.add(life));

// The shape shared-service really returns: the profile is wrapped in a "profile" object.
String _profileJson({List<String> roles = const ['DRIVER']}) => jsonEncode({
      'profile': {
        'userId': 'USR006',
        'subject': 'usr-driver',
        'roles': roles,
        'vehicleId': 'VEH001',
      },
    });

const _config = AuthConfig(issuer: 'https://id.example.com', apiBaseUrl: 'https://api.example.com');

({OidcAuthGateway gateway, FakeOidcClient client, MemoryAuthStore store, List<http.Request> requests}) _build({
  OidcTokens? tokens,
  MockClientHandler? handler,
  AuthConfig config = _config,
}) {
  final requests = <http.Request>[];
  final client = FakeOidcClient(tokens ?? _tokens());
  final store = MemoryAuthStore();
  final mock = MockClient((request) async {
    requests.add(request);
    return (handler ?? (_) async => http.Response(_profileJson(), 200))(request);
  });
  return (
    gateway: OidcAuthGateway(
      config: config,
      client: client,
      profiles: ProfileApi(client: mock, baseUrl: config.apiBaseUrl),
      store: store,
      clock: () => _now,
    ),
    client: client,
    store: store,
    requests: requests,
  );
}

void main() {
  group('sign in', () {
    test('a driver with a valid account is signed in and remembered', () async {
      final h = _build();
      final outcome = await h.gateway.signIn();
      expect(outcome.profile?.userId, 'USR006');
      expect(outcome.profile?.vehicleId, 'VEH001');
      expect((await h.store.read())?.tokens.accessToken, 'token-abc');
    });

    test('the profile is requested with the bearer token from the identity provider', () async {
      final h = _build();
      await h.gateway.signIn();
      expect(h.requests.single.url.toString(), 'https://api.example.com/api/v1/shared/profiles/me');
      expect(h.requests.single.headers['Authorization'], 'Bearer token-abc');
    });

    test('backing out of the browser is not an error and stores nothing', () async {
      final h = _build();
      h.client.result = null;
      final outcome = await h.gateway.signIn();
      expect(outcome.failure?.kind, AuthFailureKind.cancelled);
      expect(outcome.failure?.message, isNull);
      expect(await h.store.read(), isNull);
      expect(h.requests, isEmpty);
    });

    test('a rejected token asks the driver to try again', () async {
      final h = _build(handler: (_) async => http.Response('', 401));
      final outcome = await h.gateway.signIn();
      expect(outcome.failure?.kind, AuthFailureKind.unauthorized);
      expect(await h.store.read(), isNull);
    });

    test('an account that exists in the identity provider but not in Waypoint is explained', () async {
      final h = _build(handler: (_) async => http.Response('', 404));
      final outcome = await h.gateway.signIn();
      expect(outcome.failure?.kind, AuthFailureKind.notProvisioned);
      expect(outcome.failure?.message, contains('not set up in Waypoint'));
    });

    test('someone who is not a driver is denied and nothing is kept', () async {
      for (final roles in [
        ['DISPATCHER'],
        ['STORE_MANAGER'],
        <String>[],
      ]) {
        final h = _build(handler: (_) async => http.Response(_profileJson(roles: roles), 200));
        final outcome = await h.gateway.signIn();
        expect(outcome.failure?.kind, AuthFailureKind.accessDenied, reason: '$roles');
        expect(await h.store.read(), isNull);
      }
    });

    test('a network failure reaching the API is reported as unavailable', () async {
      final h = _build(handler: (_) async => throw const SocketException('offline'));
      final outcome = await h.gateway.signIn();
      expect(outcome.failure?.kind, AuthFailureKind.unavailable);
      expect(await h.store.read(), isNull);
    });

    test('a response without a profile object is treated as unavailable, not as a driver', () async {
      final h = _build(handler: (_) async => http.Response(jsonEncode({'userId': 'USR006', 'roles': ['DRIVER']}), 200));
      expect((await h.gateway.signIn()).failure?.kind, AuthFailureKind.unavailable);
    });

    test('a server error is reported as unavailable, not as a bad sign-in', () async {
      final h = _build(handler: (_) async => http.Response('boom', 503));
      expect((await h.gateway.signIn()).failure?.kind, AuthFailureKind.unavailable);
    });

    test('a failure inside the browser sign-in is surfaced', () async {
      final h = _build();
      h.client.error = const AuthFailure(AuthFailureKind.unavailable);
      expect((await h.gateway.signIn()).failure?.kind, AuthFailureKind.unavailable);
    });

    test('without configuration the identity provider is never contacted', () async {
      final h = _build(config: const AuthConfig(issuer: '', apiBaseUrl: ''));
      final outcome = await h.gateway.signIn();
      expect(outcome.failure?.kind, AuthFailureKind.notConfigured);
      expect(h.client.calls, 0);
    });
  });

  group('resuming and signing out', () {
    test('a valid previous sign-in resumes without any network call', () async {
      final h = _build();
      await h.gateway.signIn();
      h.requests.clear();
      final profile = await h.gateway.restore();
      expect(profile?.userId, 'USR006');
      expect(h.requests, isEmpty);
    });

    test('an expired sign-in is discarded', () async {
      final h = _build(tokens: _tokens(life: const Duration(seconds: -1)));
      await h.gateway.signIn();
      expect(await h.gateway.restore(), isNull);
      expect(await h.store.read(), isNull);
    });

    test('nothing stored means signed out', () async {
      expect(await _build().gateway.restore(), isNull);
    });

    test('signing out clears what is kept on the phone', () async {
      final h = _build();
      await h.gateway.signIn();
      await h.gateway.signOut();
      expect(await h.store.read(), isNull);
      expect(await h.gateway.restore(), isNull);
    });
  });

  group('configuration safety', () {
    test('plain HTTP is allowed for a local identity server in debug builds only', () {
      const local = AuthConfig(issuer: 'http://localhost:8090', apiBaseUrl: 'http://localhost:18081', releaseMode: false);
      expect(local.isConfigured, isTrue);
      expect(local.usesInsecureTransport, isTrue);
      const release = AuthConfig(issuer: 'http://localhost:8090', apiBaseUrl: 'http://localhost:18081', releaseMode: true);
      expect(release.isConfigured, isFalse);
    });

    test('HTTPS works in release builds', () {
      const release = AuthConfig(issuer: 'https://id.example.com', apiBaseUrl: 'https://api.example.com', releaseMode: true);
      expect(release.isConfigured, isTrue);
    });

    test('both URLs are required', () {
      expect(const AuthConfig(issuer: 'https://id.example.com', apiBaseUrl: '').isConfigured, isFalse);
      expect(const AuthConfig(issuer: '', apiBaseUrl: 'https://api.example.com').isConfigured, isFalse);
    });

    test('the discovery URL ignores trailing slashes', () {
      const config = AuthConfig(issuer: 'https://id.example.com/', apiBaseUrl: 'https://api.example.com//');
      expect(config.discoveryUrl, 'https://id.example.com/.well-known/openid-configuration');
      expect(config.apiRoot, 'https://api.example.com');
    });

    test('the default redirect scheme is a valid URL scheme', () {
      const config = AuthConfig(issuer: 'https://id.example.com', apiBaseUrl: 'https://api.example.com');
      final scheme = config.redirectUri.split(':').first;
      expect(RegExp(r'^[a-zA-Z][a-zA-Z0-9+.-]*$').hasMatch(scheme), isTrue, reason: scheme);
      expect(Uri.parse(config.redirectUri).scheme, scheme);
    });

    test('the resource is empty by default and the redirect uses the reviewed path', () {
      const config = AuthConfig(issuer: 'https://id.example.com', apiBaseUrl: 'https://api.example.com');
      expect(config.resource, isEmpty);
      expect(config.redirectUri, 'dev.claptac.waypointdriver:/oauth2redirect');
    });

    test('the default client and redirect match the Android manifest placeholder', () {
      const config = AuthConfig(issuer: 'https://id.example.com', apiBaseUrl: 'https://api.example.com');
      expect(config.clientId, 'waypoint-driver');
      expect(config.redirectUri, startsWith('dev.claptac.waypointdriver:'));
    });
  });

  group('tokens', () {
    test('expiry is judged against the supplied clock', () {
      final tokens = _tokens();
      expect(tokens.isExpired(_now), isFalse);
      expect(tokens.isExpired(_now.add(const Duration(hours: 2))), isTrue);
    });

    test('they survive a round trip through storage', () {
      final restored = OidcTokens.fromJson(_tokens().toJson());
      expect(restored.accessToken, 'token-abc');
      expect(restored.expiresAt, _tokens().expiresAt);
    });

    test('the subject is read from the token for bookkeeping, and bad tokens give null', () {
      final payload = base64Url.encode(utf8.encode(jsonEncode({'sub': 'usr-driver'}))).replaceAll('=', '');
      expect(OidcTokens.unverifiedSubject('a.$payload.c'), 'usr-driver');
      expect(OidcTokens.unverifiedSubject('not-a-jwt'), isNull);
      expect(OidcTokens.unverifiedSubject('a.%%%.c'), isNull);
    });
  });
}
