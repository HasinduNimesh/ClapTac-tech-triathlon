import 'dart:async';
import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:sqflite_common_ffi/sqflite_ffi.dart';
import 'package:waypoint_driver/app/driver_session.dart';
import 'package:waypoint_driver/auth/auth_gateway.dart';
import 'package:waypoint_driver/auth/profile_api.dart';
import 'package:waypoint_driver/data/driver_models.dart';
import 'package:waypoint_driver/offline/local_database.dart';
import 'package:waypoint_driver/sync/sqlite_sync_queue.dart';
import 'package:waypoint_driver/sync/sync.dart';
import 'package:waypoint_driver/sync/sync_worker.dart';
import 'package:waypoint_driver/trips/trip_source.dart';

const _driver = DriverProfile(userId: 'USR006', subject: 'usr-driver', roles: ['DRIVER'], vehicleId: 'VEH001');
const _stop = StopInfo(stopId: 'stop-1', sequence: 1, outletCode: 'O1', name: 'Dehiwala', windowStart: '', windowEnd: '', units: 10, accessNote: '', contactNote: '', goods: 'G');
const _trip = TripInfo(tripId: 'trip-1', runId: 'run-1', vehicleCode: 'VEH001', tripRef: 'PLAN1', depot: 'D', window: '', stops: [_stop], runStatus: 'prepared');
const _unreachable = TripLoad.failed('Could not reach Waypoint. Check your connection and try again.');

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

class _Source implements TripSource {
  _Source(this.results);
  final List<TripLoad> results;
  final holds = <int, Completer<void>>{};
  int calls = 0;
  @override
  Future<TripLoad> loadToday() async {
    final call = calls++;
    await holds[call]?.future;
    return results[call < results.length ? call : results.length - 1];
  }
}

Future<void> _until(bool Function() condition) async {
  final deadline = DateTime.now().add(const Duration(seconds: 3));
  while (!condition() && DateTime.now().isBefore(deadline)) {
    await Future<void>.delayed(const Duration(milliseconds: 10));
  }
}

void main() {
  sqfliteFfiInit();

  late Directory directory;
  late SqliteSyncQueue queue;

  setUp(() async {
    directory = await Directory.systemTemp.createTemp('waypoint-route-retry-');
    queue = SqliteSyncQueue(open: () => SqliteSyncQueue.openAt('${directory.path}/q.db', factory: databaseFactoryFfi));
  });
  tearDown(() => directory.delete(recursive: true));

  DriverSession session(_Source source) {
    final worker = DeliverySyncWorker(
      queue: queue,
      auth: _Auth(),
      client: MockClient((_) async => http.Response('{}', 200)),
      baseUrl: 'https://waypoint.example',
      interval: const Duration(milliseconds: 40),
    );
    final s = DriverSession(database: InMemoryLocalDatabase(), queue: queue, auth: _Auth(), trips: source, worker: worker);
    addTearDown(s.dispose);
    addTearDown(worker.stop);
    return s;
  }

  test('a route that failed to load is loaded again by itself, without tapping Try again', () async {
    final source = _Source([_unreachable, _unreachable, const TripLoad.loaded(_trip)]);
    final s = session(source);
    await s.restore();
    expect(s.hasRoute, isFalse);
    expect(s.tripsError, contains('Could not reach Waypoint'));
    await _until(() => s.hasRoute);
    expect(s.hasRoute, isTrue);
    expect(s.tripsError, isNull);
    expect(source.calls, 3);
  });

  test('the retries are quiet: no loading state, and the error stays until the route arrives', () async {
    final source = _Source([_unreachable, _unreachable, _unreachable, const TripLoad.loaded(_trip)]);
    final s = session(source);
    await s.restore();
    final errorsBeforeRoute = <String?>[];
    var sawLoading = false;
    s.addListener(() {
      if (s.tripsLoading) sawLoading = true;
      if (!s.hasRoute) errorsBeforeRoute.add(s.tripsError);
    });
    await _until(() => s.hasRoute);
    expect(s.hasRoute, isTrue);
    expect(sawLoading, isFalse, reason: 'a quiet retry does not flash the spinner');
    expect(errorsBeforeRoute, isNotEmpty);
    expect(errorsBeforeRoute.every((error) => error != null), isTrue, reason: 'the error is not cleared and re-shown on every retry');
  });

  test('a retry is not started while the last one is still waiting', () async {
    final source = _Source([_unreachable, _unreachable, const TripLoad.loaded(_trip)]);
    source.holds[1] = Completer<void>();
    final s = session(source);
    await s.restore();
    await Future<void>.delayed(const Duration(milliseconds: 300)); // several ticks go by
    expect(source.calls, 2, reason: 'one retry is waiting; the ticks do not pile up more');
    source.holds[1]!.complete();
    await _until(() => s.hasRoute);
    expect(s.hasRoute, isTrue);
  });

  test('once the route has loaded the retries stop', () async {
    final source = _Source([_unreachable, const TripLoad.loaded(_trip)]);
    final s = session(source);
    await s.restore();
    await _until(() => s.hasRoute);
    final calls = source.calls;
    await Future<void>.delayed(const Duration(milliseconds: 250));
    expect(source.calls, calls);
  });

  test('when Waypoint answers that there is no trip, there is nothing more to retry', () async {
    final source = _Source([_unreachable, const TripLoad.none()]);
    final s = session(source);
    await s.restore();
    await _until(() => source.calls >= 2 && s.tripsError == null);
    expect(s.tripsError, isNull);
    expect(s.hasRoute, isFalse);
    final calls = source.calls;
    await Future<void>.delayed(const Duration(milliseconds: 250));
    expect(source.calls, calls, reason: 'an empty answer is not a failure to retry');
  });

  test('after signing out nothing is retried', () async {
    final source = _Source([_unreachable]);
    final s = session(source);
    await s.restore();
    await s.finishTrip();
    final calls = source.calls;
    await Future<void>.delayed(const Duration(milliseconds: 250));
    expect(source.calls, calls);
  });

  test('tapping Try again still shows that it is loading', () async {
    final source = _Source([_unreachable, const TripLoad.loaded(_trip)]);
    source.holds[1] = Completer<void>();
    // No worker ticks here: only the tap loads.
    final s = DriverSession(database: InMemoryLocalDatabase(), queue: InMemorySyncQueue(), auth: _Auth(), trips: source);
    addTearDown(s.dispose);
    await s.restore();
    final loading = s.loadTrips();
    expect(s.tripsLoading, isTrue);
    expect(s.tripsError, isNull);
    source.holds[1]!.complete();
    await loading;
    expect(s.tripsLoading, isFalse);
    expect(s.hasRoute, isTrue);
  });
}
