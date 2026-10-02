import 'dart:convert';

import 'package:shared_preferences/shared_preferences.dart';

/// Device storage for the signed-in session, the saved route/load list and
/// the outgoing operation queue. Everything is keyed by the owner (subject),
/// so a shared dock tablet never mixes one loader's work with another's.
abstract class LocalStore {
  Future<String?> read(String key);
  Future<void> write(String key, String? value);

  Future<Map<String, dynamic>?> readJson(String key) async {
    final raw = await read(key);
    if (raw == null) return null;
    final decoded = jsonDecode(raw);
    return decoded is Map<String, dynamic> ? decoded : null;
  }

  Future<void> writeJson(String key, Object? value) => write(key, value == null ? null : jsonEncode(value));
}

class PrefsStore extends LocalStore {
  SharedPreferences? _prefs;
  Future<SharedPreferences> get _p async => _prefs ??= await SharedPreferences.getInstance();

  @override
  Future<String?> read(String key) async => (await _p).getString(key);

  @override
  Future<void> write(String key, String? value) async {
    final p = await _p;
    if (value == null) {
      await p.remove(key);
    } else {
      await p.setString(key, value);
    }
  }
}

class MemoryStore extends LocalStore {
  final Map<String, String> _values = {};
  @override
  Future<String?> read(String key) async => _values[key];
  @override
  Future<void> write(String key, String? value) async => value == null ? _values.remove(key) : _values[key] = value;
}
