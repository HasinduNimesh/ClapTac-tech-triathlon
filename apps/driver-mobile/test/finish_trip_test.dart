import 'dart:convert';
import 'dart:io';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:sqflite_common_ffi/sqflite_ffi.dart';
import 'package:waypoint_driver/app/driver_flow.dart';
import 'package:waypoint_driver/app/driver_session.dart';
import 'package:waypoint_driver/auth/auth_gateway.dart';
import 'package:waypoint_driver/auth/profile_api.dart';
import 'package:waypoint_driver/data/driver_models.dart';
import 'package:waypoint_driver/offline/local_database.dart';
import 'package:waypoint_driver/sync/sqlite_sync_queue.dart';
import 'package:waypoint_driver/sync/sync.dart';
import 'package:waypoint_driver/sync/sync_worker.dart';
import 'package:waypoint_driver/trips/trip_source.dart';

import 'helpers/render.dart';

const _driver = DriverProfile(userId: 'USR006', subject: 'usr-driver', roles: ['DRIVER'], vehicleId: 'VEH001');
const _stop = StopInfo(stopId: 'stop-1', sequence: 1, outletCode: 'O1', name: 'Dehiwala', windowStart: '', windowEnd: '', units: 10, accessNote: '', contactNote: '', goods: 'G');
const _trip = TripInfo(tripId: 'trip-1', runId: 'run-1', vehicleCode: 'VEH001', tripRef: 'PLAN1', depot: 'D', window: '', stops: [_stop], runStatus: 'in_progress');

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

class _Source implements TripSource {
  @override
  Future<TripLoad> loadToday() async => const TripLoad.loaded(_trip);
}

void main() {
  sqfliteFfiInit();

  group('finishing the trip', () {
    late Directory directory;
    late SqliteSyncQueue queue;
    late _Auth auth;
    late List<String> sent;
    late bool offline;
    late DriverSession session;

    setUp(() async {
      directory = await Directory.systemTemp.createTemp('waypoint-finish-test-');
      queue = SqliteSyncQueue(open: () => SqliteSyncQueue.openAt('${directory.path}/q.db', factory: databaseFactoryFfi));
      auth = _Auth();
      sent = [];
      offline = false;
      final client = MockClient((request) async {
        if (offline) throw const SocketException('no signal');
        if (request.method == 'GET') return http.Response(jsonEncode({'run': {'status': 'in_progress'}}), 200);
        final op = ((jsonDecode(request.body) as Map)['operations'] as List).single as Map;
        sent.add(op['type'] as String);
        return http.Response(jsonEncode({'results': [{'operationId': op['operationId'], 'status': 'APPLIED'}]}), 200);
      });
      final worker = DeliverySyncWorker(queue: queue, auth: auth, client: client, baseUrl: 'https://waypoint.example', interval: const Duration(hours: 1));
      session = DriverSession(database: InMemoryLocalDatabase(), queue: queue, auth: auth, trips: _Source(), worker: worker);
      await session.signIn();
    });

    tearDown(() async {
      session.dispose();
      await directory.delete(recursive: true);
    });

    Future<void> recordTheOnlyStop() async {
      await session.markArrived(_stop);
      await session.recordDelivery(_stop, const DeliveryDraft(outcome: DeliveryOutcome.failed, reason: 'OUTLET_CLOSED'));
      expect(session.routeComplete, isTrue);
    }

    test('sends the route completion before it signs out', () async {
      await recordTheOnlyStop();
      final unsent = await session.finishTrip();
      expect(unsent, 0);
      expect(session.signedIn, isFalse);
      expect(sent, ['ARRIVED', 'STOP_OUTCOME', 'ROUTE_COMPLETED']);
      expect(auth.signOuts, 1);
    });

    test('stays signed in and reports what is unsent when Waypoint cannot be reached', () async {
      await recordTheOnlyStop();
      offline = true;
      final unsent = await session.finishTrip();
      expect(unsent, greaterThan(0));
      expect(session.signedIn, isTrue);
      expect(auth.signOuts, 0);
    });

    test('signing out anyway keeps the updates queued for the next sign-in', () async {
      await recordTheOnlyStop();
      offline = true;
      expect(await session.finishTrip(), greaterThan(0));
      expect(await session.finishTrip(force: true), 0);
      expect(session.signedIn, isFalse);
      // The same driver signing in again finds the updates still queued and sends them.
      offline = false;
      await session.signIn();
      await session.retrySync();
      expect(sent, containsAll(['ARRIVED', 'STOP_OUTCOME', 'ROUTE_COMPLETED']));
    });

    test('signing out with nothing waiting does not ask', () async {
      expect(await session.finishTrip(), 0);
      expect(session.signedIn, isFalse);
    });
  });

  group('the unsent-updates dialog', () {
    setUpAll(loadAppFonts);

    Future<List<bool>> open(WidgetTester tester, int unsent) async {
      final calls = <bool>[];
      final session = _AskingSession(unsent, calls);
      await pumpScreen(tester, Builder(builder: (context) => Scaffold(body: Center(child: TextButton(onPressed: () => finishOrAsk(context, session), child: const Text('finish'))))));
      await tester.tap(find.text('finish'));
      await tester.pumpAndSettle();
      return calls;
    }

    testWidgets('says how many updates are unsent and that nothing is lost', (tester) async {
      await open(tester, 2);
      expect(find.text('Updates not sent yet'), findsOneWidget);
      expect(find.textContaining('2 updates are still saved on this phone'), findsOneWidget);
      expect(find.textContaining('Nothing is lost'), findsOneWidget);
    });

    testWidgets('uses the singular for one update', (tester) async {
      await open(tester, 1);
      expect(find.textContaining('1 update is still saved'), findsOneWidget);
    });

    testWidgets('staying signed in does not sign out', (tester) async {
      final calls = await open(tester, 2);
      await tester.tap(find.text('Stay signed in'));
      await tester.pumpAndSettle();
      expect(calls, [false]);
    });

    testWidgets('signing out anyway forces the sign-out', (tester) async {
      final calls = await open(tester, 2);
      await tester.tap(find.text('Sign out anyway'));
      await tester.pumpAndSettle();
      expect(calls, [false, true]);
    });

    testWidgets('shows no dialog when everything was sent', (tester) async {
      final calls = await open(tester, 0);
      expect(find.text('Updates not sent yet'), findsNothing);
      expect(calls, [false]);
    });
  });
}

class _AskingSession extends DriverSession {
  _AskingSession(this.unsent, this.calls) : super(database: InMemoryLocalDatabase(), queue: InMemorySyncQueue());

  final int unsent;
  final List<bool> calls;

  @override
  Future<int> finishTrip({bool force = false}) async {
    calls.add(force);
    return force ? 0 : unsent;
  }
}
