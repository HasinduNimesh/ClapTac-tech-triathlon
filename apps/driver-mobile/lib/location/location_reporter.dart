import 'dart:async';
import 'dart:math' as math;

import 'package:flutter/foundation.dart';

/// One position fix from the phone.
class Fix {
  const Fix({required this.latitude, required this.longitude, required this.at});

  final double latitude;
  final double longitude;
  final DateTime at;
}

enum LocationAccess { granted, denied, deniedForever, serviceOff }

/// Where positions come from. The phone's location service in the app; a fake in tests.
abstract class PositionSource {
  /// Whether the app may read the position right now, without asking.
  Future<LocationAccess> access();

  /// Asks the driver for permission (the system prompt) and returns the result.
  Future<LocationAccess> requestAccess();

  /// Fixes while a trip runs. The stream keeps the app alive with a visible notification, so sharing
  /// continues with the screen off.
  Stream<Fix> positions();
}

enum SendResult {
  /// Accepted.
  sent,

  /// Not accepted now (no signal, a server error). The next fix tries again.
  retry,

  /// The server says this trip no longer takes positions (finished, or not this driver's). Stop for this trip.
  stop,
}

/// Where positions go. The Waypoint API in the app; a fake in tests.
abstract class PositionSink {
  Future<SendResult> send(String tripId, Fix fix);
}

enum LocationShareState {
  /// Not tracking: no trip is running.
  off,

  /// A trip is running and the driver has not been asked yet. The screen explains and asks.
  askFirst,

  /// The driver said not now. The trip works as usual.
  declined,

  /// The phone's permission or location service is off. The trip works as usual.
  blocked,

  /// Sending the truck's position to dispatch.
  sharing,
}

/// Shares the truck's position with dispatch while a trip is running, and only then.
///
/// It is best effort and never gets in the driver's way: a trip starts, runs and finishes the same without
/// it. Fixes are thinned so the battery and the data plan are not spent: a fix is sent when the vehicle has
/// moved, or as a heartbeat when it has not. Nothing is queued; only the newest fix is worth sending, and
/// the server refuses points older than five minutes anyway.
class LocationReporter extends ChangeNotifier {
  LocationReporter({
    required this.source,
    required this.sink,
    DateTime Function()? clock,
    this.minInterval = const Duration(seconds: 30),
    this.heartbeat = const Duration(seconds: 90),
    this.minMetres = 25,
    this.maxAge = const Duration(minutes: 4),
  }) : _clock = clock ?? DateTime.now;

  final PositionSource source;
  final PositionSink sink;
  final DateTime Function() _clock;
  final Duration minInterval;
  final Duration heartbeat;
  final double minMetres;
  final Duration maxAge;

  LocationShareState _state = LocationShareState.off;
  String? _tripId;
  StreamSubscription<Fix>? _subscription;
  Fix? _lastSent;
  DateTime? _lastSentAt;
  bool _sending = false;
  bool _declinedThisTrip = false;
  int _generation = 0;

  LocationShareState get state => _state;
  String? get tripId => _tripId;
  bool get sharing => _state == LocationShareState.sharing;

  void _set(LocationShareState next) {
    if (_state == next) return;
    _state = next;
    notifyListeners();
  }

  /// A trip is running: share if the driver already allowed it on this phone, otherwise ask first. Safe to
  /// call again for the same trip.
  Future<void> startFor(String tripId) async {
    if (_tripId == tripId && _state != LocationShareState.off) return;
    await stop();
    _tripId = tripId;
    _declinedThisTrip = false;
    final generation = ++_generation;
    final access = await source.access();
    if (generation != _generation) return;
    switch (access) {
      case LocationAccess.granted:
        _listen(tripId, generation);
      case LocationAccess.denied:
        _set(LocationShareState.askFirst);
      case LocationAccess.deniedForever:
      case LocationAccess.serviceOff:
        _set(LocationShareState.blocked);
    }
  }

  /// The driver tapped Allow on the explanation: ask the system and start if it says yes.
  Future<void> allow() async {
    final tripId = _tripId;
    if (tripId == null) return;
    final generation = _generation;
    final access = await source.requestAccess();
    if (generation != _generation) return;
    if (access == LocationAccess.granted) {
      _listen(tripId, generation);
    } else {
      _set(access == LocationAccess.denied ? LocationShareState.declined : LocationShareState.blocked);
    }
  }

  /// The driver tapped Not now.
  void decline() {
    if (_state != LocationShareState.askFirst) return;
    _declinedThisTrip = true;
    _set(LocationShareState.declined);
  }

  bool get declinedThisTrip => _declinedThisTrip;

  /// Stops sharing: the trip ended, the driver signed out, or the server stopped taking positions.
  Future<void> stop() async {
    _generation++;
    final subscription = _subscription;
    _subscription = null;
    await subscription?.cancel();
    _tripId = null;
    _lastSent = null;
    _lastSentAt = null;
    _sending = false;
    _set(LocationShareState.off);
  }

  void _listen(String tripId, int generation) {
    _subscription?.cancel();
    _subscription = source.positions().listen(
      (fix) => unawaited(_onFix(tripId, generation, fix)),
      onError: (Object _) {
        // The phone's location service went away mid-trip. Nothing to show the driver except that sharing paused.
        if (generation == _generation) _set(LocationShareState.blocked);
      },
    );
    _set(LocationShareState.sharing);
  }

  /// Whether this fix is worth sending now.
  @visibleForTesting
  bool worthSending(Fix fix, DateTime now) {
    if (now.difference(fix.at) > maxAge) return false;
    final last = _lastSent;
    final lastAt = _lastSentAt;
    if (last == null || lastAt == null) return true;
    final since = now.difference(lastAt);
    if (since >= heartbeat) return true;
    return since >= minInterval && metresBetween(last, fix) >= minMetres;
  }

  Future<void> _onFix(String tripId, int generation, Fix fix) async {
    if (generation != _generation || _sending) return;
    final now = _clock();
    if (!worthSending(fix, now)) return;
    _sending = true;
    try {
      final result = await sink.send(tripId, fix);
      if (generation != _generation) return;
      switch (result) {
        case SendResult.sent:
          _lastSent = fix;
          _lastSentAt = now;
        case SendResult.retry:
          break;
        case SendResult.stop:
          await stop();
      }
    } finally {
      if (generation == _generation) _sending = false;
    }
  }
}

/// Straight-line distance between two fixes in metres.
double metresBetween(Fix a, Fix b) {
  const rad = 0.017453292519943295;
  final dLat = (b.latitude - a.latitude) * rad;
  final dLon = (b.longitude - a.longitude) * rad;
  final h = math.pow(math.sin(dLat / 2), 2) + math.cos(a.latitude * rad) * math.cos(b.latitude * rad) * math.pow(math.sin(dLon / 2), 2);
  return 2 * 6371000.0 * math.asin(math.sqrt(h < 1 ? h : 1));
}
