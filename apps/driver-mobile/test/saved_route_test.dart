import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:waypoint_driver/app/driver_session.dart';
import 'package:waypoint_driver/auth/auth_gateway.dart';
import 'package:waypoint_driver/auth/profile_api.dart';
import 'package:waypoint_driver/connectivity/connectivity_monitor.dart';
import 'package:waypoint_driver/data/driver_models.dart';
import 'package:waypoint_driver/main.dart';
import 'package:waypoint_driver/offline/local_database.dart';
import 'package:waypoint_driver/sync/sync.dart';
import 'package:waypoint_driver/trips/route_store.dart';
import 'package:waypoint_driver/trips/trip_source.dart';
import 'package:waypoint_driver/trips/trip_start.dart';

import 'helpers/render.dart';

const _driver = DriverProfile(userId: 'USR006', subject: 'usr-driver', roles: ['DRIVER'], vehicleId: 'VEH001');
const _other = DriverProfile(userId: 'USR007', subject: 'usr-other', roles: ['DRIVER'], vehicleId: 'VEH002');
const _stop1 = StopInfo(stopId: 'stop-1', sequence: 1, outletCode: 'O1', name: 'Dehiwala', windowStart: '08:00', windowEnd: '09:00', units: 10, accessNote: 'Rear gate', contactNote: 'Dock', goods: 'CHILLED', orderRef: 'ORD1');
const _stop2 = StopInfo(stopId: 'stop-2', sequence: 2, outletCode: 'O2', name: 'Nugegoda', windowStart: '', windowEnd: '', accessNote: '', contactNote: '', goods: 'G');
const _trip = TripInfo(
  tripId: 'trip-1',
  runId: 'run-1',
  vehicleCode: 'VEH001',
  tripRef: 'PLAN1',
  depot: 'DEPOT_NORTH',
  window: '08:00-09:00',
  stops: [_stop1, _stop2],
  runStatus: 'prepared',
  planId: 'plan-1',
  planVersion: 3,
);
const _unreachable = TripLoad.failed('Could not reach Waypoint. Check your connection and try again.');

final _today = DateTime.utc(2026, 10, 4, 6); // 11:30 on 4 October in Colombo
final _tomorrow = DateTime.utc(2026, 10, 5, 6);

class _Auth implements AuthGateway {
  _Auth([this.profile = _driver]);
  DriverProfile profile;
  int signOuts = 0;
  @override
  Future<AuthOutcome> signIn() async => AuthOutcome.signedIn(profile);
  @override
  Future<DriverProfile?> restore() async => profile;
  @override
  Future<void> signOut() async => signOuts++;
  @override
  Future<String?> accessToken() async => 'tok';
}

class _Source implements TripSource {
  _Source(this.results);
  final List<TripLoad> results;
  int calls = 0;
  @override
  Future<TripLoad> loadToday() async => results[calls++ < results.length ? calls - 1 : results.length - 1];
}

class _Starter implements TripStarter {
  @override
  Future<TripStartResult> start(TripInfo trip, {required String operationId}) async => const TripStartResult(TripStartStatus.started);
}

class _Monitor implements ConnectivityMonitor {
  final _controller = StreamController<bool>.broadcast();
  @override
  Future<bool> isOnline() async => true;
  @override
  Stream<bool> get changes => _controller.stream;
  void set(bool value) => _controller.add(value);
}

DriverSession _session(_Source source, MemoryRouteStore store, {_Auth? auth, DateTime? now, TripStarter? starter, ConnectivityMonitor? monitor}) {
  final s = DriverSession(
    database: InMemoryLocalDatabase(),
    queue: InMemorySyncQueue(),
    auth: auth ?? _Auth(),
    trips: source,
    starter: starter,
    routeStore: store,
    connectivity: monitor,
    clock: () => now ?? _today,
  );
  addTearDown(s.dispose);
  return s;
}

Future<void> _settle() => Future<void>.delayed(const Duration(milliseconds: 40));

void main() {
  setUpAll(loadAppFonts);

  group('saving a trip as JSON', () {
    test('a trip keeps every field the screens use, including progress', () {
      const trip = TripInfo(
        tripId: 't',
        runId: 'r',
        vehicleCode: 'VEH001',
        plate: 'WP 1',
        tripRef: 'PLAN9',
        depot: 'D',
        window: '08:00-10:00',
        stops: [_stop1, _stop2],
        completedStops: 1,
        completedStopIds: {'stop-1'},
        planId: 'p',
        planVersion: 4,
        runStatus: 'in_progress',
      );
      final copy = TripInfo.fromJson(jsonDecode(jsonEncode(trip.toJson())) as Map<String, Object?>);
      expect(copy.tripId, 't');
      expect(copy.runId, 'r');
      expect(copy.plate, 'WP 1');
      expect(copy.tripRef, 'PLAN9');
      expect(copy.depot, 'D');
      expect(copy.window, '08:00-10:00');
      expect(copy.completedStops, 1);
      expect(copy.completedStopIds, {'stop-1'});
      expect(copy.planId, 'p');
      expect(copy.planVersion, 4);
      expect(copy.started, isTrue);
      expect(copy.stops.map((s) => s.stopId), ['stop-1', 'stop-2']);
      final stop = copy.stops.first;
      expect([stop.outletCode, stop.name, stop.windowStart, stop.windowEnd, stop.units, stop.unitLabel, stop.accessNote, stop.contactNote, stop.goods, stop.orderRef],
          ['O1', 'Dehiwala', '08:00', '09:00', 10, 'units', 'Rear gate', 'Dock', 'CHILLED', 'ORD1']);
    });

    test('a stop without a quantity stays without one', () {
      final copy = StopInfo.fromJson(jsonDecode(jsonEncode(_stop2.toJson())) as Map<String, Object?>);
      expect(copy.units, isNull);
      expect(copy.unitsText, 'Quantity not recorded');
    });

    test('something that is not a trip is refused rather than half-read', () {
      expect(() => TripInfo.fromJson({'stops': <Object?>[]}), throwsFormatException);
      expect(() => TripInfo.fromJson({'tripId': 't'}), throwsFormatException);
      expect(() => SavedRoute.fromJson({'businessDate': '', 'trip': _trip.toJson()}), throwsFormatException);
    });
  });

  group('FileRouteStore', () {
    late Directory root;
    late FileRouteStore store;
    setUp(() async {
      root = await Directory.systemTemp.createTemp('waypoint-route-test-');
      store = FileRouteStore(directory: () async => Directory('${root.path}/routes'));
    });
    tearDown(() => root.delete(recursive: true));

    test('keeps a route per driver and gives it back', () async {
      await store.write('USR006', const SavedRoute(businessDate: '2026-10-04', trip: _trip));
      final back = await store.read('USR006');
      expect(back?.businessDate, '2026-10-04');
      expect(back?.trip.tripId, 'trip-1');
      expect(await store.read('USR007'), isNull, reason: 'another driver on the same phone gets nothing');
    });

    test('writing again replaces it and leaves no temporary file behind', () async {
      await store.write('USR006', const SavedRoute(businessDate: '2026-10-04', trip: _trip));
      await store.write('USR006', const SavedRoute(businessDate: '2026-10-05', trip: _trip));
      expect((await store.read('USR006'))?.businessDate, '2026-10-05');
      final files = Directory('${root.path}/routes').listSync().map((e) => e.path.split('/').last).toList();
      expect(files, [FileRouteStore.fileName('USR006')]);
    });

    test('clear removes it, and clearing nothing is fine', () async {
      await store.write('USR006', const SavedRoute(businessDate: '2026-10-04', trip: _trip));
      await store.clear('USR006');
      expect(await store.read('USR006'), isNull);
      await store.clear('USR006');
    });

    test('a damaged or foreign file is no saved route, not a crash', () async {
      await Directory('${root.path}/routes').create(recursive: true);
      final file = File('${root.path}/routes/${FileRouteStore.fileName('USR006')}');
      await file.writeAsString('not json {{{');
      expect(await store.read('USR006'), isNull);
      await file.writeAsString('[1, 2, 3]');
      expect(await store.read('USR006'), isNull);
      await file.writeAsString(jsonEncode({'businessDate': '2026-10-04', 'trip': {'tripId': ''}}));
      expect(await store.read('USR006'), isNull);
    });

    test('a user id cannot reach outside the folder', () async {
      expect(FileRouteStore.fileName('../../etc/passwd'), isNot(contains('/')));
      await store.write('../escape', const SavedRoute(businessDate: '2026-10-04', trip: _trip));
      expect(File('${root.path}/escape.json').existsSync(), isFalse);
      expect((await store.read('../escape'))?.trip.tripId, 'trip-1');
    });
  });

  group('DriverSession and the saved route', () {
    test('a route that loads is saved for today with its progress', () async {
      final store = MemoryRouteStore();
      final s = _session(_Source([const TripLoad.loaded(_trip)]), store);
      await s.signIn();
      await _settle();
      final saved = store.saved['USR006']!;
      expect(saved.businessDate, '2026-10-04');
      expect(saved.trip.tripId, 'trip-1');
      expect(s.showingSavedRoute, isFalse);
    });

    test('opened with no connection, today\'s saved route is shown instead of an error', () async {
      final store = MemoryRouteStore()..saved['USR006'] = const SavedRoute(businessDate: '2026-10-04', trip: _trip);
      final s = _session(_Source([_unreachable]), store);
      await s.restore();
      expect(s.hasRoute, isTrue);
      expect(s.showingSavedRoute, isTrue);
      expect(s.tripsError, isNull);
      expect(s.trip.tripId, 'trip-1');
      expect(s.trip.stops, hasLength(2));
      expect(s.loadResolved, isFalse, reason: 'the trip was not started, so the load check still comes first');
    });

    test('a saved route for an earlier day is not shown, and is removed', () async {
      final store = MemoryRouteStore()..saved['USR006'] = const SavedRoute(businessDate: '2026-10-03', trip: _trip);
      final s = _session(_Source([_unreachable]), store);
      await s.restore();
      expect(s.hasRoute, isFalse);
      expect(s.showingSavedRoute, isFalse);
      expect(s.tripsError, contains('Could not reach Waypoint'));
      expect(store.saved, isEmpty);
    });

    test('a saved route is only for the driver it was saved for', () async {
      final store = MemoryRouteStore()..saved['USR006'] = const SavedRoute(businessDate: '2026-10-04', trip: _trip);
      final s = _session(_Source([_unreachable]), store, auth: _Auth(_other));
      await s.restore();
      expect(s.hasRoute, isFalse);
      expect(s.tripsError, isNotNull);
    });

    test('with nothing saved the error is shown as before', () async {
      final s = _session(_Source([_unreachable]), MemoryRouteStore());
      await s.restore();
      expect(s.hasRoute, isFalse);
      expect(s.tripsError, contains('Could not reach Waypoint'));
    });

    test('when Waypoint says there is no trip, the saved route is no longer valid', () async {
      final store = MemoryRouteStore()..saved['USR006'] = const SavedRoute(businessDate: '2026-10-04', trip: _trip);
      final s = _session(_Source([const TripLoad.none()]), store);
      await s.restore();
      expect(s.hasRoute, isFalse);
      expect(s.tripsError, isNull);
      expect(store.saved, isEmpty);
    });

    test('the live route replaces the saved copy as soon as it can be loaded', () async {
      final store = MemoryRouteStore()..saved['USR006'] = const SavedRoute(businessDate: '2026-10-04', trip: _trip);
      const live = TripInfo(tripId: 'trip-1', runId: 'run-1', vehicleCode: 'VEH001', tripRef: 'PLAN1', depot: 'D', window: '', stops: [_stop1, _stop2], runStatus: 'prepared', planId: 'plan-1', planVersion: 4);
      final source = _Source([_unreachable, const TripLoad.loaded(live)]);
      final s = _session(source, store);
      await s.restore();
      expect(s.showingSavedRoute, isTrue);
      expect(s.trip.planVersion, 3);
      await s.loadTrips();
      expect(s.showingSavedRoute, isFalse);
      expect(s.trip.planVersion, 4, reason: 'the live plan version, not the saved one');
      expect(store.saved['USR006']?.trip.planVersion, 4, reason: 'the saved copy is refreshed too');
    });

    test('the connection coming back loads the live route by itself', () async {
      final store = MemoryRouteStore()..saved['USR006'] = const SavedRoute(businessDate: '2026-10-04', trip: _trip);
      final monitor = _Monitor();
      final source = _Source([_unreachable, const TripLoad.loaded(_trip)]);
      final s = _session(source, store, monitor: monitor);
      await s.restore();
      expect(s.showingSavedRoute, isTrue);
      monitor.set(false);
      await _settle();
      monitor.set(true);
      await _settle();
      expect(source.calls, 2);
      expect(s.showingSavedRoute, isFalse);
    });

    test('progress made while showing the saved route is remembered after a restart', () async {
      final store = MemoryRouteStore()..saved['USR006'] = SavedRoute(businessDate: '2026-10-04', trip: _trip.withRunStatus('in_progress'));
      final first = _session(_Source([_unreachable]), store);
      await first.restore();
      await first.markArrived(_stop1);
      await first.recordDelivery(_stop1, const DeliveryDraft(outcome: DeliveryOutcome.failed, reason: 'OUTLET_CLOSED'));
      await _settle();
      expect(store.saved['USR006']?.trip.completedStopIds, {'stop-1'});

      // The app is closed and opened again, still without a connection: stop 1 is not offered again.
      final second = _session(_Source([_unreachable]), store);
      await second.restore();
      expect(second.showingSavedRoute, isTrue);
      expect(second.trip.completedStops, 1);
      expect(second.trip.nextStop?.stopId, 'stop-2');
    });

    test('a trip started on the server is remembered as started, so the load check is not asked again', () async {
      final store = MemoryRouteStore();
      final s = _session(_Source([const TripLoad.loaded(_trip)]), store, starter: _Starter());
      await s.signIn();
      await s.confirmLoad();
      await _settle();
      expect(store.saved['USR006']?.trip.started, isTrue);
      final reopened = _session(_Source([_unreachable]), store);
      await reopened.restore();
      expect(reopened.showingSavedRoute, isTrue);
      expect(reopened.loadResolved, isTrue);
      expect(reopened.tripStartState, TripStartState.started);
    });

    test('signing out removes the saved route', () async {
      final store = MemoryRouteStore();
      final s = _session(_Source([const TripLoad.loaded(_trip)]), store);
      await s.signIn();
      await _settle();
      expect(store.saved, isNotEmpty);
      await s.finishTrip();
      expect(store.saved, isEmpty);
      expect(s.showingSavedRoute, isFalse);
    });

    test('a sign-in that is over removes it too', () async {
      final store = MemoryRouteStore()..saved['USR006'] = const SavedRoute(businessDate: '2026-10-04', trip: _trip);
      final s = _session(_Source([const TripLoad.failed('Your sign-in expired. Sign in again.', signInExpired: true)]), store);
      await s.restore();
      expect(s.signedIn, isFalse);
      expect(store.saved, isEmpty);
    });

    test('the next Waypoint day does not show yesterday\'s route', () async {
      final store = MemoryRouteStore()..saved['USR006'] = const SavedRoute(businessDate: '2026-10-04', trip: _trip);
      final s = _session(_Source([_unreachable]), store, now: _tomorrow);
      await s.restore();
      expect(s.hasRoute, isFalse);
      expect(store.saved, isEmpty);
    });

    test('a session without a store behaves as before', () async {
      final s = DriverSession(database: InMemoryLocalDatabase(), queue: InMemorySyncQueue(), auth: _Auth(), trips: _Source([_unreachable]));
      addTearDown(s.dispose);
      await s.restore();
      expect(s.hasRoute, isFalse);
      expect(s.tripsError, isNotNull);
    });
  });

  group('on screen', () {
    testWidgets('the saved route is shown with a note, and the note goes when the live route arrives', (tester) async {
      final store = MemoryRouteStore()..saved['USR006'] = SavedRoute(businessDate: ApiTripSource.dateKey(DateTime.now()), trip: _trip.withRunStatus('in_progress'));
      final source = _Source([_unreachable, const TripLoad.loaded(_trip)]);
      await pumpScreen(tester, WaypointDriverApp(database: InMemoryLocalDatabase(), queue: InMemorySyncQueue(), auth: _Auth(), trips: source, routeStore: store));
      await tester.pumpAndSettle();
      expect(find.text('Saved route'), findsOneWidget);
      expect(find.textContaining('Showing the route saved on this phone'), findsOneWidget);
      expect(find.text('Dehiwala'), findsWidgets);
      expect(find.text('No route loaded'), findsNothing);
    });
  });
}
