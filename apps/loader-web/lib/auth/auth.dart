import 'dart:async';
import 'dart:convert';
import 'dart:math';

import 'package:crypto/crypto.dart';
import 'package:flutter/foundation.dart';
import 'package:http/http.dart' as http;

import '../api/api_client.dart';
import '../shared/models.dart';
import 'browser.dart';
import 'oidc_config.dart';

/// The signed-in loader. Kept in per-tab browser storage so a reload stays signed in;
/// closing the tab signs out, which suits a shared dock tablet. No loading data is stored.
class AuthSession {
  AuthSession({required this.accessToken, required this.profile, required this.expiresAt, this.refreshToken, this.idToken});

  final String accessToken;
  final String? refreshToken;
  final String? idToken;
  final Profile profile;
  final DateTime expiresAt;

  Map<String, dynamic> toJson() => {
        'accessToken': accessToken,
        'refreshToken': refreshToken,
        'idToken': idToken,
        'profile': profile.toJson(),
        'expiresAt': expiresAt.toIso8601String(),
      };

  factory AuthSession.fromJson(Map<String, dynamic> j) => AuthSession(
        accessToken: j['accessToken'] as String,
        refreshToken: j['refreshToken'] as String?,
        idToken: j['idToken'] as String?,
        profile: Profile.fromJson(j['profile'] as Map<String, dynamic>),
        expiresAt: DateTime.parse(j['expiresAt'] as String),
      );

  AuthSession renewed({required String accessToken, String? refreshToken, String? idToken, required DateTime expiresAt}) => AuthSession(
        accessToken: accessToken,
        refreshToken: refreshToken ?? this.refreshToken,
        idToken: idToken ?? this.idToken,
        profile: profile,
        expiresAt: expiresAt,
      );
}

enum SignInError { notConfigured, denied, notVerified, exchangeFailed, noConnection, noProfile, wrongRole, wrongAudience, sessionExpired }

class SignInException implements Exception {
  SignInException(this.kind, [this.detail = '']);
  final SignInError kind;
  final String detail;
  @override
  String toString() => 'SignInException($kind${detail.isEmpty ? '' : ': $detail'})';
}

enum AuthStatus { starting, signedOut, signedIn }

/// Standard OIDC authorization code + PKCE sign-in for the loader web app: the browser goes to
/// the identity server, comes back to `/loader-app/auth/callback?code=…`, and the app exchanges
/// the code for tokens. It renews the access token with the refresh token before it expires and
/// signs out (also at the identity server, when it offers an end-session endpoint).
class AuthService extends ChangeNotifier {
  AuthService({OidcConfig? config, http.Client? client, BrowserBridge? browser, DateTime Function()? now, bool autoRenew = true, String? apiBase})
      : config = config ?? OidcConfig.fromEnvironment(),
        _apiBase = apiBase,
        _client = client ?? http.Client(),
        browser = browser ?? createBrowser(),
        _now = now ?? DateTime.now,
        _autoRenew = autoRenew;

  final OidcConfig config;
  final BrowserBridge browser;
  final http.Client _client;
  final DateTime Function() _now;
  final bool _autoRenew;
  final String? _apiBase;

  AuthStatus status = AuthStatus.starting;
  AuthSession? session;

  /// Why the person is looking at the sign-in screen, when it is not simply "not signed in yet".
  SignInException? error;

  static const _sessionKey = 'waypoint.loader.session';
  static const _stateKey = 'waypoint.loader.oidc.state';
  static const _verifierKey = 'waypoint.loader.oidc.verifier';
  // How long before expiry a token is renewed.
  static const _renewBefore = Duration(seconds: 60);

  Future<_Endpoints>? _endpoints;
  Future<bool>? _renewing;
  Timer? _timer;

  @override
  void dispose() {
    _timer?.cancel();
    super.dispose();
  }

  // ------------------------------------------------------------------ start-up

  /// Call once when the app starts: finishes a sign-in the browser has just returned from,
  /// or restores the session of this tab.
  Future<void> start() async {
    if (!config.configured) {
      _fail(SignInException(SignInError.notConfigured));
      return;
    }
    final url = browser.location;
    final returning = url.path.endsWith(loaderCallbackPath) && (url.queryParameters.containsKey('code') || url.queryParameters.containsKey('error'));
    try {
      if (returning) {
        await _completeSignIn(url);
      } else {
        await _restore();
      }
    } on SignInException catch (e) {
      _fail(e);
    }
  }

  Future<void> _restore() async {
    final raw = browser.sessionGet(_sessionKey);
    if (raw == null) return _setSignedOut();
    AuthSession stored;
    try {
      stored = AuthSession.fromJson(jsonDecode(raw) as Map<String, dynamic>);
    } catch (_) {
      browser.sessionRemove(_sessionKey);
      return _setSignedOut();
    }
    session = stored;
    status = AuthStatus.signedIn;
    if (_expiring(stored)) {
      final ok = await renew();
      if (!ok && session == null) return; // renew() already signed out and said why
    }
    _schedule();
    notifyListeners();
  }

  // ------------------------------------------------------------------ sign-in

  /// Sends the browser to the identity server. `prompt=login` makes the next person on a
  /// shared tablet enter their own credentials instead of inheriting the previous session.
  Future<void> signIn() async {
    if (!config.configured) return _fail(SignInException(SignInError.notConfigured));
    error = null;
    try {
      final endpoints = await _discover();
      final verifier = _random(64);
      final state = _random(24);
      final nonce = _random(24);
      browser.sessionSet(_verifierKey, verifier);
      browser.sessionSet(_stateKey, state);
      final target = Uri.parse(endpoints.authorization).replace(queryParameters: {
        ...Uri.parse(endpoints.authorization).queryParameters,
        'response_type': 'code',
        'client_id': config.clientId,
        'redirect_uri': config.redirectUri(browser.origin),
        'scope': config.scopes,
        // RFC 8707: asks for an access token for the Waypoint API (see OidcConfig.resource).
        if (config.resource.isNotEmpty) 'resource': config.resource,
        'state': state,
        'nonce': nonce,
        'code_challenge': challengeFor(verifier),
        'code_challenge_method': 'S256',
        'prompt': 'login',
      });
      browser.navigate(target.toString());
    } on SignInException catch (e) {
      _fail(e);
    }
  }

  Future<void> _completeSignIn(Uri url) async {
    final q = url.queryParameters;
    final expectedState = browser.sessionGet(_stateKey);
    final verifier = browser.sessionGet(_verifierKey);
    // Both are single use, and the code leaves the address bar before anything else happens.
    browser.sessionRemove(_stateKey);
    browser.sessionRemove(_verifierKey);
    browser.replaceUrl(loaderBasePath);
    if (q['error'] != null) {
      throw SignInException(SignInError.denied, q['error_description'] ?? q['error']!);
    }
    if (expectedState == null || verifier == null || q['state'] != expectedState || (q['code'] ?? '').isEmpty) {
      throw SignInException(SignInError.notVerified);
    }
    final endpoints = await _discover();
    final tokens = await _tokenRequest(endpoints.token, {
      'grant_type': 'authorization_code',
      'code': q['code']!,
      'redirect_uri': config.redirectUri(browser.origin),
      'client_id': config.clientId,
      'code_verifier': verifier,
      if (config.resource.isNotEmpty) 'resource': config.resource,
    }, failure: SignInError.exchangeFailed);
    final access = tokens['access_token'] as String?;
    if (access == null) throw SignInException(SignInError.exchangeFailed);
    // Fail here, with the reason, rather than later as a 401 from every API call.
    _checkAudience(access);

    // The access token alone decides what the person may do, so the profile is read with it and
    // only a loader gets in; nothing is stored until that has been checked.
    final Profile profile;
    try {
      final body = await ApiClient(tokenProvider: () async => access, client: _client, baseUrl: _apiBase).get('/shared/profiles/me');
      final json = body is Map<String, dynamic> ? (body['profile'] as Map<String, dynamic>? ?? body) : null;
      if (json == null || json['userId'] == null) throw SignInException(SignInError.noProfile);
      profile = Profile.fromJson(json);
    } on ApiException catch (e) {
      throw SignInException(e.status == 404 ? SignInError.noProfile : SignInError.exchangeFailed, '${e.status}');
    } on ConnectionLostException {
      throw SignInException(SignInError.noConnection);
    }
    if (!profile.roles.map((r) => r.toUpperCase()).contains('LOADER')) throw SignInException(SignInError.wrongRole, profile.role);

    final s = AuthSession(
      accessToken: access,
      refreshToken: tokens['refresh_token'] as String?,
      idToken: tokens['id_token'] as String?,
      profile: profile,
      expiresAt: _expiry(tokens),
    );
    _store(s);
  }

  // ------------------------------------------------------------------ renewal

  bool _expiring(AuthSession s) => !_now().add(_renewBefore).isBefore(s.expiresAt);

  /// A token that is good for the next request: renewed first when it is about to expire.
  /// Throws a 401 when the session cannot be kept, which the screens treat as "sign in again".
  Future<String> validToken() async {
    final s = session;
    if (s == null) throw ApiException(401, '');
    if (_expiring(s)) {
      final ok = await renew();
      if (!ok || session == null) throw ApiException(401, '');
    }
    return session!.accessToken;
  }

  /// Called when an API request was refused with 401: try a renewal, report whether it worked.
  Future<bool> renewAfterRefusal() => renew();

  /// Renews the session with the refresh token (one renewal at a time). Returns false when
  /// there is nothing to renew with or the identity server refuses; a refusal signs the person out.
  Future<bool> renew() => _renewing ??= _renew().whenComplete(() => _renewing = null);

  Future<bool> _renew() async {
    final s = session;
    final refresh = s?.refreshToken;
    if (s == null || refresh == null) {
      _expired();
      return false;
    }
    try {
      final endpoints = await _discover();
      final tokens = await _tokenRequest(endpoints.token, {
        'grant_type': 'refresh_token',
        'refresh_token': refresh,
        'client_id': config.clientId,
        if (config.resource.isNotEmpty) 'resource': config.resource,
      }, failure: SignInError.sessionExpired);
      final access = tokens['access_token'] as String?;
      if (access == null) throw SignInException(SignInError.sessionExpired);
      _checkAudience(access);
      // Identity servers may rotate the refresh token; keep the newest one.
      _store(s.renewed(accessToken: access, refreshToken: tokens['refresh_token'] as String?, idToken: tokens['id_token'] as String?, expiresAt: _expiry(tokens)), notify: true);
      return true;
    } on SignInException catch (e) {
      if (e.kind == SignInError.noConnection) return false; // keep the session; the next request retries
      if (e.kind == SignInError.wrongAudience) {
        _fail(e);
      } else {
        _expired();
      }
      return false;
    }
  }

  void _schedule() {
    _timer?.cancel();
    final s = session;
    if (!_autoRenew || s == null || s.refreshToken == null) return;
    var wait = s.expiresAt.subtract(_renewBefore).difference(_now());
    if (wait < const Duration(seconds: 5)) wait = const Duration(seconds: 5);
    _timer = Timer(wait, () => unawaited(renew()));
  }

  // ------------------------------------------------------------------ sign-out

  /// Ends the session here and, when the identity server has an end-session endpoint, there
  /// too, so the next person on a shared tablet is not signed in by the previous one's cookie.
  Future<void> signOut() async {
    final idToken = session?.idToken;
    _clear();
    status = AuthStatus.signedOut;
    error = null;
    notifyListeners();
    final end = (await _discoverQuietly())?.endSession;
    if (end != null) {
      final target = Uri.parse(end).replace(queryParameters: {
        ...Uri.parse(end).queryParameters,
        if (idToken != null) 'id_token_hint': idToken,
        'client_id': config.clientId,
        'post_logout_redirect_uri': config.postLogoutUri(browser.origin),
      });
      browser.navigate(target.toString());
    }
  }

  // ------------------------------------------------------------------ plumbing

  void _store(AuthSession s, {bool notify = true}) {
    session = s;
    status = AuthStatus.signedIn;
    error = null;
    browser.sessionSet(_sessionKey, jsonEncode(s.toJson()));
    _schedule();
    if (notify) notifyListeners();
  }

  void _clear() {
    _timer?.cancel();
    session = null;
    browser.sessionRemove(_sessionKey);
  }

  void _expired() {
    _clear();
    status = AuthStatus.signedOut;
    error = SignInException(SignInError.sessionExpired);
    notifyListeners();
  }

  void _setSignedOut() {
    status = AuthStatus.signedOut;
    notifyListeners();
  }

  void _fail(SignInException e) {
    _clear();
    status = AuthStatus.signedOut;
    error = e;
    notifyListeners();
  }

  /// When an API resource is configured, the access token must be for it: the services accept only
  /// that audience, so a token for anything else would sign the loader in and then be refused by
  /// every call. The token is read, not verified (the API verifies it); one that cannot be read
  /// as a JWT is left to the API to judge.
  void _checkAudience(String accessToken) {
    if (config.resource.isEmpty) return;
    final audiences = audiencesOf(accessToken);
    if (audiences == null) return;
    if (!audiences.contains(config.resource)) {
      throw SignInException(SignInError.wrongAudience, audiences.isEmpty ? 'none' : audiences.join(', '));
    }
  }

  DateTime _expiry(Map<String, dynamic> tokens) => _now().add(Duration(seconds: (tokens['expires_in'] as num?)?.toInt() ?? 3600));

  Future<Map<String, dynamic>> _tokenRequest(String endpoint, Map<String, String> form, {required SignInError failure}) async {
    final http.Response res;
    try {
      res = await _client.post(Uri.parse(endpoint), headers: {'Accept': 'application/json'}, body: form).timeout(const Duration(seconds: 15));
    } on TimeoutException {
      throw SignInException(SignInError.noConnection);
    } on http.ClientException {
      throw SignInException(SignInError.noConnection);
    }
    if (res.statusCode >= 500) throw SignInException(SignInError.noConnection, 'Identity service answered ${res.statusCode}.');
    if (res.statusCode != 200) throw SignInException(failure, '${res.statusCode}');
    try {
      return jsonDecode(res.body) as Map<String, dynamic>;
    } catch (_) {
      throw SignInException(failure);
    }
  }

  Future<_Endpoints> _discover() => _endpoints ??= _loadEndpoints().catchError((Object e) {
        _endpoints = null; // try again next time rather than remembering a failure
        throw e;
      });

  Future<_Endpoints?> _discoverQuietly() async {
    try {
      return await _discover();
    } catch (_) {
      return null;
    }
  }

  Future<_Endpoints> _loadEndpoints() async {
    final base = config.issuerBase;
    try {
      final res = await _client.get(Uri.parse('$base/.well-known/openid-configuration'), headers: {'Accept': 'application/json'}).timeout(const Duration(seconds: 15));
      if (res.statusCode == 200) {
        final j = jsonDecode(res.body) as Map<String, dynamic>;
        final authorization = j['authorization_endpoint'] as String?;
        final token = j['token_endpoint'] as String?;
        if (authorization != null && token != null) return _Endpoints(authorization, token, j['end_session_endpoint'] as String?);
      }
    } on TimeoutException {
      throw SignInException(SignInError.noConnection);
    } on http.ClientException {
      throw SignInException(SignInError.noConnection);
    } catch (_) {/* a malformed discovery document: use the standard paths below */}
    return _Endpoints('$base/oauth2/authorize', '$base/oauth2/token', null);
  }
}

class _Endpoints {
  _Endpoints(this.authorization, this.token, this.endSession);
  final String authorization;
  final String token;
  final String? endSession;
}

/// The `aud` claim of a JWT as a list, or null when [token] is not a readable JWT.
List<String>? audiencesOf(String token) {
  final parts = token.split('.');
  if (parts.length != 3) return null;
  try {
    final claims = jsonDecode(utf8.decode(base64Url.decode(base64Url.normalize(parts[1])))) as Map<String, dynamic>;
    final aud = claims['aud'];
    if (aud is String) return [aud];
    if (aud is List) return aud.map((e) => '$e').toList();
    return <String>[];
  } catch (_) {
    return null;
  }
}

/// The PKCE challenge for a verifier: base64url(SHA-256(verifier)) without padding.
String challengeFor(String verifier) => base64Url.encode(sha256.convert(utf8.encode(verifier)).bytes).replaceAll('=', '');

String _random(int length) {
  const chars = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-._~';
  final r = Random.secure();
  return List.generate(length, (_) => chars[r.nextInt(chars.length)]).join();
}
