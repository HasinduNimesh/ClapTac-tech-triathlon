import 'dart:convert';

import 'package:flutter_secure_storage/flutter_secure_storage.dart';

import 'oidc_tokens.dart';
import 'profile_api.dart';

/// What is kept on the phone between launches so a driver stays signed in while offline.
class StoredAuth {
  const StoredAuth({required this.tokens, required this.profile});

  factory StoredAuth.fromJson(Map<String, Object?> json) => StoredAuth(
        tokens: OidcTokens.fromJson((json['tokens']! as Map).cast<String, Object?>()),
        profile: DriverProfile.fromJson((json['profile']! as Map).cast<String, Object?>()),
      );

  final OidcTokens tokens;
  final DriverProfile profile;

  Map<String, Object?> toJson() => {'tokens': tokens.toJson(), 'profile': profile.toJson()};
}

abstract class AuthStore {
  Future<StoredAuth?> read();
  Future<void> write(StoredAuth value);
  Future<void> clear();
}

/// Tokens go to the Android Keystore / iOS Keychain, never to plain preferences.
class SecureAuthStore implements AuthStore {
  SecureAuthStore({FlutterSecureStorage? storage}) : _storage = storage ?? const FlutterSecureStorage();

  static const _key = 'waypoint.auth.v1';
  final FlutterSecureStorage _storage;

  @override
  Future<StoredAuth?> read() async {
    try {
      final raw = await _storage.read(key: _key);
      if (raw == null) return null;
      return StoredAuth.fromJson((jsonDecode(raw) as Map).cast<String, Object?>());
    } on Object {
      // Unreadable or corrupt: treat as signed out rather than crash on launch.
      await clear();
      return null;
    }
  }

  @override
  Future<void> write(StoredAuth value) => _storage.write(key: _key, value: jsonEncode(value.toJson()));

  @override
  Future<void> clear() async {
    try {
      await _storage.delete(key: _key);
    } on Object {
      // Nothing more to do; the next sign-in overwrites it.
    }
  }
}

class MemoryAuthStore implements AuthStore {
  StoredAuth? value;

  @override
  Future<StoredAuth?> read() async => value;

  @override
  Future<void> write(StoredAuth next) async => value = next;

  @override
  Future<void> clear() async => value = null;
}
