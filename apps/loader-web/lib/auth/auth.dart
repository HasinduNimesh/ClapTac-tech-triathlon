import 'dart:async';
import 'dart:convert';

import 'package:flutter/foundation.dart';
import 'package:http/http.dart' as http;
import 'package:shared_preferences/shared_preferences.dart';

import '../api/api_client.dart';
import '../shared/models.dart';

class AuthSession {
  AuthSession({required this.accessToken, required this.profile, required this.expiresAt});

  final String accessToken;
  final Profile profile;
  final DateTime expiresAt;

  bool get expired => DateTime.now().isAfter(expiresAt);

  Map<String, dynamic> toJson() => {'accessToken': accessToken, 'profile': profile.toJson(), 'expiresAt': expiresAt.toIso8601String()};
  factory AuthSession.fromJson(Map<String, dynamic> j) => AuthSession(
        accessToken: j['accessToken'] as String,
        profile: Profile.fromJson(j['profile'] as Map<String, dynamic>),
        expiresAt: DateTime.parse(j['expiresAt'] as String),
      );
}

enum SignInError { invalidCredentials, noConnection, noProfile, wrongRole }

class SignInException implements Exception {
  SignInException(this.kind, [this.detail = '']);
  final SignInError kind;
  final String detail;
}

/// Staff ID + password sign-in for the loader. Only the session token is kept
/// in the browser (so a page reload stays signed in); no loading data is stored.
class AuthService extends ChangeNotifier {
  AuthService({http.Client? client}) : _client = client ?? http.Client();

  final http.Client _client;
  AuthSession? session;
  static const _key = 'waypoint.loader.session';

  Future<void> restore() async {
    try {
      final raw = (await SharedPreferences.getInstance()).getString(_key);
      if (raw != null) {
        final s = AuthSession.fromJson(jsonDecode(raw) as Map<String, dynamic>);
        if (!s.expired) session = s;
      }
    } catch (_) {/* start signed out */}
    notifyListeners();
  }

  Future<AuthSession> signIn(String username, String password) async {
    const redirect = 'waypoint-loader://signed-in';
    try {
      // The local identity server returns the authorization code as JSON
      // (response_mode=json) because a browser cannot read a redirect.
      final authorize = await _client.post(Uri.parse('$oidcIssuer/oauth2/authorize'), headers: {'Accept': 'application/json'}, body: {
        'username': username.trim(),
        'password': password,
        'redirect_uri': redirect,
        'state': newOperationId(),
        'client_id': oidcClientId,
        'response_mode': 'json',
      }).timeout(const Duration(seconds: 15));
      if (authorize.statusCode == 401) throw SignInException(SignInError.invalidCredentials);
      if (authorize.statusCode != 200) throw SignInException(SignInError.noConnection, 'Identity service answered ${authorize.statusCode}.');
      final code = (jsonDecode(authorize.body) as Map<String, dynamic>)['code'] as String?;
      if (code == null) throw SignInException(SignInError.invalidCredentials);
      final tokenRes = await _client.post(Uri.parse('$oidcIssuer/oauth2/token'), body: {'grant_type': 'authorization_code', 'code': code, 'client_id': oidcClientId, 'redirect_uri': redirect}).timeout(const Duration(seconds: 15));
      if (tokenRes.statusCode != 200) throw SignInException(SignInError.invalidCredentials);
      final tokenJson = jsonDecode(tokenRes.body) as Map<String, dynamic>;
      final token = tokenJson['access_token'] as String;
      final expiresIn = (tokenJson['expires_in'] as num?)?.toInt() ?? 3600;
      final profileBody = await ApiClient(tokenProvider: () => token).get('/shared/profiles/me');
      final profileJson = profileBody is Map<String, dynamic> ? (profileBody['profile'] as Map<String, dynamic>? ?? profileBody) : null;
      if (profileJson == null) throw SignInException(SignInError.noProfile);
      final profile = Profile.fromJson(profileJson);
      if (profile.role != 'LOADER') throw SignInException(SignInError.wrongRole, profile.role);
      final s = AuthSession(accessToken: token, profile: profile, expiresAt: DateTime.now().add(Duration(seconds: expiresIn - 60)));
      session = s;
      try {
        await (await SharedPreferences.getInstance()).setString(_key, jsonEncode(s.toJson()));
      } catch (_) {/* stays signed in for this tab */}
      notifyListeners();
      return s;
    } on TimeoutException {
      throw SignInException(SignInError.noConnection);
    } on http.ClientException {
      throw SignInException(SignInError.noConnection);
    } on ConnectionLostException {
      throw SignInException(SignInError.noConnection);
    }
  }

  Future<void> signOut() async {
    session = null;
    try {
      await (await SharedPreferences.getInstance()).remove(_key);
    } catch (_) {}
    notifyListeners();
  }
}
