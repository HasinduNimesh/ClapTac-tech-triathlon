import 'dart:async';
import 'dart:io';

import 'package:flutter/services.dart';
import 'package:flutter_appauth/flutter_appauth.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:sqflite_common_ffi/sqflite_ffi.dart';
import 'package:waypoint_driver/auth/auth_config.dart';
import 'package:waypoint_driver/auth/auth_failure.dart';
import 'package:waypoint_driver/auth/auth_gateway.dart';
import 'package:waypoint_driver/auth/auth_store.dart';
import 'package:waypoint_driver/auth/oidc_client.dart';
import 'package:waypoint_driver/auth/oidc_tokens.dart';
import 'package:waypoint_driver/auth/profile_api.dart';
import 'package:waypoint_driver/data/driver_models.dart';
import 'package:waypoint_driver/sync/operations.dart';
import 'package:waypoint_driver/sync/sqlite_sync_queue.dart';
import 'package:waypoint_driver/sync/sync_worker.dart';
import 'package:waypoint_driver/trips/trip_source.dart';
import 'package:waypoint_driver/trips/trip_start.dart';
import 'package:waypoint_driver/trips/trips_api.dart';

final _now = DateTime.utc(2026, 10, 4, 9);
const _driver = DriverProfile(userId: 'USR006', subject: 'usr-driver', roles: ['DRIVER'], vehicleId: 'VEH001');
const _config = AuthConfig(issuer: 'http://localhost:8090', apiBaseUrl: 'http://localhost:18080', releaseMode: false);

OidcTokens _tokens({String access = 'access-1', String? refresh = 'refresh-1', Duration life = const Duration(hours: 1)}) =>
    OidcTokens(accessToken: access, refreshToken: refresh, expiresAt: _now.add(life));

class _FakeAppAuth extends FlutterAppAuth {
  AuthorizationTokenResponse? signInResponse;
  TokenRequest? request;
  int calls = 0;
  Object? error;
  Future<void>? hold;
  TokenResponse response = TokenResponse('access-2', 'refresh-2', _now.add(const Duration(hours: 1)), null, 'Bearer', const ['openid'], null);

  @override
  Future<AuthorizationTokenResponse> authorizeAndExchangeCode(AuthorizationTokenRequest request) async => signInResponse!;

  @override
  Future<TokenResponse> token(TokenRequest request) async {
    calls++;
    this.request = request;
    await hold;
    if (error != null) throw error!;
    return response;
  }
}

FlutterAppAuthPlatformException _serverError(String oauthError) => FlutterAppAuthPlatformException(
      code: 'token_failed',
      message: 'Token request failed',
      platformErrorDetails: FlutterAppAuthPlatformErrorDetails(error: oauthError),
    );

class _FakeClient implements OidcClient {
  OidcTokens? next;
  Object? error;
  Future<void>? hold;
  int refreshes = 0;
  OidcTokens? lastRefreshed;

  @override
  Future<OidcTokens?> signIn() async => null;

  @override
  Future<OidcTokens> refresh(OidcTokens current) async {
    refreshes++;
    lastRefreshed = current;
    await hold;
    if (error != null) throw error!;
    return next ?? _tokens(access: 'access-2', refresh: 'refresh-2');
  }
}

({OidcAuthGateway gateway, _FakeClient client, MemoryAuthStore store}) _gateway({OidcTokens? stored}) {
  final client = _FakeClient();
  final store = MemoryAuthStore();
  if (stored != null) store.value = StoredAuth(tokens: stored, profile: _driver);
  final gateway = OidcAuthGateway(
    config: _config,
    client: client,
    profiles: ProfileApi(client: MockClient((_) async => http.Response('{}', 500)), baseUrl: 'http://localhost:18080'),
    store: store,
    clock: () => _now,
  );
  return (gateway: gateway, client: client, store: store);
}

void main() {
  sqfliteFfiInit();

  group('OidcTokens', () {
    test('keeps the refresh token through storage, and older stored sessions simply have none', () {
      final restored = OidcTokens.fromJson(_tokens().toJson());
      expect(restored.refreshToken, 'refresh-1');
      expect(restored.canRefresh, isTrue);
      final old = OidcTokens.fromJson({'accessToken': 'a', 'expiresAt': _now.millisecondsSinceEpoch});
      expect(old.refreshToken, isNull);
      expect(old.canRefresh, isFalse);
      expect(old.toJson().containsKey('refreshToken'), isFalse);
    });

    test('an empty refresh token is not one', () {
      expect(_tokens(refresh: '').canRefresh, isFalse);
    });

    test('isExpired can look ahead by a skew', () {
      final t = _tokens(life: const Duration(seconds: 30));
      expect(t.isExpired(_now), isFalse);
      expect(t.isExpired(_now, skew: const Duration(seconds: 60)), isTrue);
      expect(t.isExpired(_now.add(const Duration(seconds: 30))), isTrue, reason: 'expired exactly at its expiry time');
    });
  });

  group('AuthConfig scopes', () {
    test('default to openid and profile, and OIDC_SCOPES can add offline_access', () {
      expect(AuthConfig.parseScopes(''), ['openid', 'profile']);
      expect(AuthConfig.parseScopes('   '), ['openid', 'profile']);
      expect(AuthConfig.parseScopes('openid profile offline_access'), ['openid', 'profile', 'offline_access']);
      expect(AuthConfig.parseScopes('openid\n  offline_access'), ['openid', 'offline_access']);
    });
  });

  group('AppAuthOidcClient.signIn', () {
    test('keeps the refresh token the provider issued', () async {
      final appAuth = _FakeAppAuth()
        ..signInResponse = AuthorizationTokenResponse('access-1', 'refresh-1', _now.add(const Duration(hours: 1)), 'id', 'Bearer', const ['openid', 'offline_access'], null, null);
      final tokens = await AppAuthOidcClient(_config, appAuth: appAuth, clock: () => _now).signIn();
      expect(tokens?.refreshToken, 'refresh-1');
    });

    test('works with a provider that issues none, as before', () async {
      final appAuth = _FakeAppAuth()
        ..signInResponse = AuthorizationTokenResponse('access-1', null, _now.add(const Duration(hours: 1)), 'id', 'Bearer', const ['openid'], null, null);
      final tokens = await AppAuthOidcClient(_config, appAuth: appAuth, clock: () => _now).signIn();
      expect(tokens?.refreshToken, isNull);
      expect(tokens?.canRefresh, isFalse);
    });
  });

  group('AppAuthOidcClient.refresh', () {
    late _FakeAppAuth appAuth;
    late AppAuthOidcClient client;
    setUp(() {
      appAuth = _FakeAppAuth();
      client = AppAuthOidcClient(_config, appAuth: appAuth, clock: () => _now);
    });

    test('sends a refresh_token grant for this client and returns the rotated tokens', () async {
      final tokens = await client.refresh(_tokens());
      final request = appAuth.request!;
      expect(request.refreshToken, 'refresh-1');
      expect(request.grantType, 'refresh_token');
      expect(request.clientId, 'waypoint-driver');
      expect(request.discoveryUrl, 'http://localhost:8090/.well-known/openid-configuration');
      expect(tokens.accessToken, 'access-2');
      expect(tokens.refreshToken, 'refresh-2');
      expect(tokens.expiresAt, _now.add(const Duration(hours: 1)));
    });

    test('sends the API resource when one is configured', () async {
      const production = AuthConfig(issuer: 'https://id.example.com', apiBaseUrl: 'https://api.example.com', resource: 'https://api.example.com/api/v1', releaseMode: true);
      await AppAuthOidcClient(production, appAuth: appAuth, clock: () => _now).refresh(_tokens());
      expect(appAuth.request!.additionalParameters, {'resource': 'https://api.example.com/api/v1'});
      expect(appAuth.request!.allowInsecureConnections, isFalse);
    });

    test('keeps the old refresh token when the provider does not rotate it', () async {
      appAuth.response = TokenResponse('access-2', null, _now.add(const Duration(hours: 1)), null, 'Bearer', null, null);
      expect((await client.refresh(_tokens())).refreshToken, 'refresh-1');
    });

    test('a refresh token the provider refuses means signing in again', () async {
      appAuth.error = _serverError('invalid_grant');
      await expectLater(client.refresh(_tokens()), throwsA(isA<AuthFailure>().having((f) => f.kind, 'kind', AuthFailureKind.unauthorized)));
    });

    test('no connection is not a refusal', () async {
      appAuth.error = PlatformException(code: 'network', message: 'offline');
      await expectLater(client.refresh(_tokens()), throwsA(isA<AuthFailure>().having((f) => f.kind, 'kind', AuthFailureKind.unavailable)));
      appAuth.error = _serverError('temporarily_unavailable');
      await expectLater(client.refresh(_tokens()), throwsA(isA<AuthFailure>().having((f) => f.kind, 'kind', AuthFailureKind.unavailable)));
    });

    test('a response without an access token is treated as unreachable', () async {
      appAuth.response = TokenResponse(null, 'r', null, null, null, null, null);
      await expectLater(client.refresh(_tokens()), throwsA(isA<AuthFailure>().having((f) => f.kind, 'kind', AuthFailureKind.unavailable)));
    });

    test('without a refresh token it does not call the provider', () async {
      await expectLater(client.refresh(_tokens(refresh: null)), throwsA(isA<AuthFailure>().having((f) => f.kind, 'kind', AuthFailureKind.unauthorized)));
      expect(appAuth.calls, 0);
    });
  });

  group('OidcAuthGateway.accessToken', () {
    test('a token with time left is used as it is', () async {
      final h = _gateway(stored: _tokens());
      expect(await h.gateway.accessToken(), 'access-1');
      expect(h.client.refreshes, 0);
    });

    test('a token about to expire is refreshed first', () async {
      final h = _gateway(stored: _tokens(life: const Duration(seconds: 30)));
      expect(await h.gateway.accessToken(), 'access-2');
      expect(h.client.refreshes, 1);
    });

    test('an expired token is refreshed, and the new tokens and the same profile are stored', () async {
      final h = _gateway(stored: _tokens(life: const Duration(minutes: -5)));
      expect(await h.gateway.accessToken(), 'access-2');
      final stored = (await h.store.read())!;
      expect(stored.tokens.accessToken, 'access-2');
      expect(stored.tokens.refreshToken, 'refresh-2');
      expect(stored.profile.userId, 'USR006');
      expect(h.client.lastRefreshed?.refreshToken, 'refresh-1');
    });

    test('an expired token with no refresh token means signing in again', () async {
      final h = _gateway(stored: _tokens(refresh: null, life: const Duration(minutes: -5)));
      expect(await h.gateway.accessToken(), isNull);
      expect(h.client.refreshes, 0);
    });

    test('a refresh token the provider refuses ends the session', () async {
      final h = _gateway(stored: _tokens(life: const Duration(minutes: -5)));
      h.client.error = const AuthFailure(AuthFailureKind.unauthorized);
      expect(await h.gateway.accessToken(), isNull);
      expect(await h.store.read(), isNull, reason: 'a refused refresh token is not kept');
    });

    test('no connection throws unavailable and leaves the session untouched', () async {
      final stored = _tokens(life: const Duration(minutes: -5));
      final h = _gateway(stored: stored);
      h.client.error = const AuthFailure(AuthFailureKind.unavailable);
      await expectLater(h.gateway.accessToken(), throwsA(isA<AuthFailure>().having((f) => f.kind, 'kind', AuthFailureKind.unavailable)));
      expect((await h.store.read())!.tokens.refreshToken, 'refresh-1');
      // Back online: the same refresh token works.
      h.client.error = null;
      expect(await h.gateway.accessToken(), 'access-2');
    });

    test('callers at the same time share one refresh', () async {
      final h = _gateway(stored: _tokens(life: const Duration(minutes: -5)));
      final release = Completer<void>();
      h.client.hold = release.future;
      final results = Future.wait([h.gateway.accessToken(), h.gateway.accessToken(), h.gateway.accessToken()]);
      await Future<void>.delayed(const Duration(milliseconds: 20));
      release.complete();
      expect(await results, ['access-2', 'access-2', 'access-2']);
      expect(h.client.refreshes, 1, reason: 'a rotating provider would refuse a second use of the same refresh token');
    });

    test('with no session there is nothing to return', () async {
      expect(await _gateway().gateway.accessToken(), isNull);
    });
  });

  group('OidcAuthGateway.restore', () {
    test('an expired access token with a refresh token is still a session, so the app opens offline', () async {
      final h = _gateway(stored: _tokens(life: const Duration(minutes: -5)));
      expect((await h.gateway.restore())?.userId, 'USR006');
      expect(await h.store.read(), isNotNull);
      expect(h.client.refreshes, 0, reason: 'opening the app does not need the network');
    });

    test('an expired access token with no refresh token is discarded', () async {
      final h = _gateway(stored: _tokens(refresh: null, life: const Duration(minutes: -5)));
      expect(await h.gateway.restore(), isNull);
      expect(await h.store.read(), isNull);
    });
  });

  group('callers treat a failed refresh as a connection problem, not a sign-out', () {
    final offlineAuth = _OfflineAuth();

    test('loading the route reports it and does not say the sign-in expired', () async {
      final load = await ApiTripSource(api: TripsApi(client: MockClient((_) async => http.Response('{}', 200)), baseUrl: 'http://api'), auth: offlineAuth).loadToday();
      expect(load.trip, isNull);
      expect(load.signInExpired, isFalse);
      expect(load.failure, contains('Could not reach Waypoint'));
    });

    test('starting the trip waits to retry', () async {
      const trip = TripInfo(tripId: 't', runId: 'r', vehicleCode: 'V', tripRef: 'P', depot: 'D', window: '', stops: [], planId: 'p', planVersion: 1);
      final result = await ApiTripStarter(client: MockClient((_) async => http.Response('{}', 200)), baseUrl: 'http://api', auth: offlineAuth).start(trip, operationId: 'op');
      expect(result.status, TripStartStatus.offline);
    });

    test('the sync worker reports offline and keeps the queue', () async {
      final directory = await Directory.systemTemp.createTemp('waypoint-refresh-test-');
      addTearDown(() => directory.delete(recursive: true));
      final queue = SqliteSyncQueue(open: () => SqliteSyncQueue.openAt('${directory.path}/q.db', factory: databaseFactoryFfi));
      await queue.useOwner('USR006');
      await queue.enqueue(Operations.incident(
        operationId: 'inc',
        trip: const TripInfo(tripId: 't', runId: 'r', vehicleCode: 'V', tripRef: 'P', depot: 'D', window: '', stops: []),
        category: 'ROAD',
        description: 'Road closed',
        occurredAt: _now,
      ).toSyncEvent());
      var requests = 0;
      final worker = DeliverySyncWorker(
        queue: queue,
        auth: offlineAuth,
        client: MockClient((_) async {
          requests++;
          return http.Response('{}', 200);
        }),
        baseUrl: 'http://api',
        interval: const Duration(hours: 1),
      );
      final offline = Completer<void>();
      worker.onProgress = (progress, _) {
        if (progress == SyncProgress.offline && !offline.isCompleted) offline.complete();
      };
      worker.start();
      await offline.future.timeout(const Duration(seconds: 5));
      worker.stop();
      expect(requests, 0);
      expect((await queue.first())!.state, 'pending');
    });
  });
}

class _OfflineAuth implements AuthGateway {
  @override
  Future<String?> accessToken() async => throw const AuthFailure(AuthFailureKind.unavailable);
  @override
  Future<DriverProfile?> restore() async => null;
  @override
  Future<AuthOutcome> signIn() async => const AuthOutcome.failed(AuthFailure(AuthFailureKind.cancelled));
  @override
  Future<void> signOut() async {}
}
