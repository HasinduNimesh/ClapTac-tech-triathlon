import 'dart:async';
import 'dart:convert';

import 'package:flutter/foundation.dart';
import 'package:http/http.dart' as http;

import '../api/api_client.dart';
import '../offline/store.dart';
import '../shared/models.dart';

class AuthSession {
  AuthSession({required this.subject, required this.accessToken, required this.profile, required this.signedInAt});

  final String subject;
  final String accessToken;
  final Profile profile;
  final DateTime signedInAt;

  Map<String, dynamic> toJson() => {'subject': subject, 'accessToken': accessToken, 'profile': profile.toJson(), 'signedInAt': signedInAt.toIso8601String()};
  factory AuthSession.fromJson(Map<String, dynamic> j) => AuthSession(
        subject: j['subject'] as String,
        accessToken: j['accessToken'] as String,
        profile: Profile.fromJson(j['profile'] as Map<String, dynamic>),
        signedInAt: DateTime.parse(j['signedInAt'] as String),
      );
}

enum SignInError { invalidCredentials, noSignal, noProfile, wrongRole }

class SignInException implements Exception {
  SignInException(this.kind, [this.detail = '']);
  final SignInError kind;
  final String detail;
}

/// Signs in with the staff ID and password typed on the device (the shared
/// sign-in screen). The local identity provider issues an authorization code
/// for the form post, which is exchanged for an access token.
class AuthService extends ChangeNotifier {
  AuthService({required this.store, http.Client? client}) : _client = client ?? http.Client();

  final LocalStore store;
  final http.Client _client;
  AuthSession? session;
  static const _sessionKey = 'session.current';
  static const _lastKey = 'session.last';

  Future<void> restore() async {
    final saved = await store.readJson(_sessionKey);
    if (saved != null) session = AuthSession.fromJson(saved);
    notifyListeners();
  }

  /// The last session on this phone, offered as "Continue offline" when there is no signal.
  Future<AuthSession?> lastSession() async {
    final saved = await store.readJson(_lastKey);
    return saved == null ? null : AuthSession.fromJson(saved);
  }

  Future<AuthSession> signIn(String username, String password, {bool keepSignedIn = true}) async {
    const redirect = 'waypoint://auth/callback';
    try {
      final authorize = http.Request('POST', Uri.parse('$oidcIssuer/oauth2/authorize'))
        ..followRedirects = false
        ..headers['Accept'] = 'application/json'
        ..bodyFields = {'username': username.trim(), 'password': password, 'redirect_uri': redirect, 'state': newOperationId(), 'client_id': oidcClientId, 'response_mode': 'json'};
      final response = await _client.send(authorize).timeout(const Duration(seconds: 15));
      if (response.statusCode == 401) throw SignInException(SignInError.invalidCredentials);
      final body = await response.stream.bytesToString();
      final location = response.headers['location'];
      String? code = location == null ? null : Uri.parse(location).queryParameters['code'];
      if (code == null && body.trim().startsWith('{')) code = (jsonDecode(body) as Map<String, dynamic>)['code'] as String?;
      if (code == null) throw SignInException(SignInError.invalidCredentials, 'No authorization code (${response.statusCode}).');
      final tokenRes = await _client.post(Uri.parse('$oidcIssuer/oauth2/token'), body: {'grant_type': 'authorization_code', 'code': code, 'client_id': oidcClientId, 'redirect_uri': redirect}).timeout(const Duration(seconds: 15));
      if (tokenRes.statusCode != 200) throw SignInException(SignInError.invalidCredentials);
      final token = RegExp('"access_token"\\s*:\\s*"([^"]+)"').firstMatch(tokenRes.body)?.group(1);
      if (token == null) throw SignInException(SignInError.invalidCredentials);
      final api = ApiClient(tokenProvider: () => token);
      final profileBody = await api.get('/shared/profiles/me');
      final profileJson = profileBody is Map<String, dynamic> ? (profileBody['profile'] as Map<String, dynamic>? ?? profileBody) : null;
      if (profileJson == null) throw SignInException(SignInError.noProfile);
      final profile = Profile.fromJson(profileJson);
      if (profile.role != 'DRIVER' && profile.role != 'LOADER') throw SignInException(SignInError.wrongRole, profile.role);
      final s = AuthSession(subject: profile.userId, accessToken: token, profile: profile, signedInAt: DateTime.now());
      session = s;
      if (keepSignedIn) await store.writeJson(_sessionKey, s.toJson());
      await store.writeJson(_lastKey, s.toJson());
      notifyListeners();
      return s;
    } on TimeoutException {
      throw SignInException(SignInError.noSignal);
    } on http.ClientException {
      throw SignInException(SignInError.noSignal);
    } on OfflineException {
      throw SignInException(SignInError.noSignal);
    }
  }

  /// Opens the saved run without a network check (Figma "Sign in / Phone / No signal").
  Future<void> continueOffline(AuthSession last) async {
    session = last;
    notifyListeners();
  }

  /// Signs out. Saved, unsent work stays on the device for the same person.
  Future<void> signOut() async {
    session = null;
    await store.write(_sessionKey, null);
    notifyListeners();
  }
}
