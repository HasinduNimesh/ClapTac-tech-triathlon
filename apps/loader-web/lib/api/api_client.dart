import 'dart:async';
import 'dart:convert';
import 'dart:math';

import 'package:http/http.dart' as http;

/// The loader app is served by the Waypoint NGINX, so by default it talks to
/// its own origin (`/api/v1`, `/oauth2`). Override with --dart-define for dev.
const _apiOverride = String.fromEnvironment('API_BASE_URL');
const _issuerOverride = String.fromEnvironment('OIDC_ISSUER');
final apiBaseUrl = _apiOverride.isNotEmpty ? _apiOverride : '${Uri.base.origin}/api/v1';
final oidcIssuer = _issuerOverride.isNotEmpty ? _issuerOverride : Uri.base.origin;
const oidcClientId = String.fromEnvironment('OIDC_CLIENT_ID', defaultValue: 'waypoint-loader');

class ApiException implements Exception {
  ApiException(this.status, this.body);
  final int status;
  final String body;

  /// Problem-details `type`, e.g. `dispatcher_decision_required`.
  String get type {
    try {
      final j = jsonDecode(body);
      if (j is Map<String, dynamic>) return '${j['type'] ?? ''}';
    } catch (_) {}
    return '';
  }

  String get detail {
    try {
      final j = jsonDecode(body);
      if (j is Map<String, dynamic>) return '${j['detail'] ?? j['title'] ?? body}';
    } catch (_) {}
    return body;
  }

  @override
  String toString() => '$status: $body';
}

/// The service did not answer. The loader app is online-only: nothing is
/// saved for later, the loader retries when the connection is back.
class ConnectionLostException implements Exception {
  ConnectionLostException(this.cause);
  final Object cause;
  @override
  String toString() => 'connection lost: $cause';
}

String newOperationId() {
  final r = Random.secure();
  final bytes = List<int>.generate(16, (_) => r.nextInt(256));
  bytes[6] = (bytes[6] & 0x0f) | 0x40;
  bytes[8] = (bytes[8] & 0x3f) | 0x80;
  final hex = bytes.map((b) => b.toRadixString(16).padLeft(2, '0')).join();
  return '${hex.substring(0, 8)}-${hex.substring(8, 12)}-${hex.substring(12, 16)}-${hex.substring(16, 20)}-${hex.substring(20)}';
}

class ApiClient {
  ApiClient({required this.tokenProvider, http.Client? client, String? baseUrl})
      : baseUrl = baseUrl ?? apiBaseUrl,
        _client = client ?? http.Client();

  final String Function() tokenProvider;
  final String baseUrl;
  final http.Client _client;
  static const timeout = Duration(seconds: 15);

  Map<String, String> _headers(Map<String, String>? extra) => {
        'Accept': 'application/json',
        'Content-Type': 'application/json',
        'Authorization': 'Bearer ${tokenProvider()}',
        ...?extra,
      };

  Future<dynamic> _send(Future<http.Response> Function() call) async {
    http.Response res;
    try {
      res = await call().timeout(timeout);
    } on TimeoutException catch (e) {
      throw ConnectionLostException(e);
    } on http.ClientException catch (e) {
      throw ConnectionLostException(e);
    }
    if (res.statusCode == 502 || res.statusCode == 503 || res.statusCode == 504) throw ConnectionLostException(ApiException(res.statusCode, res.body));
    if (res.statusCode < 200 || res.statusCode >= 300) throw ApiException(res.statusCode, res.body);
    return res.body.isEmpty ? <String, dynamic>{} : jsonDecode(res.body);
  }

  Future<dynamic> get(String path) => _send(() => _client.get(Uri.parse('$baseUrl$path'), headers: _headers(null)));
  Future<dynamic> post(String path, {Object? body, Map<String, String>? headers}) =>
      _send(() => _client.post(Uri.parse('$baseUrl$path'), headers: _headers(headers), body: body == null ? null : jsonEncode(body)));
  Future<dynamic> put(String path, {Object? body, Map<String, String>? headers}) =>
      _send(() => _client.put(Uri.parse('$baseUrl$path'), headers: _headers(headers), body: body == null ? null : jsonEncode(body)));
  Future<dynamic> delete(String path) => _send(() => _client.delete(Uri.parse('$baseUrl$path'), headers: _headers(null)));
}
