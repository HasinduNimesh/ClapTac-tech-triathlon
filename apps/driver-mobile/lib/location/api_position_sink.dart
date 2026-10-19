import 'dart:async';
import 'dart:convert';

import 'package:http/http.dart' as http;

import '../auth/auth_failure.dart';
import '../auth/auth_gateway.dart';
import 'location_reporter.dart';

/// Sends a position to `POST /api/v1/delivery/trips/{id}/location`. Best effort: any failure to reach Waypoint is
/// just "try again with the next fix". Only the server saying the trip no longer takes positions stops sharing.
class ApiPositionSink implements PositionSink {
  ApiPositionSink({required http.Client client, required String baseUrl, required AuthGateway auth, this.timeout = const Duration(seconds: 15)})
      : _client = client,
        _auth = auth,
        _baseUrl = baseUrl.replaceAll(RegExp(r'/+$'), '');

  final http.Client _client;
  final AuthGateway _auth;
  final String _baseUrl;
  final Duration timeout;

  @override
  Future<SendResult> send(String tripId, Fix fix) async {
    final String? token;
    try {
      token = await _auth.accessToken();
    } on AuthFailure {
      return SendResult.retry;
    }
    if (token == null) return SendResult.retry;
    try {
      final response = await _client
          .post(
            Uri.parse('$_baseUrl/api/v1/delivery/trips/${Uri.encodeComponent(tripId)}/location'),
            headers: {'Authorization': 'Bearer $token', 'Content-Type': 'application/json'},
            body: jsonEncode({'latitude': fix.latitude, 'longitude': fix.longitude, 'timestamp': fix.at.toUtc().toIso8601String()}),
          )
          .timeout(timeout);
      final code = response.statusCode;
      if (code >= 200 && code < 300) return SendResult.sent;
      // Not active any more (409), not this driver's trip (403), unknown (404): sending again cannot help.
      if (code == 403 || code == 404 || code == 409) return SendResult.stop;
      // A rejected timestamp or coordinate (400) or a server fault: the next fix is a fresh try.
      return SendResult.retry;
    } on TimeoutException {
      return SendResult.retry;
    } on Exception {
      return SendResult.retry;
    }
  }
}
