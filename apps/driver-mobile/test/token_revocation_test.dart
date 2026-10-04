import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:waypoint_driver/app/driver_session.dart';
import 'package:waypoint_driver/auth/auth_config.dart';
import 'package:waypoint_driver/auth/auth_gateway.dart';
import 'package:waypoint_driver/auth/auth_store.dart';
import 'package:waypoint_driver/auth/oidc_client.dart';
import 'package:waypoint_driver/auth/oidc_tokens.dart';
import 'package:waypoint_driver/auth/profile_api.dart';
import 'package:waypoint_driver/auth/revocation_queue.dart';
import 'package:waypoint_driver/auth/token_revoker.dart';
import 'package:waypoint_driver/connectivity/connectivity_monitor.dart';
import 'package:waypoint_driver/offline/local_database.dart';
import 'package:waypoint_driver/sync/sync.dart';

final _now = DateTime.utc(2026, 10, 4, 9);
const _driver = DriverProfile(userId: 'USR006', subject: 'usr-driver', roles: ['DRIVER'], vehicleId: 'VEH001');
const _dispatcher = DriverProfile(userId: 'USR002', subject: 'usr-dispatcher', roles: ['DISPATCHER']);
const _issuer = 'https://id.example.com';
const _config = AuthConfig(issuer: _issuer, apiBaseUrl: 'https://api.example.com', releaseMode: true);
const _local = AuthConfig(issuer: 'http://localhost:8090', apiBaseUrl: 'http://localhost:18080', releaseMode: false);

OidcTokens _tokens({String? refresh = 'refresh-1'}) => OidcTokens(accessToken: 'access-1', refreshToken: refresh, expiresAt: _now.add(const Duration(hours: 1)));

String _discovery({String? revocation = 'https://id.example.com/oauth2/revoke'}) => jsonEncode({'issuer': _issuer, if (revocation != null) 'revocation_endpoint': revocation});

class _Calls {
  final urls = <String>[];
  final forms = <Map<String, String>>[];
}

MockClient _server(_Calls calls, {String? discovery, int discoveryStatus = 200, int revokeStatus = 200, Object? revokeError}) {
  return MockClient((request) async {
    calls.urls.add('${request.method} ${request.url}');
    if (request.method == 'GET') return http.Response(discovery ?? _discovery(), discoveryStatus);
    calls.forms.add(request.bodyFields);
    if (revokeError != null) throw revokeError;
    return http.Response('', revokeStatus);
  });
}

class _Revoker implements TokenRevoker {
  _Revoker([this.outcome = RevokeOutcome.done]);
  RevokeOutcome outcome;
  Object? error;
  Future<void>? hold;
  final revoked = <String>[];
  @override
  Future<RevokeOutcome> revokeRefreshToken(String refreshToken) async {
    revoked.add(refreshToken);
    await hold;
    if (error != null) throw error!;
    return outcome;
  }
}

class _Client implements OidcClient {
  OidcTokens? signInResult;
  @override
  Future<OidcTokens?> signIn() async => signInResult;
  @override
  Future<OidcTokens> refresh(OidcTokens current) async => throw UnimplementedError();
}

/// A store that records what the pending list held at the moment the session was cleared.
class _ObservingStore extends MemoryAuthStore {
  _ObservingStore(this.pending);
  final RevocationQueue pending;
  List<String>? pendingWhenCleared;
  @override
  Future<void> clear() async {
    pendingWhenCleared = await pending.read();
    await super.clear();
  }
}

({OidcAuthGateway gateway, MemoryAuthStore store, MemoryRevocationQueue pending, _Revoker revoker, _Client client}) _gateway({OidcTokens? stored, _Revoker? revoker, bool withRevoker = true, DriverProfile profile = _driver, MemoryAuthStore? store}) {
  final theStore = store ?? MemoryAuthStore();
  if (stored != null) theStore.value = StoredAuth(tokens: stored, profile: _driver);
  final pending = MemoryRevocationQueue();
  final theRevoker = revoker ?? _Revoker();
  final client = _Client();
  final gateway = OidcAuthGateway(
    config: _config,
    client: client,
    profiles: ProfileApi(
      client: MockClient((_) async => http.Response(jsonEncode({'profile': {'userId': profile.userId, 'subject': profile.subject, 'roles': profile.roles}}), 200)),
      baseUrl: 'https://api.example.com',
    ),
    store: theStore,
    revoker: withRevoker ? theRevoker : null,
    pendingRevocations: withRevoker ? pending : null,
    clock: () => _now,
  );
  return (gateway: gateway, store: theStore, pending: pending, revoker: theRevoker, client: client);
}

Future<void> _settle() => Future<void>.delayed(const Duration(milliseconds: 30));

void main() {
  group('HttpTokenRevoker', () {
    test('asks the discovered endpoint to revoke the refresh token, as a public client', () async {
      final calls = _Calls();
      final outcome = await HttpTokenRevoker(client: _server(calls), config: _config).revokeRefreshToken('rt-123');
      expect(outcome, RevokeOutcome.done);
      expect(calls.urls, ['GET https://id.example.com/.well-known/openid-configuration', 'POST https://id.example.com/oauth2/revoke']);
      expect(calls.forms.single, {'token': 'rt-123', 'token_type_hint': 'refresh_token', 'client_id': 'waypoint-driver'});
    });

    test('the token is only sent to the revocation endpoint, never to the discovery document', () async {
      final seen = <String>[];
      final client = MockClient((request) async {
        seen.add('${request.method} ${request.url} ${request.body}');
        return request.method == 'GET' ? http.Response(_discovery(), 200) : http.Response('', 200);
      });
      await HttpTokenRevoker(client: client, config: _config).revokeRefreshToken('secret-token');
      expect(seen.where((line) => line.contains('secret-token')).length, 1);
      expect(seen.firstWhere((line) => line.contains('secret-token')), startsWith('POST https://id.example.com/oauth2/revoke'));
    });

    test('a provider with no revocation endpoint is not an error, and nothing is sent', () async {
      final calls = _Calls();
      final outcome = await HttpTokenRevoker(client: _server(calls, discovery: _discovery(revocation: null)), config: _config).revokeRefreshToken('rt');
      expect(outcome, RevokeOutcome.notSupported);
      expect(calls.forms, isEmpty);
    });

    test('an endpoint over plain HTTP is refused in a release build, so the token never travels in clear', () async {
      final calls = _Calls();
      final outcome = await HttpTokenRevoker(client: _server(calls, discovery: _discovery(revocation: 'http://id.example.com/oauth2/revoke')), config: _config).revokeRefreshToken('rt');
      expect(outcome, RevokeOutcome.notSupported);
      expect(calls.forms, isEmpty);
    });

    test('a local identity server over HTTP is allowed in a debug build', () async {
      final calls = _Calls();
      final client = _server(calls, discovery: jsonEncode({'revocation_endpoint': 'http://localhost:8090/oauth2/revoke'}));
      expect(await HttpTokenRevoker(client: client, config: _local).revokeRefreshToken('rt'), RevokeOutcome.done);
      expect(calls.urls.last, 'POST http://localhost:8090/oauth2/revoke');
    });

    test('server errors and a missing connection mean try again later', () async {
      for (final status in [500, 502, 503, 408, 429]) {
        expect(await HttpTokenRevoker(client: _server(_Calls(), revokeStatus: status), config: _config).revokeRefreshToken('rt'), RevokeOutcome.retryLater, reason: '$status');
      }
      expect(await HttpTokenRevoker(client: _server(_Calls(), revokeError: const SocketException('no signal')), config: _config).revokeRefreshToken('rt'), RevokeOutcome.retryLater);
      expect(await HttpTokenRevoker(client: _server(_Calls(), revokeError: http.ClientException('reset')), config: _config).revokeRefreshToken('rt'), RevokeOutcome.retryLater);
    });

    test('a slow provider times out and is retried later', () async {
      final client = MockClient((request) async {
        await Future<void>.delayed(const Duration(milliseconds: 200));
        return http.Response(_discovery(), 200);
      });
      final outcome = await HttpTokenRevoker(client: client, config: _config, timeout: const Duration(milliseconds: 40)).revokeRefreshToken('rt');
      expect(outcome, RevokeOutcome.retryLater);
    });

    test('a refusal from the provider is final', () async {
      for (final status in [400, 401, 403]) {
        expect(await HttpTokenRevoker(client: _server(_Calls(), revokeStatus: status), config: _config).revokeRefreshToken('rt'), RevokeOutcome.gaveUp, reason: '$status');
      }
    });

    test('trouble reading the discovery document', () async {
      expect(await HttpTokenRevoker(client: _server(_Calls(), discoveryStatus: 503), config: _config).revokeRefreshToken('rt'), RevokeOutcome.retryLater);
      expect(await HttpTokenRevoker(client: _server(_Calls(), discoveryStatus: 404), config: _config).revokeRefreshToken('rt'), RevokeOutcome.gaveUp);
      expect(await HttpTokenRevoker(client: _server(_Calls(), discovery: 'not json'), config: _config).revokeRefreshToken('rt'), RevokeOutcome.retryLater);
    });
  });

  group('SecureRevocationQueue', () {
    late SecureRevocationQueue queue;
    setUp(() {
      FlutterSecureStorage.setMockInitialValues({});
      queue = SecureRevocationQueue();
    });

    test('keeps a token once, and removes it', () async {
      await queue.add('a');
      await queue.add('a');
      await queue.add('b');
      expect(await queue.read(), ['a', 'b']);
      await queue.remove('a');
      expect(await queue.read(), ['b']);
      await queue.remove('missing');
      expect(await queue.read(), ['b']);
    });

    test('sign-outs close together do not overwrite each other', () async {
      await Future.wait([for (var i = 0; i < 6; i++) queue.add('token-$i')]);
      expect((await queue.read()).toSet(), {for (var i = 0; i < 6; i++) 'token-$i'});
    });

    test('keeps only the newest few, so a phone cannot hoard tokens', () async {
      for (var i = 0; i < SecureRevocationQueue.maxPending + 4; i++) {
        await queue.add('token-$i');
      }
      final kept = await queue.read();
      expect(kept, hasLength(SecureRevocationQueue.maxPending));
      expect(kept.first, 'token-4');
      expect(kept.last, 'token-${SecureRevocationQueue.maxPending + 3}');
    });

    test('an unreadable stored list is treated as empty instead of failing a sign-out', () async {
      FlutterSecureStorage.setMockInitialValues({'waypoint.auth.pending-revocations.v1': 'not json {{'});
      expect(await SecureRevocationQueue().read(), isEmpty);
      await SecureRevocationQueue().add('fresh');
      expect(await SecureRevocationQueue().read(), ['fresh']);
    });

    test('removing the last token leaves nothing stored', () async {
      await queue.add('only');
      await queue.remove('only');
      expect(await queue.read(), isEmpty);
    });
  });

  group('signing out', () {
    test('signs out at once and revokes the refresh token', () async {
      final h = _gateway(stored: _tokens());
      await h.gateway.signOut();
      expect(await h.store.read(), isNull);
      await _settle();
      expect(h.revoker.revoked, ['refresh-1']);
      expect(await h.pending.read(), isEmpty, reason: 'revoked, so nothing is left to retry');
    });

    test('the token is on the pending list before the session is cleared', () async {
      final pending = MemoryRevocationQueue();
      final store = _ObservingStore(pending);
      store.value = StoredAuth(tokens: _tokens(), profile: _driver);
      final gateway = OidcAuthGateway(
        config: _config,
        client: _Client(),
        profiles: ProfileApi(client: MockClient((_) async => http.Response('{}', 500)), baseUrl: 'https://api.example.com'),
        store: store,
        revoker: _Revoker(RevokeOutcome.retryLater),
        pendingRevocations: pending,
        clock: () => _now,
      );
      await gateway.signOut();
      expect(store.pendingWhenCleared, ['refresh-1'], reason: 'so a crash between the two cannot lose the revocation');
    });

    test('with no signal the driver is still signed out, and the token waits to be revoked', () async {
      final h = _gateway(stored: _tokens(), revoker: _Revoker(RevokeOutcome.retryLater));
      await h.gateway.signOut();
      expect(await h.store.read(), isNull);
      await _settle();
      expect(await h.pending.read(), ['refresh-1']);
    });

    test('sign-out does not wait for the provider', () async {
      final revoker = _Revoker()..hold = Completer<void>().future;
      final h = _gateway(stored: _tokens(), revoker: revoker);
      await h.gateway.signOut().timeout(const Duration(seconds: 1));
      expect(await h.store.read(), isNull);
    });

    test('a revoker that throws cannot break sign-out', () async {
      final revoker = _Revoker()..error = StateError('boom');
      final h = _gateway(stored: _tokens(), revoker: revoker);
      await h.gateway.signOut();
      await _settle();
      expect(await h.store.read(), isNull);
      expect(await h.pending.read(), ['refresh-1'], reason: 'left for the next try');
    });

    test('a refusal or an unsupported provider drops the token instead of keeping it for ever', () async {
      for (final outcome in [RevokeOutcome.gaveUp, RevokeOutcome.notSupported]) {
        final h = _gateway(stored: _tokens(), revoker: _Revoker(outcome));
        await h.gateway.signOut();
        await _settle();
        expect(await h.pending.read(), isEmpty, reason: '$outcome');
      }
    });

    test('with no refresh token nothing is sent', () async {
      final h = _gateway(stored: _tokens(refresh: null));
      await h.gateway.signOut();
      await _settle();
      expect(h.revoker.revoked, isEmpty);
      expect(await h.pending.read(), isEmpty);
      expect(await h.store.read(), isNull);
    });

    test('signing out with nobody signed in is harmless', () async {
      final h = _gateway();
      await h.gateway.signOut();
      expect(h.revoker.revoked, isEmpty);
    });

    test('a gateway without a revoker signs out exactly as before', () async {
      final h = _gateway(stored: _tokens(), withRevoker: false);
      await h.gateway.signOut();
      expect(await h.store.read(), isNull);
      expect(h.revoker.revoked, isEmpty);
    });
  });

  group('retrying later', () {
    test('tokens still waiting are revoked on the next try, and only then removed', () async {
      final revoker = _Revoker(RevokeOutcome.retryLater);
      final h = _gateway(stored: _tokens(), revoker: revoker);
      await h.gateway.signOut();
      await _settle();
      expect(await h.pending.read(), ['refresh-1']);
      await h.gateway.retryPendingRevocations();
      expect(await h.pending.read(), ['refresh-1'], reason: 'still no luck');
      revoker.outcome = RevokeOutcome.done;
      await h.gateway.retryPendingRevocations();
      expect(await h.pending.read(), isEmpty);
      expect(revoker.revoked, ['refresh-1', 'refresh-1', 'refresh-1']);
    });

    test('each waiting token is tried, and one failing does not stop the others', () async {
      final h = _gateway(revoker: _Revoker(RevokeOutcome.retryLater));
      h.pending.tokens.addAll(['a', 'b', 'c']);
      await h.gateway.retryPendingRevocations();
      expect(h.revoker.revoked, ['a', 'b', 'c']);
      h.revoker.outcome = RevokeOutcome.done;
      await h.gateway.retryPendingRevocations();
      expect(h.pending.tokens, isEmpty);
    });

    test('two tries at once do not send the same token twice', () async {
      final hold = Completer<void>();
      final revoker = _Revoker()..hold = hold.future;
      final h = _gateway(revoker: revoker);
      h.pending.tokens.add('a');
      final first = h.gateway.retryPendingRevocations();
      final second = h.gateway.retryPendingRevocations();
      await _settle();
      hold.complete();
      await Future.wait([first, second]);
      expect(revoker.revoked, ['a']);
    });

    test('with nothing waiting it does nothing', () async {
      final h = _gateway();
      await h.gateway.retryPendingRevocations();
      expect(h.revoker.revoked, isEmpty);
    });
  });

  group('an account that is not a driver', () {
    test('its tokens are not kept, and its refresh token is revoked', () async {
      final h = _gateway(profile: _dispatcher);
      h.client.signInResult = _tokens(refresh: 'dispatcher-refresh');
      final outcome = await h.gateway.signIn();
      expect(outcome.profile, isNull);
      expect(await h.store.read(), isNull);
      await _settle();
      expect(h.revoker.revoked, ['dispatcher-refresh']);
    });
  });

  group('the session retries revocations', () {
    DriverSession session(_RetryGateway gateway, {ConnectivityMonitor? monitor}) {
      final s = DriverSession(database: InMemoryLocalDatabase(), queue: InMemorySyncQueue(), auth: gateway, connectivity: monitor);
      addTearDown(s.dispose);
      return s;
    }

    test('when the app starts, even with nobody signed in', () async {
      final gateway = _RetryGateway(profile: null);
      await session(gateway).restore();
      expect(gateway.retries, 1);
    });

    test('when the connection returns, even after signing out', () async {
      final monitor = _Monitor();
      final gateway = _RetryGateway(profile: null);
      session(gateway, monitor: monitor);
      await _settle();
      monitor.set(false);
      await _settle();
      monitor.set(true);
      await _settle();
      expect(gateway.retries, 1);
    });

    test('a gateway that cannot revoke is simply left alone', () async {
      final s = DriverSession(database: InMemoryLocalDatabase(), queue: InMemorySyncQueue(), auth: _PlainGateway());
      addTearDown(s.dispose);
      await s.restore();
      expect(s.signedIn, isFalse);
    });
  });
}

class _Monitor implements ConnectivityMonitor {
  final _controller = StreamController<bool>.broadcast();
  @override
  Future<bool> isOnline() async => true;
  @override
  Stream<bool> get changes => _controller.stream;
  void set(bool value) => _controller.add(value);
}

class _RetryGateway implements AuthGateway, RevocationRetry {
  _RetryGateway({required this.profile});
  final DriverProfile? profile;
  int retries = 0;
  @override
  Future<void> retryPendingRevocations() async => retries++;
  @override
  Future<AuthOutcome> signIn() async => const AuthOutcome.signedIn(_driver);
  @override
  Future<DriverProfile?> restore() async => profile;
  @override
  Future<void> signOut() async {}
  @override
  Future<String?> accessToken() async => 'tok';
}

class _PlainGateway implements AuthGateway {
  @override
  Future<AuthOutcome> signIn() async => const AuthOutcome.signedIn(_driver);
  @override
  Future<DriverProfile?> restore() async => null;
  @override
  Future<void> signOut() async {}
  @override
  Future<String?> accessToken() async => 'tok';
}
