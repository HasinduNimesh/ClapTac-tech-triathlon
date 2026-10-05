import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:waypoint_loader/api/api_client.dart';
import 'package:waypoint_loader/auth/auth.dart';
import 'package:waypoint_loader/auth/browser_stub.dart';
import 'package:waypoint_loader/auth/oidc_config.dart';
import 'package:waypoint_loader/auth/sign_in_screen.dart';
import 'package:waypoint_loader/theme/tokens.dart';

const issuer = 'https://id.example.test';
const config = OidcConfig(issuer: issuer);
const apiResource = 'https://app.example.test/api/v1';
const resourceConfig = OidcConfig(issuer: issuer, resource: apiResource);
const api = 'https://app.example.test/api/v1';

/// A stand-in identity server and API that records what the app sent it.
class FakeServer {
  final List<http.Request> requests = [];
  String role = 'LOADER';
  int profileStatus = 200;
  int tokenStatus = 200;
  bool offline = false;
  int refreshes = 0;
  Map<String, String> lastTokenForm = {};

  /// When set, access tokens are JWTs with this `aud` (a string or a list), as a real identity server issues.
  Object? audience;
  Object? renewedAudience;

  String accessToken(String name, Object? aud) {
    if (aud == null) return name;
    String part(Object o) => base64Url.encode(utf8.encode(jsonEncode(o))).replaceAll('=', '');
    return '${part({'alg': 'none'})}.${part({'aud': aud, 'jti': name})}.sig';
  }

  Map<String, dynamic> discovery = {
    'authorization_endpoint': '$issuer/oauth2/authorize',
    'token_endpoint': '$issuer/oauth2/token',
    'end_session_endpoint': '$issuer/oauth2/logout',
  };

  late final MockClient client = MockClient((req) async {
    requests.add(req);
    if (offline) throw http.ClientException('network down');
    final path = req.url.path;
    if (path == '/.well-known/openid-configuration') return http.Response(jsonEncode(discovery), 200);
    if (path == '/oauth2/token') {
      lastTokenForm = Uri.splitQueryString(req.body);
      if (tokenStatus != 200) return http.Response('{"error":"invalid_grant"}', tokenStatus);
      if (lastTokenForm['grant_type'] == 'refresh_token') {
        refreshes++;
        return http.Response(jsonEncode({'access_token': accessToken('access-renewed-$refreshes', renewedAudience ?? audience), 'refresh_token': 'refresh-rotated-$refreshes', 'expires_in': 300}), 200);
      }
      return http.Response(jsonEncode({'access_token': accessToken('access-1', audience), 'refresh_token': 'refresh-1', 'id_token': 'id-1', 'expires_in': 300}), 200);
    }
    if (path == '/api/v1/shared/profiles/me') {
      if (profileStatus != 200) return http.Response('{}', profileStatus);
      return http.Response(jsonEncode({'profile': {'userId': 'USR004', 'roles': [role], 'depot': 'DEPOT_NORTH'}}), 200);
    }
    return http.Response('not found', 404);
  });

  List<http.Request> tokenRequests() => requests.where((r) => r.url.path == '/oauth2/token').toList();
}

AuthService serviceFor(FakeServer server, MemoryBrowser browser, {DateTime Function()? now, OidcConfig? with_}) =>
    AuthService(config: with_ ?? config, client: server.client, browser: browser, apiBase: api, now: now, autoRenew: false);

MemoryBrowser browserAt(String url) => MemoryBrowser(location: Uri.parse(url), origin: 'https://app.example.test');

/// Starts a sign-in, then returns the callback URL the identity server would redirect back to.
Future<(AuthService, MemoryBrowser, FakeServer, Uri)> signedInto({String? returnedState, DateTime Function()? now, OidcConfig? with_, FakeServer? on}) async {
  final server = on ?? FakeServer();
  final browser = browserAt('https://app.example.test/loader-app/');
  final auth = serviceFor(server, browser, now: now, with_: with_);
  await auth.start();
  await auth.signIn();
  final sent = Uri.parse(browser.navigations.single);
  final state = returnedState ?? sent.queryParameters['state']!;
  final callback = Uri.parse('https://app.example.test$loaderCallbackPath?code=the-code&state=$state');
  return (auth, browser, server, callback);
}

void main() {
  group('handing over from the web app', () {
    const handoffKey = 'waypoint.loader.handoff';
    final clock = DateTime.utc(2026, 10, 5, 9);

    Future<(AuthService, MemoryBrowser)> startWith(Map<String, String> storage) async {
      final browser = browserAt('https://app.example.test/loader-app/');
      browser.storage.addAll(storage);
      final auth = serviceFor(FakeServer(), browser, now: () => clock);
      await auth.start();
      return (auth, browser);
    }

    test('a fresh hand-off goes straight to the identity server without prompt=login, once', () async {
      final (auth, browser) = await startWith({handoffKey: '${clock.millisecondsSinceEpoch - 3000}'});
      final url = Uri.parse(browser.navigations.single);
      expect('${url.scheme}://${url.host}${url.path}', '$issuer/oauth2/authorize');
      expect(url.queryParameters.containsKey('prompt'), isFalse, reason: 'the web app just signed this same person in, in this tab');
      expect(url.queryParameters['client_id'], 'waypoint-loader');
      expect(browser.storage.containsKey(handoffKey), isFalse, reason: 'single use');
      expect(auth.status, AuthStatus.signedOut);
    });

    test('an old hand-off is ignored, so the person sees the Sign in screen and a forced login', () async {
      final (auth, browser) = await startWith({handoffKey: '${clock.millisecondsSinceEpoch - const Duration(minutes: 10).inMilliseconds}'});
      expect(browser.navigations, isEmpty);
      expect(browser.storage.containsKey(handoffKey), isFalse);
      expect(auth.status, AuthStatus.signedOut);
      await auth.signIn();
      expect(Uri.parse(browser.navigations.single).queryParameters['prompt'], 'login');
    });

    test('a marker from the future or with junk in it is ignored', () async {
      for (final value in ['${clock.millisecondsSinceEpoch + 600000}', 'yes', '']) {
        final (_, browser) = await startWith({handoffKey: value});
        expect(browser.navigations, isEmpty, reason: 'value "$value"');
      }
    });

    test('opening the loader app with no hand-off never signs in by itself', () async {
      final (auth, browser) = await startWith({});
      expect(browser.navigations, isEmpty);
      expect(auth.status, AuthStatus.signedOut);
    });

    test('a signed-out message such as inactivity is shown instead of starting another sign-in', () async {
      final (auth, browser) = await startWith({
        handoffKey: '${clock.millisecondsSinceEpoch - 1000}',
        'waypoint.loader.signout.reason': 'inactivity',
      });
      expect(browser.navigations, isEmpty);
      expect(auth.error, isNotNull);
    });
  });

  group('carrying the web app\'s sign-in over (no second password)', () {
    const handoffKey = 'waypoint.loader.handoff';
    const webKey = 'oidc.user:$issuer:waypoint-web';
    var now = DateTime.utc(2026, 10, 5, 9);

    String webUser(String token, {Duration validFor = const Duration(hours: 1)}) =>
        jsonEncode({'access_token': token, 'expires_at': DateTime.utc(2026, 10, 5, 9).add(validFor).millisecondsSinceEpoch ~/ 1000, 'token_type': 'Bearer'});

    Future<(AuthService, MemoryBrowser, FakeServer)> start({
      Map<String, String> storage = const {},
      String? role,
      OidcConfig? with_,
    }) async {
      now = DateTime.utc(2026, 10, 5, 9);
      final server = FakeServer();
      if (role != null) server.role = role;
      final browser = browserAt('https://app.example.test/loader-app/');
      browser.storage.addAll(storage);
      final auth = serviceFor(server, browser, now: () => now, with_: with_);
      await auth.start();
      return (auth, browser, server);
    }

    test('a loader who has just signed in on the web app is signed in here without going to the identity server', () async {
      final (auth, browser, server) = await start(storage: {handoffKey: '${DateTime.utc(2026, 10, 5, 9).millisecondsSinceEpoch - 2000}', webKey: webUser('web-token')});
      expect(auth.status, AuthStatus.signedIn);
      expect(browser.navigations, isEmpty, reason: 'no second trip to the identity server, so no second password');
      expect(auth.session!.accessToken, 'web-token');
      expect(auth.session!.refreshToken, isNull);
      expect(auth.session!.profile.roles.map((r) => r.toUpperCase()), contains('LOADER'));
      expect(browser.storage.containsKey(handoffKey), isFalse, reason: 'single use');
      expect(server.requests.where((r) => r.url.path == '/oauth2/token'), isEmpty);
      final profile = server.requests.singleWhere((r) => r.url.path == '/api/v1/shared/profiles/me');
      expect(profile.headers['Authorization'], 'Bearer web-token');
    });

    test('the borrowed session ends when the web token runs out, because it cannot be renewed', () async {
      final (auth, _, _) = await start(storage: {handoffKey: '${DateTime.utc(2026, 10, 5, 9).millisecondsSinceEpoch - 2000}', webKey: webUser('web-token')});
      expect(auth.status, AuthStatus.signedIn);
      // The app starts renewing a minute before expiry; with no refresh token that is the end of the session.
      now = now.add(const Duration(minutes: 59, seconds: 30));
      await expectLater(auth.validToken(), throwsA(isA<ApiException>()));
      expect(auth.status, AuthStatus.signedOut);
    });

    test('a web token that is about to expire is not borrowed: the normal sign-in goes ahead', () async {
      final (auth, browser, _) = await start(storage: {
        handoffKey: '${DateTime.utc(2026, 10, 5, 9).millisecondsSinceEpoch - 2000}',
        webKey: webUser('web-token', validFor: const Duration(minutes: 2)),
      });
      expect(auth.status, AuthStatus.signedOut);
      expect(Uri.parse(browser.navigations.single).path, '/oauth2/authorize');
    });

    test('a web token for another API is not borrowed', () async {
      final server = FakeServer();
      final wrong = server.accessToken('web', 'https://other.example.test/api');
      final (auth, browser, _) = await start(
        storage: {handoffKey: '${DateTime.utc(2026, 10, 5, 9).millisecondsSinceEpoch - 2000}', webKey: webUser(wrong)},
        with_: resourceConfig,
      );
      expect(auth.status, AuthStatus.signedOut);
      expect(browser.navigations, hasLength(1), reason: 'falls back to the identity server, which reports the audience problem itself');
    });

    test('somebody who is not a loader is not signed in this way', () async {
      final (auth, browser, _) = await start(
        storage: {handoffKey: '${DateTime.utc(2026, 10, 5, 9).millisecondsSinceEpoch - 2000}', webKey: webUser('web-token')},
        role: 'DRIVER',
      );
      expect(auth.status, AuthStatus.signedOut);
      expect(auth.session, isNull);
      expect(browser.navigations, hasLength(1));
    });

    test('without the hand-off marker a web sign-in is never used: a shared tablet still asks the next person', () async {
      final (auth, browser, server) = await start(storage: {webKey: webUser('web-token')});
      expect(auth.status, AuthStatus.signedOut);
      expect(browser.navigations, isEmpty);
      expect(server.requests.where((r) => r.url.path == '/api/v1/shared/profiles/me'), isEmpty);
    });

    test('an old marker is not enough either', () async {
      final (auth, browser, _) = await start(storage: {
        handoffKey: '${DateTime.utc(2026, 10, 5, 9).millisecondsSinceEpoch - const Duration(minutes: 10).inMilliseconds}',
        webKey: webUser('web-token'),
      });
      expect(auth.status, AuthStatus.signedOut);
      expect(browser.navigations, isEmpty);
    });

    test('damaged web storage falls back to the normal sign-in', () async {
      for (final junk in ['not json', '[]', '{}', jsonEncode({'access_token': 'x'})]) {
        final (auth, browser, _) = await start(storage: {handoffKey: '${DateTime.utc(2026, 10, 5, 9).millisecondsSinceEpoch - 2000}', webKey: junk});
        expect(auth.status, AuthStatus.signedOut, reason: junk);
        expect(browser.navigations, hasLength(1), reason: junk);
      }
    });
  });

  group('starting a sign-in', () {
    test('goes to the identity server with a PKCE challenge, state, the callback under /loader-app/ and the registered client', () async {
      final server = FakeServer();
      final browser = browserAt('https://app.example.test/loader-app/');
      final auth = serviceFor(server, browser);
      await auth.start();
      expect(auth.status, AuthStatus.signedOut);

      await auth.signIn();
      final url = Uri.parse(browser.navigations.single);
      expect('${url.scheme}://${url.host}${url.path}', '$issuer/oauth2/authorize');
      final q = url.queryParameters;
      expect(q['response_type'], 'code');
      expect(q['client_id'], 'waypoint-loader');
      expect(q['redirect_uri'], 'https://app.example.test/loader-app/auth/callback');
      expect(q['scope'], contains('openid'));
      expect(q['scope'], contains('offline_access'));
      expect(q['code_challenge_method'], 'S256');
      expect(q['prompt'], 'login', reason: 'a shared tablet must ask the next person for their own credentials');
      // The challenge is the SHA-256 of a verifier that stays in this tab, never in the URL.
      final verifier = browser.storage['waypoint.loader.oidc.verifier']!;
      expect(q['code_challenge'], challengeFor(verifier));
      expect(url.toString(), isNot(contains(verifier)));
      expect(browser.storage['waypoint.loader.oidc.state'], q['state']);
    });

    test('uses the standard discovery document, and the standard paths when there is none', () async {
      final server = FakeServer()..discovery = {};
      final browser = browserAt('https://app.example.test/loader-app/');
      final auth = serviceFor(server, browser);
      await auth.signIn();
      expect(browser.navigations.single, startsWith('$issuer/oauth2/authorize?'));
    });

    test('a build without an identity server says so instead of guessing one', () async {
      final auth = AuthService(config: const OidcConfig(issuer: ''), browser: browserAt('https://app.example.test/loader-app/'), autoRenew: false);
      await auth.start();
      expect(auth.status, AuthStatus.signedOut);
      expect(auth.error?.kind, SignInError.notConfigured);
    });
  });

  group('coming back from the identity server', () {
    test('exchanges the code with the verifier, checks the loader role, stores the session and clears the code from the address bar', () async {
      final (_, browser, server, callback) = await signedInto();
      final verifier = browser.storage['waypoint.loader.oidc.verifier']!;
      browser.location = callback;

      final again = serviceFor(server, browser);
      await again.start();

      expect(again.status, AuthStatus.signedIn);
      expect(again.session!.accessToken, 'access-1');
      expect(again.session!.profile.userId, 'USR004');
      final form = server.lastTokenForm;
      expect(form['grant_type'], 'authorization_code');
      expect(form['code'], 'the-code');
      expect(form['code_verifier'], verifier);
      expect(form['redirect_uri'], 'https://app.example.test/loader-app/auth/callback');
      expect(form['client_id'], 'waypoint-loader');
      expect(browser.replacements, [loaderBasePath], reason: 'the code must not stay in the history');
      expect(browser.storage.containsKey('waypoint.loader.oidc.verifier'), isFalse, reason: 'single use');
      expect(browser.storage.containsKey('waypoint.loader.oidc.state'), isFalse);
    });

    test('a different state is refused without asking for tokens (forged or replayed callback)', () async {
      final (_, browser, server, callback) = await signedInto(returnedState: 'not-the-state');
      browser.location = callback;
      final again = serviceFor(server, browser);
      await again.start();
      expect(again.status, AuthStatus.signedOut);
      expect(again.error?.kind, SignInError.notVerified);
      expect(server.tokenRequests(), isEmpty);
    });

    test('a callback with no sign-in in progress in this tab is refused', () async {
      final server = FakeServer();
      final browser = browserAt('https://app.example.test$loaderCallbackPath?code=x&state=y');
      final auth = serviceFor(server, browser);
      await auth.start();
      expect(auth.error?.kind, SignInError.notVerified);
      expect(server.tokenRequests(), isEmpty);
    });

    test('an error from the identity server is shown, not swallowed', () async {
      final server = FakeServer();
      final browser = browserAt('https://app.example.test$loaderCallbackPath?error=access_denied&error_description=Account%20disabled');
      final auth = serviceFor(server, browser);
      await auth.start();
      expect(auth.error?.kind, SignInError.denied);
      expect(auth.error?.detail, 'Account disabled');
    });

    test('someone who is not a loader never gets a session', () async {
      final (_, browser, server, callback) = await signedInto();
      server.role = 'DISPATCHER';
      browser.location = callback;
      final again = serviceFor(server, browser);
      await again.start();
      expect(again.status, AuthStatus.signedOut);
      expect(again.error?.kind, SignInError.wrongRole);
      expect(again.session, isNull);
      expect(browser.storage.containsKey('waypoint.loader.session'), isFalse);
    });

    test('a person who is not provisioned in Waypoint gets a clear message', () async {
      final (_, browser, server, callback) = await signedInto();
      server.profileStatus = 404;
      browser.location = callback;
      final again = serviceFor(server, browser);
      await again.start();
      expect(again.error?.kind, SignInError.noProfile);
    });

    test('a refused code exchange signs nobody in', () async {
      final (_, browser, server, callback) = await signedInto();
      server.tokenStatus = 400;
      browser.location = callback;
      final again = serviceFor(server, browser);
      await again.start();
      expect(again.status, AuthStatus.signedOut);
      expect(again.error?.kind, SignInError.exchangeFailed);
    });
  });

  group('keeping the session alive', () {
    /// Signs in at [start]; the returned one-element list is the test's clock, move time by editing it.
    Future<(AuthService, MemoryBrowser, FakeServer, List<DateTime>)> signedInAt(DateTime start) async {
      final clock = <DateTime>[start];
      DateTime now() => clock.single;
      final (_, browser, server, callback) = await signedInto(now: now);
      browser.location = callback;
      final auth = serviceFor(server, browser, now: now);
      await auth.start();
      expect(auth.status, AuthStatus.signedIn);
      return (auth, browser, server, clock);
    }

    test('the token is used as it is until shortly before it expires, then renewed with the refresh token', () async {
      final t0 = DateTime(2026, 10, 4, 9);
      final (auth, _, server, clock) = await signedInAt(t0);
      expect(await auth.validToken(), 'access-1');
      expect(server.refreshes, 0);

      clock[0] = t0.add(const Duration(seconds: 250)); // 50 s left of 300
      expect(await auth.validToken(), 'access-renewed-1');
      expect(server.refreshes, 1);
      expect(server.lastTokenForm['grant_type'], 'refresh_token');
      expect(server.lastTokenForm['refresh_token'], 'refresh-1');
      expect(auth.session!.refreshToken, 'refresh-rotated-1', reason: 'a rotated refresh token replaces the old one');
      expect(auth.session!.profile.userId, 'USR004', reason: 'the profile is not fetched again');
    });

    test('several requests at the same moment cause one renewal', () async {
      final t0 = DateTime(2026, 10, 4, 9);
      final (auth, _, server, clock) = await signedInAt(t0);
      clock[0] = t0.add(const Duration(seconds: 280));
      final tokens = await Future.wait([auth.validToken(), auth.validToken(), auth.validToken()]);
      expect(tokens.toSet(), {'access-renewed-1'});
      expect(server.refreshes, 1);
    });

    test('a session the identity server will no longer renew signs the person out and says why', () async {
      final t0 = DateTime(2026, 10, 4, 9);
      final (auth, browser, server, clock) = await signedInAt(t0);
      clock[0] = t0.add(const Duration(seconds: 290));
      server.tokenStatus = 400;
      await expectLater(auth.validToken(), throwsA(isA<Object>()));
      expect(auth.status, AuthStatus.signedOut);
      expect(auth.error?.kind, SignInError.sessionExpired);
      expect(browser.storage.containsKey('waypoint.loader.session'), isFalse);
    });

    test('losing the connection during a renewal keeps the session so the next request can try again', () async {
      final t0 = DateTime(2026, 10, 4, 9);
      final (auth, _, server, clock) = await signedInAt(t0);
      clock[0] = t0.add(const Duration(seconds: 290));
      server.offline = true;
      await expectLater(auth.validToken(), throwsA(isA<Object>()));
      expect(auth.status, AuthStatus.signedIn);
      server.offline = false;
      expect(await auth.validToken(), 'access-renewed-1');
    });

    test('reloading the page restores the session of this tab, and renews it first when it has expired', () async {
      final t0 = DateTime(2026, 10, 4, 9);
      final (_, browser, server, clock) = await signedInAt(t0);
      final reload = AuthService(config: config, client: server.client, browser: browser, apiBase: api, now: () => clock.single, autoRenew: false);
      await reload.start();
      expect(reload.status, AuthStatus.signedIn);
      expect(reload.session!.accessToken, 'access-1');

      clock[0] = t0.add(const Duration(minutes: 10));
      final later = AuthService(config: config, client: server.client, browser: browser, apiBase: api, now: () => clock.single, autoRenew: false);
      await later.start();
      expect(later.status, AuthStatus.signedIn);
      expect(later.session!.accessToken, startsWith('access-renewed'));
    });
  });

  group('asking for a token for the Waypoint API (RFC 8707 resource)', () {
    test('sends the resource on the authorize request, the code exchange and every renewal', () async {
      final server = FakeServer()..audience = apiResource;
      final (_, browser, _, callback) = await signedInto(with_: resourceConfig, on: server);
      expect(Uri.parse(browser.navigations.single).queryParameters['resource'], apiResource);

      browser.location = callback;
      final clock = <DateTime>[DateTime(2026, 10, 4, 9)];
      final auth = AuthService(config: resourceConfig, client: server.client, browser: browser, apiBase: api, now: () => clock.single, autoRenew: false);
      await auth.start();
      expect(auth.status, AuthStatus.signedIn);
      expect(server.lastTokenForm['resource'], apiResource, reason: 'the code exchange asks for the same API');

      clock[0] = DateTime(2026, 10, 4, 9, 4, 50);
      await auth.validToken();
      expect(server.lastTokenForm['grant_type'], 'refresh_token');
      expect(server.lastTokenForm['resource'], apiResource, reason: 'so does a renewal');
    });

    test('sends nothing when no resource is configured (the local identity server)', () async {
      final (_, browser, server, callback) = await signedInto();
      expect(Uri.parse(browser.navigations.single).queryParameters.containsKey('resource'), isFalse);
      browser.location = callback;
      await serviceFor(server, browser).start();
      expect(server.lastTokenForm.containsKey('resource'), isFalse);
    });

    test('accepts a token whose audience is the API, alone or among others', () async {
      for (final aud in [apiResource, [apiResource, 'something-else']]) {
        final server = FakeServer()..audience = aud;
        final (_, browser, _, callback) = await signedInto(with_: resourceConfig, on: server);
        browser.location = callback;
        final auth = serviceFor(server, browser, with_: resourceConfig);
        await auth.start();
        expect(auth.status, AuthStatus.signedIn, reason: 'aud $aud');
      }
    });

    test('refuses a token for another API with the reason, instead of a 401 on every call later', () async {
      final server = FakeServer()..audience = 'waypoint-api';
      final (_, browser, _, callback) = await signedInto(with_: resourceConfig, on: server);
      browser.location = callback;
      final auth = serviceFor(server, browser, with_: resourceConfig);
      await auth.start();
      expect(auth.status, AuthStatus.signedOut);
      expect(auth.error?.kind, SignInError.wrongAudience);
      expect(auth.error?.detail, 'waypoint-api');
      expect(auth.session, isNull);
      expect(browser.storage.containsKey('waypoint.loader.session'), isFalse);
      expect(server.requests.where((r) => r.url.path == '/api/v1/shared/profiles/me'), isEmpty, reason: 'the API is never called with it');
    });

    test('a token with no audience at all is refused too', () async {
      final server = FakeServer()..audience = <String>[];
      final (_, browser, _, callback) = await signedInto(with_: resourceConfig, on: server);
      browser.location = callback;
      final auth = serviceFor(server, browser, with_: resourceConfig);
      await auth.start();
      expect(auth.error?.kind, SignInError.wrongAudience);
    });

    test('a renewal that returns a token for the wrong API signs the person out and says so', () async {
      final server = FakeServer()..audience = apiResource;
      final clock = <DateTime>[DateTime(2026, 10, 4, 9)];
      final (_, browser, _, callback) = await signedInto(with_: resourceConfig, on: server, now: () => clock.single);
      browser.location = callback;
      final auth = AuthService(config: resourceConfig, client: server.client, browser: browser, apiBase: api, now: () => clock.single, autoRenew: false);
      await auth.start();
      server.renewedAudience = 'waypoint-api';
      clock[0] = DateTime(2026, 10, 4, 9, 4, 50);
      await expectLater(auth.validToken(), throwsA(isA<Object>()));
      expect(auth.status, AuthStatus.signedOut);
      expect(auth.error?.kind, SignInError.wrongAudience);
    });

    test('a token that is not a readable JWT is left for the API to judge', () async {
      final server = FakeServer(); // opaque tokens
      final (_, browser, _, callback) = await signedInto(with_: resourceConfig, on: server);
      browser.location = callback;
      final auth = serviceFor(server, browser, with_: resourceConfig);
      await auth.start();
      expect(auth.status, AuthStatus.signedIn);
    });

    test('reads the audience of a JWT', () {
      String jwt(Object payload) => 'x.${base64Url.encode(utf8.encode(jsonEncode(payload))).replaceAll('=', '')}.y';
      expect(audiencesOf(jwt({'aud': 'a'})), ['a']);
      expect(audiencesOf(jwt({'aud': ['a', 'b']})), ['a', 'b']);
      expect(audiencesOf(jwt({'sub': 'x'})), isEmpty);
      expect(audiencesOf('not-a-jwt'), isNull);
    });
  });

  group('signing out', () {
    test('clears the session and ends it at the identity server, so the next person is not signed in by it', () async {
      final (_, browser, server, callback) = await signedInto();
      browser.location = callback;
      final auth = serviceFor(server, browser);
      await auth.start();
      browser.navigations.clear();

      await auth.signOut();
      expect(auth.status, AuthStatus.signedOut);
      expect(auth.session, isNull);
      expect(browser.storage.containsKey('waypoint.loader.session'), isFalse);
      final end = Uri.parse(browser.navigations.single);
      expect(end.path, '/oauth2/logout');
      expect(end.queryParameters['id_token_hint'], 'id-1');
      expect(end.queryParameters['post_logout_redirect_uri'], 'https://app.example.test/loader-app/');
      expect(end.queryParameters['client_id'], 'waypoint-loader');
    });

    test('without an end-session endpoint it still signs out locally', () async {
      final (_, browser, server, callback) = await signedInto();
      server.discovery = {'authorization_endpoint': '$issuer/oauth2/authorize', 'token_endpoint': '$issuer/oauth2/token'};
      browser.location = callback;
      final auth = serviceFor(server, browser);
      await auth.start();
      browser.navigations.clear();
      await auth.signOut();
      expect(auth.status, AuthStatus.signedOut);
      expect(browser.navigations, isEmpty);
    });
  });

  testWidgets('the sign-in screen explains the app is online-only and sends the browser to sign in', (tester) async {
    final server = FakeServer();
    final browser = browserAt('https://app.example.test/loader-app/');
    final auth = serviceFor(server, browser);
    await auth.start();
    await tester.pumpWidget(MaterialApp(theme: waypointTheme(), home: SignInScreen(auth: auth)));
    expect(find.textContaining('works online only'), findsOneWidget);
    expect(find.byType(TextField), findsNothing, reason: 'the password is typed on the identity server, never in this app');
    await tester.tap(find.text('Sign in').last);
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 10));
    expect(browser.navigations, hasLength(1));
  });

  testWidgets('an unconfigured build cannot start a sign-in and says what is missing', (tester) async {
    final auth = AuthService(config: const OidcConfig(issuer: ''), browser: browserAt('https://app.example.test/loader-app/'), autoRenew: false);
    await auth.start();
    await tester.pumpWidget(MaterialApp(theme: waypointTheme(), home: SignInScreen(auth: auth)));
    expect(find.text('Sign-in is not set up'), findsOneWidget);
  });
}
