import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:connectivity_plus/connectivity_plus.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:sqflite_common_ffi/sqflite_ffi.dart';
import 'package:waypoint_driver/app/driver_session.dart';
import 'package:waypoint_driver/auth/auth_gateway.dart';
import 'package:waypoint_driver/auth/profile_api.dart';
import 'package:waypoint_driver/connectivity/connectivity_monitor.dart';
import 'package:waypoint_driver/data/driver_models.dart';
import 'package:waypoint_driver/main.dart';
import 'package:waypoint_driver/offline/local_database.dart';
import 'package:waypoint_driver/sync/sqlite_sync_queue.dart';
import 'package:waypoint_driver/sync/sync.dart';
import 'package:waypoint_driver/sync/sync_worker.dart';
import 'package:waypoint_driver/trips/trip_source.dart';
import 'package:waypoint_driver/trips/trip_start.dart';

import 'helpers/render.dart';

const _driver = DriverProfile(userId: 'USR006', subject: 'usr-driver', roles: ['DRIVER'], vehicleId: 'VEH001');
const _stop = StopInfo(stopId: 'stop-1', sequence: 1, outletCode: 'O1', name: 'Dehiwala', windowStart: '', windowEnd: '', units: 10, accessNote: '', contactNote: '', goods: 'G');
const _trip = TripInfo(tripId: 'trip-1', runId: 'run-1', vehicleCode: 'VEH001', tripRef: 'PLAN1', depot: 'D', window: '', stops: [_stop], planId: 'p', planVersion: 1, runStatus: 'prepared');

class _Monitor implements ConnectivityMonitor {
  _Monitor({this.online = true});

  bool online;
  final _controller = StreamController<bool>.broadcast();

  void set(bool value) {
    online = value;
    _controller.add(value);
  }

  @override
  Future<bool> isOnline() async => online;

  @override
  Stream<bool> get changes => _controller.stream;
}

class _Auth implements AuthGateway {
  @override
  Future<AuthOutcome> signIn() async => const AuthOutcome.signedIn(_driver);
  @override
  Future<DriverProfile?> restore() async => null;
  @override
  Future<void> signOut() async {}
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
  _Starter(this.results);
  final List<TripStartResult> results;
  int calls = 0;
  @override
  Future<TripStartResult> start(TripInfo trip, {required String operationId}) async => results[calls++ < results.length ? calls - 1 : results.length - 1];
}

Future<void> _settle() => Future<void>.delayed(const Duration(milliseconds: 50));

void main() {
  sqfliteFfiInit();
  setUpAll(loadAppFonts);

  group('PlatformConnectivityMonitor.onlineFrom', () {
    test('is online when any transport is up', () {
      expect(PlatformConnectivityMonitor.onlineFrom([ConnectivityResult.wifi]), isTrue);
      expect(PlatformConnectivityMonitor.onlineFrom([ConnectivityResult.mobile]), isTrue);
      expect(PlatformConnectivityMonitor.onlineFrom([ConnectivityResult.none, ConnectivityResult.mobile]), isTrue);
      expect(PlatformConnectivityMonitor.onlineFrom([ConnectivityResult.vpn]), isTrue);
    });

    test('is offline with no transport', () {
      expect(PlatformConnectivityMonitor.onlineFrom([ConnectivityResult.none]), isFalse);
      expect(PlatformConnectivityMonitor.onlineFrom(const []), isFalse);
    });
  });

  group('DriverSession and the network', () {
    test('starts from what the monitor reports and follows its changes', () async {
      final monitor = _Monitor(online: false);
      final session = DriverSession(database: InMemoryLocalDatabase(), queue: InMemorySyncQueue(), connectivity: monitor);
      addTearDown(session.dispose);
      expect(session.deviceOnline, isTrue, reason: 'online until the monitor says otherwise');
      await _settle();
      expect(session.deviceOnline, isFalse);
      expect(session.offline, isTrue);
      monitor.set(true);
      await _settle();
      expect(session.deviceOnline, isTrue);
      expect(session.offline, isFalse);
    });

    test('a session without a monitor is always online', () {
      final session = DriverSession(database: InMemoryLocalDatabase(), queue: InMemorySyncQueue());
      addTearDown(session.dispose);
      expect(session.offline, isFalse);
    });

    test('tells its listeners when the connection changes, and only when it changes', () async {
      final monitor = _Monitor();
      final session = DriverSession(database: InMemoryLocalDatabase(), queue: InMemorySyncQueue(), connectivity: monitor);
      addTearDown(session.dispose);
      await _settle();
      var notified = 0;
      session.addListener(() => notified++);
      monitor.set(true);
      await _settle();
      expect(notified, 0, reason: 'no change');
      monitor.set(false);
      await _settle();
      expect(notified, 1);
    });

    test('stops listening when the session is disposed', () async {
      final monitor = _Monitor();
      final session = DriverSession(database: InMemoryLocalDatabase(), queue: InMemorySyncQueue(), connectivity: monitor);
      await _settle();
      session.dispose();
      monitor.set(false);
      await _settle();
      expect(session.deviceOnline, isTrue, reason: 'a disposed session ignores later events');
    });

    test('coming back online retries a route that could not be loaded', () async {
      final monitor = _Monitor();
      final source = _Source([const TripLoad.failed('Could not reach Waypoint. Check your connection and try again.'), const TripLoad.loaded(_trip)]);
      final session = DriverSession(database: InMemoryLocalDatabase(), queue: InMemorySyncQueue(), auth: _Auth(), trips: source, connectivity: monitor);
      addTearDown(session.dispose);
      await session.signIn();
      expect(session.hasRoute, isFalse);
      monitor.set(false);
      await _settle();
      expect(source.calls, 1, reason: 'going offline does not retry');
      monitor.set(true);
      await _settle();
      expect(source.calls, 2);
      expect(session.hasRoute, isTrue);
    });

    test('coming back online starts a trip that was waiting for a connection', () async {
      final monitor = _Monitor();
      final starter = _Starter([const TripStartResult(TripStartStatus.offline), const TripStartResult(TripStartStatus.started)]);
      final session = DriverSession(database: InMemoryLocalDatabase(), queue: InMemorySyncQueue(), auth: _Auth(), trips: _Source([const TripLoad.loaded(_trip)]), starter: starter, connectivity: monitor);
      addTearDown(session.dispose);
      await session.signIn();
      await session.confirmLoad();
      expect(session.tripStartState, TripStartState.waiting);
      monitor.set(false);
      monitor.set(true);
      await _settle();
      expect(starter.calls, 2);
      expect(session.tripStartState, TripStartState.started);
    });

    test('coming back online sends what is waiting without waiting for the timer', () async {
      final directory = await Directory.systemTemp.createTemp('waypoint-connectivity-test-');
      addTearDown(() => directory.delete(recursive: true));
      final queue = SqliteSyncQueue(open: () => SqliteSyncQueue.openAt('${directory.path}/q.db', factory: databaseFactoryFfi));
      final sent = <String>[];
      var reachable = false;
      final worker = DeliverySyncWorker(
        queue: queue,
        auth: _Auth(),
        client: MockClient((request) async {
          if (!reachable) throw const SocketException('no signal');
          if (request.method == 'GET') return http.Response(jsonEncode({'run': {'status': 'in_progress'}}), 200);
          final op = ((jsonDecode(request.body) as Map)['operations'] as List).single as Map;
          sent.add(op['type'] as String);
          return http.Response(jsonEncode({'results': [{'operationId': op['operationId'], 'status': 'APPLIED'}]}), 200);
        }),
        baseUrl: 'https://waypoint.example',
        interval: const Duration(hours: 1),
      );
      final monitor = _Monitor();
      final session = DriverSession(database: InMemoryLocalDatabase(), queue: queue, auth: _Auth(), trips: _Source([const TripLoad.loaded(_trip)]), worker: worker, connectivity: monitor);
      addTearDown(session.dispose);
      await session.signIn();
      await session.markArrived(_stop);
      await _settle();
      expect(sent, isEmpty);
      monitor.set(false);
      await _settle();
      reachable = true;
      monitor.set(true);
      await Future<void>.delayed(const Duration(milliseconds: 300));
      expect(sent, ['ARRIVED']);
      worker.stop();
    });
  });

  group('screens', () {
    Future<_Monitor> boot(WidgetTester tester, {bool online = true}) async {
      final monitor = _Monitor(online: online);
      await pumpScreen(tester, WaypointDriverApp(database: InMemoryLocalDatabase(), queue: InMemorySyncQueue(), demoAuth: true, connectivity: monitor));
      await tester.pump(const Duration(milliseconds: 20));
      return monitor;
    }

    Future<void> signIn(WidgetTester tester) async {
      await tester.enterText(find.byType(TextField).first, 'DRV-0318');
      await tester.enterText(find.byType(TextField).last, 'secret');
      await tester.pump();
      await tester.tap(find.text('Sign in').last);
      await tester.pumpAndSettle();
      await tester.tap(find.text('Confirm load on board'));
      await tester.pumpAndSettle();
      await tester.pump(const Duration(seconds: 4));
      await tester.pumpAndSettle();
    }

    testWidgets('signing in waits for signal when the phone is offline', (tester) async {
      await boot(tester, online: false);
      expect(find.text('Waiting for signal…'), findsOneWidget);
      expect(find.textContaining('Move to an area with coverage'), findsOneWidget);
      expect(find.text('Sign in'), findsWidgets);
    });

    testWidgets('the sign-in comes back by itself when the signal does', (tester) async {
      final monitor = await boot(tester, online: false);
      monitor.set(true);
      await tester.pumpAndSettle();
      expect(find.text('Waiting for signal…'), findsNothing);
      expect(find.byType(TextField), findsNWidgets(2));
    });

    testWidgets('the route says there is no connection and that nothing is lost', (tester) async {
      final monitor = await boot(tester);
      await signIn(tester);
      expect(find.text('No connection'), findsNothing);
      monitor.set(false);
      await tester.pumpAndSettle();
      expect(find.text('No connection'), findsOneWidget);
      expect(find.textContaining('saved on this phone and sent when you are back online'), findsOneWidget);
      monitor.set(true);
      await tester.pumpAndSettle();
      expect(find.text('No connection'), findsNothing);
    });

    testWidgets('a stop opened while offline shows the offline variant and it follows the connection', (tester) async {
      final monitor = await boot(tester);
      await signIn(tester);
      monitor.set(false);
      await tester.pumpAndSettle();
      await tester.tap(find.text('View Stop'));
      await tester.pumpAndSettle();
      await tester.tap(find.text('I’ve stopped safely'));
      await tester.pumpAndSettle();
      expect(find.text('You can still record this delivery'), findsOneWidget);
      expect(find.text('Save on this device'), findsOneWidget);
      monitor.set(true);
      await tester.pumpAndSettle();
      expect(find.text('You can still record this delivery'), findsNothing);
      expect(find.text('Save delivery'), findsOneWidget);
    });
  });
}
