import 'dart:async';
import 'dart:convert';
import 'dart:math';

import 'package:http/http.dart' as http;
import 'package:http_parser/http_parser.dart';

/// The loader app is served by the Waypoint NGINX, so by default it talks to
/// its own origin (`/api/v1`). Override with --dart-define=API_BASE_URL for dev.
const _apiOverride = String.fromEnvironment('API_BASE_URL');
final apiBaseUrl = _apiOverride.isNotEmpty ? _apiOverride : '${Uri.base.origin}/api/v1';

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
  /// [tokenProvider] returns a valid access token, renewing it first when it is about to
  /// expire. [onUnauthorized] is asked once when a request is refused with 401 (the token
  /// was revoked or expired early); it returns true when a fresh token is available, and
  /// the request is then repeated with it.
  ApiClient({required this.tokenProvider, this.onUnauthorized, http.Client? client, String? baseUrl})
      : baseUrl = baseUrl ?? apiBaseUrl,
        _client = client ?? http.Client();

  final Future<String> Function() tokenProvider;
  final Future<bool> Function()? onUnauthorized;
  final String baseUrl;
  final http.Client _client;
  static const timeout = Duration(seconds: 15);

  Map<String, String> _headers(String token, Map<String, String>? extra) => {
        'Accept': 'application/json',
        'Content-Type': 'application/json',
        'Authorization': 'Bearer $token',
        ...?extra,
      };

  Future<http.Response> _guarded(Future<http.Response> Function() call) async {
    try {
      return await call().timeout(timeout);
    } on TimeoutException catch (e) {
      throw ConnectionLostException(e);
    } on http.ClientException catch (e) {
      throw ConnectionLostException(e);
    }
  }

  /// Sends one request built by [call] for the given token. A 401 is retried once
  /// with a renewed token; [call] therefore builds a fresh request each time.
  Future<dynamic> _send(Future<http.Response> Function(String token) call) async {
    var token = await tokenProvider();
    var res = await _guarded(() => call(token));
    if (res.statusCode == 401 && onUnauthorized != null && await onUnauthorized!()) {
      token = await tokenProvider();
      res = await _guarded(() => call(token));
    }
    if (res.statusCode == 502 || res.statusCode == 503 || res.statusCode == 504) throw ConnectionLostException(ApiException(res.statusCode, res.body));
    if (res.statusCode < 200 || res.statusCode >= 300) throw ApiException(res.statusCode, res.body);
    return res.body.isEmpty ? <String, dynamic>{} : jsonDecode(res.body);
  }

  Uri _uri(String path) => Uri.parse('$baseUrl$path');

  Future<dynamic> get(String path) => _send((t) => _client.get(_uri(path), headers: _headers(t, null)));
  Future<dynamic> post(String path, {Object? body, Map<String, String>? headers}) =>
      _send((t) => _client.post(_uri(path), headers: _headers(t, headers), body: body == null ? null : jsonEncode(body)));
  Future<dynamic> put(String path, {Object? body, Map<String, String>? headers}) =>
      _send((t) => _client.put(_uri(path), headers: _headers(t, headers), body: body == null ? null : jsonEncode(body)));
  Future<dynamic> delete(String path) => _send((t) => _client.delete(_uri(path), headers: _headers(t, null)));

  /// Multipart upload of one file (a shortfall photo).
  Future<dynamic> upload(String path, {required List<int> bytes, required String mime, String filename = 'photo', Map<String, String>? headers}) => _send((t) async {
        final req = http.MultipartRequest('POST', _uri(path))
          ..headers.addAll({'Accept': 'application/json', 'Authorization': 'Bearer $t', ...?headers})
          ..files.add(http.MultipartFile.fromBytes('file', bytes, filename: filename, contentType: MediaType.parse(mime)));
        return http.Response.fromStream(await _client.send(req));
      });
}
