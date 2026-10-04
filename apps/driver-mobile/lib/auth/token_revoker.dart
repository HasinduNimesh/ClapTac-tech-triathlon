import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:http/http.dart' as http;

import 'auth_config.dart';

/// How an attempt to revoke a token ended.
enum RevokeOutcome {
  /// The provider revoked it (RFC 7009 answers 200 even for a token it no longer knows).
  done,

  /// The provider could not be reached or had a server error: try again later.
  retryLater,

  /// The provider answered that it will not do this (a client error), so repeating it will not help.
  gaveUp,

  /// The provider does not publish a revocation endpoint, or it is not one the app may use.
  notSupported,
}

abstract class TokenRevoker {
  /// Asks the identity provider to invalidate [refreshToken], so a copy that leaks after sign-out is
  /// worthless.
  Future<RevokeOutcome> revokeRefreshToken(String refreshToken);
}

/// RFC 7009 token revocation over HTTP. The endpoint comes from the provider's discovery document
/// (`revocation_endpoint`). The app is a public client, so it sends only its client id.
class HttpTokenRevoker implements TokenRevoker {
  HttpTokenRevoker({required http.Client client, required AuthConfig config, this.timeout = const Duration(seconds: 8)})
      : _client = client,
        _config = config;

  final http.Client _client;
  final AuthConfig _config;
  final Duration timeout;

  @override
  Future<RevokeOutcome> revokeRefreshToken(String refreshToken) async {
    try {
      final discovery = await _client.get(Uri.parse(_config.discoveryUrl), headers: {'Accept': 'application/json'}).timeout(timeout);
      if (discovery.statusCode >= 500 || discovery.statusCode == 408 || discovery.statusCode == 429) return RevokeOutcome.retryLater;
      if (discovery.statusCode != 200) return RevokeOutcome.gaveUp;
      final document = jsonDecode(discovery.body);
      final endpoint = document is Map ? document['revocation_endpoint'] : null;
      if (endpoint is! String || endpoint.isEmpty) return RevokeOutcome.notSupported;
      final uri = Uri.tryParse(endpoint);
      if (uri == null || !_allowed(uri)) return RevokeOutcome.notSupported;

      final response = await _client.post(
        uri,
        headers: {'Content-Type': 'application/x-www-form-urlencoded', 'Accept': 'application/json'},
        body: {'token': refreshToken, 'token_type_hint': 'refresh_token', 'client_id': _config.clientId},
      ).timeout(timeout);
      final code = response.statusCode;
      if (code == 200) return RevokeOutcome.done;
      if (code >= 500 || code == 408 || code == 429) return RevokeOutcome.retryLater;
      return RevokeOutcome.gaveUp;
    } on SocketException {
      return RevokeOutcome.retryLater;
    } on TimeoutException {
      return RevokeOutcome.retryLater;
    } on http.ClientException {
      return RevokeOutcome.retryLater;
    } on FormatException {
      // An unreadable discovery document may be a passing fault.
      return RevokeOutcome.retryLater;
    }
  }

  /// The token is never sent over plain HTTP except to a local identity server in a debug build.
  bool _allowed(Uri uri) {
    if (uri.scheme == 'https') return true;
    return uri.scheme == 'http' && !_config.releaseMode && _config.usesInsecureTransport;
  }
}
