import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:http/http.dart' as http;

import '../data/driver_models.dart';

enum TripsFailureKind { unauthorized, forbidden, notFound, unavailable, serverProblem }

class TripsFailure implements Exception {
  const TripsFailure(this.kind, [this.status]);

  final TripsFailureKind kind;

  /// The HTTP status Waypoint answered with, for [TripsFailureKind.serverProblem].
  final int? status;

  String get message {
    switch (kind) {
      case TripsFailureKind.unauthorized:
        return 'Your sign-in was not accepted. Sign in again.';
      case TripsFailureKind.forbidden:
        return 'This account cannot load a route. It needs a vehicle assigned in Waypoint.';
      case TripsFailureKind.notFound:
        return 'That trip could not be found.';
      case TripsFailureKind.unavailable:
        return 'Could not reach Waypoint. Check your connection and try again.';
      case TripsFailureKind.serverProblem:
        // Waypoint answered, so the phone's connection is fine; blaming it would send the driver hunting for signal.
        return 'Waypoint had a problem loading your route${status == null ? '' : ' (error $status)'}. Try again in a minute. If it keeps happening, tell dispatch.';
    }
  }

  @override
  String toString() => 'TripsFailure($kind)';
}

/// One entry of `GET /api/v1/delivery/drivers/me/trips?date=`.
class TripSummary {
  const TripSummary({required this.tripId, required this.status, required this.tripNumber});

  factory TripSummary.fromJson(Map<String, Object?> json) => TripSummary(
        tripId: json['tripId'] as String? ?? '',
        status: json['status'] as String? ?? '',
        tripNumber: (json['tripNumber'] as num?)?.toInt() ?? 0,
      );

  final String tripId;

  /// "ready" (loaded, no run yet), or the run status: prepared, in_progress, completed.
  final String status;
  final int tripNumber;

  bool get isCompleted => status == 'completed';
}

class TripsApi {
  TripsApi({required http.Client client, required String baseUrl, this.timeout = const Duration(seconds: 20)})
      : _client = client,
        _baseUrl = baseUrl.replaceAll(RegExp(r'/+$'), '');

  final http.Client _client;
  final String _baseUrl;
  final Duration timeout;

  /// The driver's trips for [date] (`yyyy-MM-dd`). The server decides which vehicle that is.
  Future<List<TripSummary>> tripsFor(String date, String accessToken) async {
    final body = await _get('/api/v1/delivery/drivers/me/trips?date=$date', accessToken);
    final items = body is Map<String, Object?> ? body['items'] : null;
    if (items is! List) throw const TripsFailure(TripsFailureKind.unavailable);
    return [for (final item in items) if (item is Map<String, Object?>) TripSummary.fromJson(item)];
  }

  /// A trip with its stops. For a driver the server prepares the run on the first read.
  Future<TripInfo> trip(String tripId, String accessToken) async {
    final body = await _get('/api/v1/delivery/trips/${Uri.encodeComponent(tripId)}', accessToken);
    if (body is! Map<String, Object?>) throw const TripsFailure(TripsFailureKind.unavailable);
    try {
      return tripFromJson(body);
    } on FormatException {
      throw const TripsFailure(TripsFailureKind.unavailable);
    }
  }

  Future<Object?> _get(String path, String accessToken) async {
    final http.Response response;
    try {
      response = await _client.get(
        Uri.parse('$_baseUrl$path'),
        headers: {'Authorization': 'Bearer $accessToken', 'Accept': 'application/json'},
      ).timeout(timeout);
    } on SocketException {
      throw const TripsFailure(TripsFailureKind.unavailable);
    } on TimeoutException {
      throw const TripsFailure(TripsFailureKind.unavailable);
    } on http.ClientException {
      throw const TripsFailure(TripsFailureKind.unavailable);
    }
    switch (response.statusCode) {
      case 200:
        try {
          return jsonDecode(response.body);
        } on FormatException {
          throw const TripsFailure(TripsFailureKind.unavailable);
        }
      case 401:
        throw const TripsFailure(TripsFailureKind.unauthorized);
      case 403:
        throw const TripsFailure(TripsFailureKind.forbidden);
      case 404:
        throw const TripsFailure(TripsFailureKind.notFound);
      default:
        if (response.statusCode >= 500) throw TripsFailure(TripsFailureKind.serverProblem, response.statusCode);
        throw const TripsFailure(TripsFailureKind.unavailable);
    }
  }
}

/// Maps the trip detail (`{tripId, run, stops}`) to what the screens show. Throws [FormatException]
/// when the ids the app needs to send operations are missing.
TripInfo tripFromJson(Map<String, Object?> json) {
  final run = json['run'];
  final stopsJson = json['stops'];
  if (run is! Map<String, Object?> || stopsJson is! List) throw const FormatException('trip detail');
  final tripId = json['tripId'] as String? ?? run['tripId'] as String? ?? '';
  final runId = run['id'] as String? ?? '';
  if (tripId.isEmpty || runId.isEmpty) throw const FormatException('trip or run id');

  final stops = [
    for (final item in stopsJson)
      if (item is Map<String, Object?>) _stopFromJson(item),
  ]..sort((a, b) => a.sequence.compareTo(b.sequence));
  if (stops.any((stop) => stop.stopId.isEmpty)) throw const FormatException('stop id');

  final opens = [for (final stop in stops) if (stop.windowStart.isNotEmpty) stop.windowStart]..sort();
  final closes = [for (final stop in stops) if (stop.windowEnd.isNotEmpty) stop.windowEnd]..sort();

  // Stops the server already finished (a resumed run) count as done, in order.
  var done = 0;
  final completedStopIds = <String>{};
  for (final item in stopsJson) {
    if (item is Map<String, Object?> && item['status'] == 'completed') {
      done++;
      if (item['id'] is String) completedStopIds.add(item['id'] as String);
    }
  }

  return TripInfo(
    tripId: tripId,
    runId: runId,
    vehicleCode: run['vehicleId'] as String? ?? '',
    tripRef: _tripRef(run, tripId),
    depot: run['depot'] as String? ?? '',
    window: opens.isEmpty || closes.isEmpty ? '' : '${opens.first}-${closes.last}',
    stops: stops,
    completedStops: done,
    completedStopIds: completedStopIds,
    planId: run['planId'] as String? ?? '',
    planVersion: (json['currentPlanVersion'] as num?)?.toInt() ?? (run['planVersion'] as num?)?.toInt() ?? 0,
    runStatus: run['status'] as String? ?? json['status'] as String? ?? '',
  );
}

/// What the driver sees as the trip's name: the plan reference (PLAN000002), not the uuid.
String _tripRef(Map<String, Object?> run, String tripId) {
  final plan = (run['planRef'] as String?)?.trim() ?? '';
  if (plan.isNotEmpty) return plan;
  return tripId.length > 8 ? tripId.substring(0, 8) : tripId;
}

StopInfo _stopFromJson(Map<String, Object?> json) {
  String text(String key) => (json[key] as String?)?.trim() ?? '';
  String clock(String key) {
    final value = text(key);
    return value.length >= 5 ? value.substring(0, 5) : value;
  }

  final outletId = text('outletId');
  final name = text('outletName');
  final access = text('accessInstructions');
  final site = [text('dockType'), text('parkingConstraint')].where((part) => part.isNotEmpty).join(' · ');
  final unitLabel = text('unitLabel');
  return StopInfo(
    stopId: text('id'),
    sequence: (json['stopSequence'] as num?)?.toInt() ?? 0,
    outletCode: name.isNotEmpty ? outletId : '',
    name: name.isNotEmpty ? name : (outletId.isNotEmpty ? outletId : (text('orderRef').isNotEmpty ? text('orderRef') : 'Stop ${(json['stopSequence'] as num?)?.toInt() ?? 0}')),
    windowStart: clock('plannedWindowOpen'),
    windowEnd: clock('plannedWindowClose'),
    units: (json['expectedUnits'] as num?)?.toInt(),
    unitLabel: unitLabel.isEmpty ? 'units' : unitLabel,
    accessNote: access.isEmpty ? 'No access instructions recorded' : access,
    contactNote: site,
    goods: text('temperatureRequirement').isEmpty ? 'Goods' : text('temperatureRequirement'),
    orderRef: text('orderRef'),
    orderId: text('orderId'),
    outcomeCode: text('outcomeCode'),
    deliveredUnits: (json['deliveredUnits'] as num?)?.toInt(),
    district: text('district'),
    latitude: _coordinate(json['latitude']),
    longitude: _coordinate(json['longitude']),
    locationApproximate: json['locationApproximate'] == true,
  );
}

/// A coordinate the server sent, or null when it is missing or not a number (the stop is then
/// searched for by name instead of navigated to).
double? _coordinate(Object? value) => value is num && value.isFinite ? value.toDouble() : null;
