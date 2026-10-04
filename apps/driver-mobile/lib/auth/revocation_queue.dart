import 'dart:async';
import 'dart:convert';

import 'package:flutter_secure_storage/flutter_secure_storage.dart';

/// Refresh tokens that still have to be revoked at the identity provider because the driver signed out
/// without a connection (or the provider could not be reached). They are not usable by the app: they are
/// kept only so the revocation can be repeated, then removed.
abstract class RevocationQueue {
  Future<List<String>> read();
  Future<void> add(String token);
  Future<void> remove(String token);
}

/// Kept in the Android Keystore / iOS Keychain, like the tokens themselves.
class SecureRevocationQueue implements RevocationQueue {
  SecureRevocationQueue({FlutterSecureStorage? storage}) : _storage = storage ?? const FlutterSecureStorage();

  static const _key = 'waypoint.auth.pending-revocations.v1';

  /// A device that is signed out a lot without signal must not keep tokens forever.
  static const maxPending = 10;

  final FlutterSecureStorage _storage;
  Future<void> _tail = Future<void>.value();

  /// One change at a time, so two sign-outs close together cannot overwrite each other's entry.
  Future<T> _serial<T>(Future<T> Function() operation) {
    final result = Completer<T>();
    _tail = _tail.then((_) async {
      try {
        result.complete(await operation());
      } on Object catch (error, stack) {
        result.completeError(error, stack);
      }
    });
    return result.future;
  }

  Future<List<String>> _load() async {
    try {
      final raw = await _storage.read(key: _key);
      if (raw == null) return [];
      final decoded = jsonDecode(raw);
      if (decoded is! List) return [];
      return [for (final item in decoded) if (item is String && item.isNotEmpty) item];
    } on Object {
      // Unreadable: start again rather than fail a sign-out.
      return [];
    }
  }

  Future<void> _save(List<String> tokens) async {
    if (tokens.isEmpty) {
      await _storage.delete(key: _key);
    } else {
      await _storage.write(key: _key, value: jsonEncode(tokens));
    }
  }

  @override
  Future<List<String>> read() => _serial(_load);

  @override
  Future<void> add(String token) => _serial(() async {
        final tokens = await _load();
        if (!tokens.contains(token)) tokens.add(token);
        await _save(tokens.length > maxPending ? tokens.sublist(tokens.length - maxPending) : tokens);
      });

  @override
  Future<void> remove(String token) => _serial(() async {
        final tokens = await _load();
        if (tokens.remove(token)) await _save(tokens);
      });
}

class MemoryRevocationQueue implements RevocationQueue {
  final tokens = <String>[];

  @override
  Future<List<String>> read() async => List.of(tokens);

  @override
  Future<void> add(String token) async {
    if (!tokens.contains(token)) tokens.add(token);
  }

  @override
  Future<void> remove(String token) async => tokens.remove(token);
}
