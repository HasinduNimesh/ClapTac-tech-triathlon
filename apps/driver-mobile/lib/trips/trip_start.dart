import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:http/http.dart' as http;

import '../auth/auth_gateway.dart';
import '../data/driver_models.dart';

enum TripStartStatus {
  /// The run is in progress on the server (started now, or already was).
  started,

  /// Waypoint could not be reached; try again later without losing anything.
  offline,

  /// The stored sign-in is not accepted any more.
  signInNeeded,

  /// The server refused: the plan changed, the account cannot start trips, or similar. Retrying
  /// the same request will not help, so the driver is told why.
  refused,
}

class TripStartResult {
  const TripStartResult(this.status, [this.message]);

  final TripStartStatus status;
  final String? message;
}

abstract class TripStarter {
  /// Acknowledges the current plan version, then starts the run. Safe to repeat: both calls are
  /// idempotent on the server.
  Future<TripStartResult> start(TripInfo trip, {required String operationId});
}

/// Starts a trip through the Waypoint API: `POST /api/v1/planning/plans/{id}/acknowledgements`
/// with the plan version, then `POST /api/v1/delivery/trips/{id}/start` with an Idempotency-Key.
/// Both need a connection; the server does not accept a trip start through the sync endpoint.
class ApiTripStarter implements TripStarter {
  ApiTripStarter({required http.Client client, required String baseUrl, required AuthGateway auth, this.timeout = const Duration(seconds: 20)})
      : _client = client,
        _auth = auth,
        _baseUrl = baseUrl.replaceAll(RegExp(r'/+$'), '');

  final http.Client _client;
  final AuthGateway _auth;
  final String _baseUrl;
  final Duration timeout;

  @override
  Future<TripStartResult> start(TripInfo trip, {required String operationId}) async {
    if (trip.started) return const TripStartResult(TripStartStatus.started);
    final token = await _auth.accessToken();
    if (token == null) return const TripStartResult(TripStartStatus.signInNeeded, 'Your sign-in expired. Sign in again.');
    if (trip.planId.isEmpty) {
      return const TripStartResult(TripStartStatus.refused, 'This trip has no plan to acknowledge, so it cannot be started from the phone.');
    }
    try {
      final ack = await _post(
        '/api/v1/planning/plans/${Uri.encodeComponent(trip.planId)}/acknowledgements',
        token,
        body: {'version': trip.planVersion},
      );
      final ackFailure = _failure(ack, whatFailed: 'acknowledge the plan');
      if (ackFailure != null) return ackFailure;
      final started = await _post(
        '/api/v1/delivery/trips/${Uri.encodeComponent(trip.tripId)}/start',
        token,
        headers: {'Idempotency-Key': operationId},
        body: {'operationId': operationId},
      );
      return _failure(started, whatFailed: 'start the trip') ?? const TripStartResult(TripStartStatus.started);
    } on SocketException {
      return const TripStartResult(TripStartStatus.offline);
    } on TimeoutException {
      return const TripStartResult(TripStartStatus.offline);
    } on http.ClientException {
      return const TripStartResult(TripStartStatus.offline);
    }
  }

  Future<http.Response> _post(String path, String token, {Map<String, String> headers = const {}, required Map<String, Object?> body}) {
    return _client
        .post(
          Uri.parse('$_baseUrl$path'),
          headers: {'Authorization': 'Bearer $token', 'Content-Type': 'application/json', 'Accept': 'application/json', ...headers},
          body: jsonEncode(body),
        )
        .timeout(timeout);
  }

  TripStartResult? _failure(http.Response response, {required String whatFailed}) {
    final code = response.statusCode;
    if (code >= 200 && code < 300) return null;
    if (code == 401) return const TripStartResult(TripStartStatus.signInNeeded, 'Your sign-in was not accepted. Sign in again.');
    if (code >= 500) return const TripStartResult(TripStartStatus.offline);
    if (code == 403) return TripStartResult(TripStartStatus.refused, 'This account is not allowed to $whatFailed.');
    if (code == 409) {
      return TripStartResult(TripStartStatus.refused, 'The plan changed, so the trip could not start. Reload your route and read the new instructions. ${_detail(response)}'.trim());
    }
    return TripStartResult(TripStartStatus.refused, 'Waypoint could not $whatFailed (HTTP $code). ${_detail(response)}'.trim());
  }

  String _detail(http.Response response) {
    try {
      final body = jsonDecode(response.body);
      if (body is Map && body['detail'] is String) return body['detail'] as String;
    } on FormatException {
      // No readable detail.
    }
    return '';
  }
}
