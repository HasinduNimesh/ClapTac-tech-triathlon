import 'dart:convert';
import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:sqflite_common_ffi/sqflite_ffi.dart';
import 'package:waypoint_driver/app/driver_session.dart';
import 'package:waypoint_driver/auth/auth_gateway.dart';
import 'package:waypoint_driver/auth/profile_api.dart';
import 'package:waypoint_driver/data/driver_models.dart';
import 'package:waypoint_driver/main.dart';
import 'package:waypoint_driver/offline/local_database.dart';
import 'package:waypoint_driver/sync/sqlite_sync_queue.dart';
import 'package:waypoint_driver/sync/sync.dart';
import 'package:waypoint_driver/sync/sync_worker.dart';
import 'package:waypoint_driver/trips/trip_source.dart';
import 'package:waypoint_driver/widgets/driver_shell.dart';

import 'helpers/render.dart';

const _driver = DriverProfile(userId: 'USR006', subject: 'usr-driver', roles: ['DRIVER'], vehicleId: 'VEH001');
const _stopA = StopInfo(stopId: 'stop-a', sequence: 1, outletCode: 'OUTA', name: 'Dehiwala', windowStart: '', windowEnd: '', units: 10, accessNote: '', contactNote: '', goods: 'G');
const _stopB1 = StopInfo(stopId: 'stop-b1', sequence: 1, outletCode: 'OUTB', name: 'Nugegoda', windowStart: '', windowEnd: '', units: 5, accessNote: '', contactNote: '', goods: 'G');
const _tripA = TripInfo(tripId: 'trip-a', runId: 'run-a', vehicleCode: 'VEH001', tripRef: 'PLAN-A', depot: 'D', window: '', stops: [_stopA], runStatus: 'in_progress');
const _tripB = TripInfo(tripId: 'trip-b', runId: 'run-b', vehicleCode: 'VEH001', tripRef: 'PLAN-B', depot: 'D', window: '', stops: [_stopB1], runStatus: 'prepared', planId: 'p', planVersion: 1);

class _Auth implements AuthGateway {
  int signOuts = 0;
  @override
  Future<AuthOutcome> signIn() async => const AuthOutcome.signedIn(_driver);
  @override
  Future<DriverProfile?> restore() async => null;
  @override
  Future<void> signOut() async => signOuts++;
  @override
  Future<String?> accessToken() async => 'tok';
}

/// The first read is trip A; later reads answer with whatever [then] holds.
class _Source implements TripSource {
  _Source(this.then);
  TripLoad then;
  int calls = 0;
  @override
  Future<TripLoad> loadToday() async => calls++ == 0 ? const TripLoad.loaded(_tripA) : then;
}

void main() {
  sqfliteFfiInit();
  setUpAll(loadAppFonts);

  group('completing a route', () {
    late Directory directory;
    late SqliteSyncQueue queue;
    late _Auth auth;
    late List<String> sent;
    late bool offline;

    setUp(() async {
      directory = await Directory.systemTemp.createTemp('waypoint-next-trip-test-');
      queue = SqliteSyncQueue(open: () => SqliteSyncQueue.openAt('${directory.path}/q.db', factory: databaseFactoryFfi));
      auth = _Auth();
      sent = [];
      offline = false;
    });

    tearDown(() => directory.delete(recursive: true));

    DriverSession session(_Source source) {
      final worker = DeliverySyncWorker(
        queue: queue,
        auth: auth,
        client: MockClient((request) async {
          if (offline) throw const SocketException('no signal');
          if (request.method == 'GET') return http.Response(jsonEncode({'run': {'status': 'in_progress'}}), 200);
          final op = ((jsonDecode(request.body) as Map)['operations'] as List).single as Map;
          sent.add(op['type'] as String);
          return http.Response(jsonEncode({'results': [{'operationId': op['operationId'], 'status': 'APPLIED'}]}), 200);
        }),
        baseUrl: 'https://waypoint.example',
        interval: const Duration(hours: 1),
      );
      final s = DriverSession(database: InMemoryLocalDatabase(), queue: queue, auth: auth, trips: source, worker: worker);
      addTearDown(s.dispose);
      return s;
    }

    Future<void> doTheOnlyStop(DriverSession s) async {
      await s.markArrived(_stopA);
      await s.recordDelivery(_stopA, const DeliveryDraft(outcome: DeliveryOutcome.failed, reason: 'OUTLET_CLOSED'));
      expect(s.routeComplete, isTrue);
    }

    test('opens the next trip when there is one, instead of signing out', () async {
      final s = session(_Source(const TripLoad.loaded(_tripB)));
      await s.signIn();
      await doTheOnlyStop(s);
      final result = await s.completeTrip();
      expect(result.kind, TripWrapUp.nextTrip);
      expect(result.finished?.tripRef, 'PLAN-A');
      expect(result.next?.tripRef, 'PLAN-B');
      expect(sent, ['ARRIVED', 'STOP_OUTCOME', 'ROUTE_COMPLETED'], reason: 'the finished trip was sent before the next one opened');
      expect(s.signedIn, isTrue);
      expect(auth.signOuts, 0);
      expect(s.trip.tripId, 'trip-b');
      expect(s.trip.completedStops, 0, reason: 'trip A\'s results do not leak into trip B');
      expect(s.routeComplete, isFalse);
      expect(s.loadResolved, isFalse, reason: 'the driver checks the next load too');
      expect(s.tripStartState, TripStartState.notStarted);
      expect(s.tab, DriverTab.route);
    });

    test('signs out when there is no further trip', () async {
      final s = session(_Source(const TripLoad.none()));
      await s.signIn();
      await doTheOnlyStop(s);
      final result = await s.completeTrip();
      expect(result.kind, TripWrapUp.signedOut);
      expect(s.signedIn, isFalse);
      expect(auth.signOuts, 1);
      expect(sent, contains('ROUTE_COMPLETED'));
    });

    test('does not loop on the trip it just finished', () async {
      final s = session(_Source(const TripLoad.loaded(_tripA)));
      await s.signIn();
      await doTheOnlyStop(s);
      expect((await s.completeTrip()).kind, TripWrapUp.signedOut);
      expect(s.signedIn, isFalse);
    });

    test('signs out when the next trip could not be checked, because everything is already sent', () async {
      final s = session(_Source(const TripLoad.failed('Could not reach Waypoint. Check your connection and try again.')));
      await s.signIn();
      await doTheOnlyStop(s);
      final result = await s.completeTrip();
      expect(result.kind, TripWrapUp.signedOut);
      expect(sent, contains('ROUTE_COMPLETED'));
    });

    test('keeps the driver signed in, with the same trip, when updates could not be sent', () async {
      final source = _Source(const TripLoad.loaded(_tripB));
      final s = session(source);
      await s.signIn();
      await doTheOnlyStop(s);
      offline = true;
      final result = await s.completeTrip();
      expect(result.kind, TripWrapUp.unsent);
      expect(result.unsent, greaterThan(0));
      expect(s.signedIn, isTrue);
      expect(s.trip.tripId, 'trip-a');
      expect(source.calls, 1, reason: 'the next trip is not looked up before the finished one is sent');
      // Back online, finishing again goes through to the next trip.
      offline = false;
      final again = await s.completeTrip();
      expect(again.kind, TripWrapUp.nextTrip);
      expect(s.trip.tripId, 'trip-b');
    });

    test('a route that is not finished just signs out like before', () async {
      final s = session(_Source(const TripLoad.loaded(_tripB)));
      await s.signIn();
      expect(s.routeComplete, isFalse);
      final result = await s.completeTrip();
      expect(result.kind, TripWrapUp.signedOut);
      expect(s.signedIn, isFalse);
    });
  });

  group('on screen', () {
    testWidgets('finishing one trip opens the next trip\'s load check and says so', (tester) async {
      final source = _Source(const TripLoad.loaded(_tripB));
      await pumpScreen(tester, WaypointDriverApp(database: InMemoryLocalDatabase(), queue: InMemorySyncQueue(), auth: _Auth(), trips: source));
      await tester.tap(find.text('Continue to sign in'));
      await tester.pumpAndSettle();
      expect(find.text('Check the load before you leave'), findsNothing, reason: 'trip A is already started');

      await tester.tap(find.text('View Stop'));
      await tester.pumpAndSettle();
      await tester.tap(find.text('I’ve stopped safely'));
      await tester.pumpAndSettle();
      await tester.tap(find.text('Failed'));
      await tester.pump();
      await tester.tap(find.text('Save delivery'));
      await tester.pumpAndSettle();
      await tester.tap(find.text('Save take-back offline'));
      await tester.pumpAndSettle();
      await tester.tap(find.text('Back to route'));
      await tester.pumpAndSettle();
      await tester.tap(find.text('Summary'));
      await tester.pumpAndSettle();
      await tester.ensureVisible(find.text('Finish trip'));
      await tester.tap(find.text('Finish trip'));
      await tester.pumpAndSettle();

      expect(find.text('Check the load before you leave'), findsOneWidget);
      expect(find.textContaining('PLAN-B'), findsWidgets);
      expect(find.textContaining('Trip PLAN-A is complete and sent'), findsOneWidget);
      expect(find.text('Continue to sign in'), findsNothing, reason: 'still signed in');
    });
  });
}
