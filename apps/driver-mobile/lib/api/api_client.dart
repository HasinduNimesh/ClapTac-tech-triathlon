import 'dart:async';
import 'dart:convert';
import 'dart:math';

import 'package:flutter/foundation.dart' show kIsWeb;
import 'package:http/http.dart' as http;

/// Configure with `--dart-define=API_BASE_URL=... --dart-define=OIDC_ISSUER=...`.
/// The web build is served by the Waypoint NGINX, so it talks to its own origin.
const _apiOverride = String.fromEnvironment('API_BASE_URL');
const _issuerOverride = String.fromEnvironment('OIDC_ISSUER');
final apiBaseUrl = _apiOverride.isNotEmpty ? _apiOverride : (kIsWeb ? '${Uri.base.origin}/api/v1' : 'http://localhost/api/v1');
final oidcIssuer = _issuerOverride.isNotEmpty ? _issuerOverride : (kIsWeb ? Uri.base.origin : 'http://localhost:8090');
const oidcClientId = String.fromEnvironment('OIDC_CLIENT_ID', defaultValue: 'waypoint-mobile');

class ApiException implements Exception {
  ApiException(this.status, this.body);
  final int status;
  final String body;
  @override
  String toString() => '$status: $body';
}

/// No connectivity or the service did not answer. Work is kept on the device.
class OfflineException implements Exception {
  OfflineException(this.cause);
  final Object cause;
  @override
  String toString() => 'offline: $cause';
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
  ApiClient({required this.tokenProvider, http.Client? client, String? baseUrl}) : baseUrl = baseUrl ?? apiBaseUrl, _client = client ?? http.Client();

  final String Function() tokenProvider;
  final String baseUrl;
  final http.Client _client;
  static const timeout = Duration(seconds: 15);

  Map<String, String> _headers([Map<String, String>? extra, bool json = true]) => {
        'Accept': 'application/json',
        if (json) 'Content-Type': 'application/json',
        'Authorization': 'Bearer ${tokenProvider()}',
        ...?extra,
      };

  Future<dynamic> _send(Future<http.Response> Function() call) async {
    http.Response res;
    try {
      res = await call().timeout(timeout);
    } on TimeoutException catch (e) {
      throw OfflineException(e);
    } on http.ClientException catch (e) {
      throw OfflineException(e);
    }
    if (res.statusCode == 502 || res.statusCode == 503 || res.statusCode == 504) throw OfflineException(ApiException(res.statusCode, res.body));
    if (res.statusCode < 200 || res.statusCode >= 300) throw ApiException(res.statusCode, res.body);
    return res.body.isEmpty ? <String, dynamic>{} : jsonDecode(res.body);
  }

  Future<dynamic> get(String path) => _send(() => _client.get(Uri.parse('$baseUrl$path'), headers: _headers(null, false)));
  Future<dynamic> post(String path, {Object? body, Map<String, String>? headers}) =>
      _send(() => _client.post(Uri.parse('$baseUrl$path'), headers: _headers(headers), body: body == null ? null : jsonEncode(body)));
  Future<dynamic> put(String path, {Object? body, Map<String, String>? headers}) =>
      _send(() => _client.put(Uri.parse('$baseUrl$path'), headers: _headers(headers), body: body == null ? null : jsonEncode(body)));
  Future<dynamic> delete(String path) => _send(() => _client.delete(Uri.parse('$baseUrl$path'), headers: _headers()));

  Future<dynamic> multipart(String path, {required Map<String, String> fields, required List<int> bytes, required String filename, Map<String, String>? headers}) {
    return _send(() async {
      final request = http.MultipartRequest('POST', Uri.parse('$baseUrl$path'))
        ..headers.addAll(_headers(headers, false))
        ..fields.addAll(fields)
        ..files.add(http.MultipartFile.fromBytes('file', bytes, filename: filename));
      return http.Response.fromStream(await _client.send(request));
    });
  }
}
