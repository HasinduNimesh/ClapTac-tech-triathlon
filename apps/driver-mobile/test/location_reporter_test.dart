import 'dart:async';
import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:waypoint_driver/auth/auth_failure.dart';
import 'package:waypoint_driver/auth/auth_gateway.dart';
import 'package:waypoint_driver/auth/profile_api.dart';
import 'package:waypoint_driver/location/api_position_sink.dart';
import 'package:waypoint_driver/location/location_reporter.dart';

class _Source implements PositionSource {
  LocationAccess current = LocationAccess.granted;
  LocationAccess afterAsking = LocationAccess.granted;
  int asked = 0;
  var controller = StreamController<Fix>.broadcast();

  @override
  Future<LocationAccess> access() async => current;
  @override
  Future<LocationAccess> requestAccess() async {
    asked++;
    current = afterAsking;
    return afterAsking;
  }

  @override
  Stream<Fix> positions() => controller.stream;
}

class _Sink implements PositionSink {
  final sent = <(String, Fix)>[];
  SendResult next = SendResult.sent;
  Completer<void>? hold;

  @override
  Future<SendResult> send(String tripId, Fix fix) async {
    await hold?.future;
    sent.add((tripId, fix));
    return next;
  }
}

final _t0 = DateTime.utc(2026, 10, 7, 9, 0, 0);

Fix _fix(DateTime at, {double lat = 6.9271, double lon = 79.8612}) => Fix(latitude: lat, longitude: lon, at: at);

// 0.001 degrees of latitude is about 111 m.
const _step = 0.001;

void main() {
  late _Source source;
  late _Sink sink;
  late DateTime now;
  late LocationReporter reporter;

  setUp(() {
    source = _Source();
    sink = _Sink();
    now = _t0;
    reporter = LocationReporter(source: source, sink: sink, clock: () => now);
  });

  tearDown(() => reporter.stop());

  Future<void> emit(Fix fix) async {
    source.controller.add(fix);
    await Future<void>.delayed(Duration.zero);
    await Future<void>.delayed(Duration.zero);
  }

  group('starting', () {
    test('shares at once when the phone already allows it', () async {
      await reporter.startFor('trip-1');
      expect(reporter.state, LocationShareState.sharing);
      expect(source.asked, 0);
    });

    test('asks first when permission has not been given, and shares after the driver allows', () async {
      source.current = LocationAccess.denied;
      await reporter.startFor('trip-1');
      expect(reporter.state, LocationShareState.askFirst);
      expect(source.asked, 0, reason: 'the system prompt waits for the driver to say yes on the explanation');
      await reporter.allow();
      expect(source.asked, 1);
      expect(reporter.state, LocationShareState.sharing);
    });

    test('a driver who says not now keeps working without sharing', () async {
      source.current = LocationAccess.denied;
      await reporter.startFor('trip-1');
      reporter.decline();
      expect(reporter.state, LocationShareState.declined);
      await emit(_fix(_t0));
      expect(sink.sent, isEmpty);
    });

    test('a refusal at the system prompt, or location switched off, is shown as blocked or declined, never as sharing', () async {
      source.current = LocationAccess.denied;
      source.afterAsking = LocationAccess.deniedForever;
      await reporter.startFor('trip-1');
      await reporter.allow();
      expect(reporter.state, LocationShareState.blocked);

      source.current = LocationAccess.serviceOff;
      await reporter.startFor('trip-2');
      expect(reporter.state, LocationShareState.blocked);
    });

    test('starting again for the same trip changes nothing', () async {
      await reporter.startFor('trip-1');
      await emit(_fix(_t0));
      await reporter.startFor('trip-1');
      expect(reporter.state, LocationShareState.sharing);
      expect(sink.sent, hasLength(1));
    });
  });

  group('what is sent', () {
    test('the first fix goes, and carries the trip it belongs to', () async {
      await reporter.startFor('trip-1');
      await emit(_fix(_t0));
      expect(sink.sent.single.$1, 'trip-1');
    });

    test('fixes inside the interval, or without movement, are thinned out', () async {
      await reporter.startFor('trip-1');
      await emit(_fix(_t0));
      now = _t0.add(const Duration(seconds: 10));
      await emit(_fix(now, lat: 6.9271 + _step));
      expect(sink.sent, hasLength(1), reason: 'moved, but too soon');
      now = _t0.add(const Duration(seconds: 40));
      await emit(_fix(now, lat: 6.9271 + 0.00001));
      expect(sink.sent, hasLength(1), reason: 'long enough, but it barely moved');
    });

    test('movement after the interval is sent', () async {
      await reporter.startFor('trip-1');
      await emit(_fix(_t0));
      now = _t0.add(const Duration(seconds: 35));
      await emit(_fix(now, lat: 6.9271 + _step));
      expect(sink.sent, hasLength(2));
    });

    test('a parked truck still sends a heartbeat so dispatch knows the phone is alive', () async {
      await reporter.startFor('trip-1');
      await emit(_fix(_t0));
      now = _t0.add(const Duration(seconds: 95));
      await emit(_fix(now));
      expect(sink.sent, hasLength(2));
    });

    test('a fix that is already too old is dropped, because the server would refuse it', () async {
      await reporter.startFor('trip-1');
      now = _t0.add(const Duration(minutes: 10));
      await emit(_fix(_t0));
      expect(sink.sent, isEmpty);
    });

    test('a failed send is retried with the next fix instead of counting as sent', () async {
      await reporter.startFor('trip-1');
      sink.next = SendResult.retry;
      await emit(_fix(_t0));
      sink.next = SendResult.sent;
      now = _t0.add(const Duration(seconds: 5));
      await emit(_fix(now));
      expect(sink.sent, hasLength(2), reason: 'the second fix is not thinned because the first never got through');
      expect(reporter.state, LocationShareState.sharing);
    });

    test('only one send is in flight at a time', () async {
      await reporter.startFor('trip-1');
      sink.hold = Completer<void>();
      await emit(_fix(_t0));
      await emit(_fix(_t0.add(const Duration(seconds: 1)), lat: 7.0));
      sink.hold!.complete();
      await Future<void>.delayed(Duration.zero);
      expect(sink.sent, hasLength(1));
    });
  });

  group('stopping', () {
    test('the server saying the trip no longer takes positions stops sharing for that trip', () async {
      await reporter.startFor('trip-1');
      sink.next = SendResult.stop;
      await emit(_fix(_t0));
      expect(reporter.state, LocationShareState.off);
      await emit(_fix(_t0.add(const Duration(minutes: 3)), lat: 7.0));
      expect(sink.sent, hasLength(1));
    });

    test('stop ends it at once: no more positions after the trip ends or the driver signs out', () async {
      await reporter.startFor('trip-1');
      await reporter.stop();
      expect(reporter.state, LocationShareState.off);
      expect(reporter.tripId, isNull);
      await emit(_fix(_t0));
      expect(sink.sent, isEmpty);
    });

    test('a new trip starts with a clean slate, so its first fix is never thinned by the old trip', () async {
      await reporter.startFor('trip-1');
      await emit(_fix(_t0));
      source.controller = StreamController<Fix>.broadcast();
      await reporter.startFor('trip-2');
      now = _t0.add(const Duration(seconds: 5));
      await emit(_fix(now));
      expect(sink.sent.map((e) => e.$1), ['trip-1', 'trip-2']);
    });

    test('the location service failing mid-trip pauses sharing without throwing', () async {
      await reporter.startFor('trip-1');
      source.controller.addError(Exception('service off'));
      await Future<void>.delayed(Duration.zero);
      expect(reporter.state, LocationShareState.blocked);
    });
  });

  test('metresBetween is about 111 m for 0.001 degrees of latitude', () {
    expect(metresBetween(_fix(_t0), _fix(_t0, lat: 6.9271 + _step)), closeTo(111.2, 1));
  });

  group('ApiPositionSink', () {
    final fix = _fix(DateTime.utc(2026, 10, 7, 9, 0, 0), lat: 6.9271, lon: 79.8612);

    ApiPositionSink sinkWith(http.Client client, {String? token = 'tok'}) =>
        ApiPositionSink(client: client, baseUrl: 'https://waypoint.example/', auth: _Auth(token));

    test('posts the position to the trip with the bearer token', () async {
      late http.Request seen;
      final api = sinkWith(MockClient((r) async {
        seen = r;
        return http.Response('{}', 200);
      }));
      expect(await api.send('trip 1', fix), SendResult.sent);
      expect(seen.method, 'POST');
      expect(seen.url.toString(), 'https://waypoint.example/api/v1/delivery/trips/trip%201/location');
      expect(seen.headers['Authorization'], 'Bearer tok');
      expect(jsonDecode(seen.body), {'latitude': 6.9271, 'longitude': 79.8612, 'timestamp': '2026-10-07T09:00:00.000Z'});
    });

    test('a trip that no longer takes positions stops sharing; anything else is a retry', () async {
      for (final code in [403, 404, 409]) {
        expect(await sinkWith(MockClient((_) async => http.Response('', code))).send('t', fix), SendResult.stop, reason: '$code');
      }
      for (final code in [400, 401, 429, 500, 503]) {
        expect(await sinkWith(MockClient((_) async => http.Response('', code))).send('t', fix), SendResult.retry, reason: '$code');
      }
    });

    test('no signal, a timeout or no sign-in is a retry and never throws', () async {
      expect(await sinkWith(MockClient((_) async => throw http.ClientException('offline'))).send('t', fix), SendResult.retry);
      expect(await sinkWith(MockClient((_) async => throw TimeoutException('slow'))).send('t', fix), SendResult.retry);
      expect(await sinkWith(MockClient((_) async => http.Response('', 200)), token: null).send('t', fix), SendResult.retry);
    });
  });
}

class _Auth implements AuthGateway {
  _Auth(this.token);
  final String? token;
  @override
  Future<AuthOutcome> signIn() async => const AuthOutcome.failed(AuthFailure(AuthFailureKind.cancelled));
  @override
  Future<DriverProfile?> restore() async => null;
  @override
  Future<void> signOut() async {}
  @override
  Future<String?> accessToken() async => token;
}
