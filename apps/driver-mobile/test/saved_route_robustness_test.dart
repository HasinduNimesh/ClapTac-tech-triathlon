import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:waypoint_driver/app/driver_session.dart';
import 'package:waypoint_driver/auth/auth_gateway.dart';
import 'package:waypoint_driver/auth/profile_api.dart';
import 'package:waypoint_driver/data/driver_models.dart';
import 'package:waypoint_driver/offline/local_database.dart';
import 'package:waypoint_driver/sync/sync.dart';
import 'package:waypoint_driver/trips/route_store.dart';
import 'package:waypoint_driver/trips/trip_source.dart';

const _driver = DriverProfile(userId: 'USR006', subject: 'usr-driver', roles: ['DRIVER'], vehicleId: 'VEH001');
const _stop = StopInfo(stopId: 'stop-1', sequence: 1, outletCode: 'O1', name: 'Dehiwala', windowStart: '', windowEnd: '', units: 10, accessNote: '', contactNote: '', goods: 'G');
TripInfo _trip({int planVersion = 1, String runStatus = 'prepared'}) =>
    TripInfo(tripId: 'trip-1', runId: 'run-1', vehicleCode: 'VEH001', tripRef: 'PLAN1', depot: 'D', window: '', stops: const [_stop], planVersion: planVersion, runStatus: runStatus);

Map<String, Object?> _validTrip() => jsonDecode(jsonEncode(_trip().toJson())) as Map<String, Object?>;

class _Auth implements AuthGateway {
  @override
  Future<AuthOutcome> signIn() async => const AuthOutcome.signedIn(_driver);
  @override
  Future<DriverProfile?> restore() async => _driver;
  @override
  Future<void> signOut() async {}
  @override
  Future<String?> accessToken() async => 'tok';
}

class _Trips implements TripSource {
  _Trips(this.result);
  final TripLoad result;
  @override
  Future<TripLoad> loadToday() async => result;
}

void main() {
  late Directory root;
  late FileRouteStore store;
  setUp(() async {
    root = await Directory.systemTemp.createTemp('waypoint-route-robust-');
    store = FileRouteStore(directory: () async => Directory('${root.path}/routes'));
  });
  tearDown(() => root.delete(recursive: true));

  List<String> filesLeft() => Directory('${root.path}/routes').existsSync()
      ? Directory('${root.path}/routes').listSync().map((e) => e.path.split('/').last).toList()
      : <String>[];

  group('writes that overlap', () {
    test('rapid successive writes end with the last one and leave no temporary file', () async {
      final writes = <Future<void>>[];
      for (var version = 1; version <= 25; version++) {
        writes.add(store.write('USR006', SavedRoute(businessDate: '2026-10-04', trip: _trip(planVersion: version))));
      }
      await Future.wait(writes);
      expect((await store.read('USR006'))?.trip.planVersion, 25, reason: 'an older snapshot must never win');
      expect(filesLeft(), [FileRouteStore.fileName('USR006')]);
    });

    test('a clear that follows a write that has not finished still leaves nothing behind', () async {
      final pending = store.write('USR006', SavedRoute(businessDate: '2026-10-04', trip: _trip()));
      await store.clear('USR006');
      await pending;
      expect(await store.read('USR006'), isNull, reason: 'a pending write must not bring the route back after sign-out');
      expect(filesLeft(), isEmpty);
    });

    test('a write after a clear is kept', () async {
      unawaited(store.write('USR006', SavedRoute(businessDate: '2026-10-04', trip: _trip(planVersion: 1))));
      unawaited(store.clear('USR006'));
      await store.write('USR006', SavedRoute(businessDate: '2026-10-04', trip: _trip(planVersion: 2)));
      expect((await store.read('USR006'))?.trip.planVersion, 2);
    });

    test('one driver\'s writes do not wait on another\'s', () async {
      await Future.wait([
        store.write('USR006', SavedRoute(businessDate: '2026-10-04', trip: _trip(planVersion: 1))),
        store.write('USR007', SavedRoute(businessDate: '2026-10-04', trip: _trip(planVersion: 2))),
      ]);
      expect((await store.read('USR006'))?.trip.planVersion, 1);
      expect((await store.read('USR007'))?.trip.planVersion, 2);
    });

    test('a failed write does not stop the ones after it', () async {
      // The folder cannot be created because a file is in the way.
      await File('${root.path}/routes').writeAsString('not a folder');
      await expectLater(store.write('USR006', SavedRoute(businessDate: '2026-10-04', trip: _trip())), throwsA(anything));
      await File('${root.path}/routes').delete();
      await store.write('USR006', SavedRoute(businessDate: '2026-10-04', trip: _trip(planVersion: 9)));
      expect((await store.read('USR006'))?.trip.planVersion, 9);
    });

    test('signing out while a save is still being written leaves no route behind', () async {
      final session = DriverSession(
        database: InMemoryLocalDatabase(),
        queue: InMemorySyncQueue(),
        auth: _Auth(),
        trips: _Trips(TripLoad.loaded(_trip(runStatus: 'in_progress'))),
        routeStore: store,
      );
      addTearDown(session.dispose);
      await session.signIn();
      // A stop is recorded (which starts a save that is not awaited) and the driver signs out at once.
      await session.markArrived(_stop);
      unawaited(session.recordDelivery(_stop, const DeliveryDraft(outcome: DeliveryOutcome.failed, reason: 'OUTLET_CLOSED')));
      await session.finishTrip();
      await Future<void>.delayed(const Duration(milliseconds: 100));
      expect(await store.read('USR006'), isNull);
      expect(filesLeft(), isEmpty);
    });
  });

  group('a damaged saved route', () {
    final damaged = <String, void Function(Map<String, Object?>)>{
      'completedStopIds is a string': (j) => j['completedStopIds'] = 'bad',
      'completedStopIds holds a map': (j) => j['completedStopIds'] = [<String, Object?>{}],
      'tripId is a number': (j) => j['tripId'] = 5,
      'stops is a string': (j) => j['stops'] = 'x',
      'a stop is not an object': (j) => j['stops'] = ['x'],
      'planVersion is text': (j) => j['planVersion'] = 'a',
      'completedStops is text': (j) => j['completedStops'] = 'many',
      'runStatus is a list': (j) => j['runStatus'] = <Object?>[],
      'a stop sequence is text': (j) => ((j['stops']! as List).first as Map<String, Object?>)['sequence'] = '1',
      'a stop quantity is text': (j) => ((j['stops']! as List).first as Map<String, Object?>)['units'] = 'ten',
      'a stop name is a number': (j) => ((j['stops']! as List).first as Map<String, Object?>)['name'] = 7,
    };

    for (final entry in damaged.entries) {
      test('${entry.key}: the model refuses it as a format error', () {
        final json = _validTrip();
        entry.value(json);
        expect(() => TripInfo.fromJson(json), throwsFormatException);
      });

      test('${entry.key}: reading it gives no saved route instead of an exception', () async {
        final json = _validTrip();
        entry.value(json);
        await Directory('${root.path}/routes').create(recursive: true);
        await File('${root.path}/routes/${FileRouteStore.fileName('USR006')}').writeAsString(jsonEncode({'businessDate': '2026-10-04', 'trip': json}));
        expect(await store.read('USR006'), isNull);
      });
    }

    test('the business date being the wrong type is also just no saved route', () async {
      await Directory('${root.path}/routes').create(recursive: true);
      await File('${root.path}/routes/${FileRouteStore.fileName('USR006')}').writeAsString(jsonEncode({'businessDate': 20261004, 'trip': _validTrip()}));
      expect(await store.read('USR006'), isNull);
    });

    test('a valid route still reads back, including numbers written as whole doubles', () async {
      final json = _validTrip();
      json['planVersion'] = 3.0; // a JSON encoder may write a whole number this way
      final trip = TripInfo.fromJson(json);
      expect(trip.planVersion, 3);
    });

    test('opening the app with a damaged saved route does not crash, it behaves as having none', () async {
      final json = _validTrip()..['completedStopIds'] = 'bad';
      await Directory('${root.path}/routes').create(recursive: true);
      await File('${root.path}/routes/${FileRouteStore.fileName('USR006')}').writeAsString(jsonEncode({'businessDate': ApiTripSourceToday.key(), 'trip': json}));
      final session = DriverSession(
        database: InMemoryLocalDatabase(),
        queue: InMemorySyncQueue(),
        auth: _Auth(),
        trips: _Trips(const TripLoad.failed('Could not reach Waypoint. Check your connection and try again.')),
        routeStore: store,
      );
      addTearDown(session.dispose);
      await session.restore();
      expect(session.hasRoute, isFalse);
      expect(session.showingSavedRoute, isFalse);
      expect(session.tripsError, contains('Could not reach Waypoint'));
    });
  });
}

class ApiTripSourceToday {
  static String key() => ApiTripSource.dateKey(DateTime.now());
}
