import 'dart:convert';
import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:waypoint_driver/main.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:waypoint_driver/app/driver_session.dart';
import 'package:waypoint_driver/auth/auth_gateway.dart';
import 'package:waypoint_driver/auth/profile_api.dart';
import 'package:waypoint_driver/data/driver_models.dart';
import 'package:waypoint_driver/offline/local_database.dart';
import 'package:waypoint_driver/sync/sync.dart';
import 'package:waypoint_driver/trips/trip_source.dart';
import 'package:waypoint_driver/trips/trip_start.dart';
import 'package:waypoint_driver/trips/trips_api.dart';

import 'helpers/render.dart';

const _driver = DriverProfile(userId: 'USR006', subject: 'usr-driver', roles: ['DRIVER'], vehicleId: 'VEH001');

Map<String, Object?> _detail({String status = 'prepared', int version = 3}) => {
      'tripId': 'trip-1',
      'currentPlanVersion': version,
      'run': {'id': 'run-1', 'tripId': 'trip-1', 'planId': 'plan-1', 'planRef': 'PLAN000001', 'vehicleId': 'VEH001', 'status': status},
      'stops': [
        {'id': 'stop-1', 'outletId': 'O1', 'outletName': 'Dehiwala', 'stopSequence': 1, 'expectedUnits': 10, 'status': 'pending'},
      ],
    };

class _Auth implements AuthGateway {
  String? token = 'tok';
  int signOuts = 0;

  @override
  Future<AuthOutcome> signIn() async => const AuthOutcome.signedIn(_driver);
  @override
  Future<DriverProfile?> restore() async => null;
  @override
  Future<void> signOut() async => signOuts++;
  @override
  Future<String?> accessToken() async => token;
}

class _Source implements TripSource {
  _Source(this.detail);
  Map<String, Object?> detail;
  @override
  Future<TripLoad> loadToday() async => TripLoad.loaded(tripFromJson(detail));
}

class _Starter implements TripStarter {
  _Starter(this.results);
  final List<TripStartResult> results;
  final calls = <(String, String, int)>[];

  @override
  Future<TripStartResult> start(TripInfo trip, {required String operationId}) async {
    calls.add((trip.tripId, operationId, trip.planVersion));
    return results[calls.length - 1 < results.length ? calls.length - 1 : results.length - 1];
  }
}

http.Response _json(Object body, [int status = 200]) => http.Response(jsonEncode(body), status, headers: {'content-type': 'application/json'});

void main() {
  setUpAll(loadAppFonts);

  group('tripFromJson plan and run status', () {
    test('keeps the plan to acknowledge, its current version and the run status', () {
      final trip = tripFromJson(_detail());
      expect(trip.planId, 'plan-1');
      expect(trip.planVersion, 3);
      expect(trip.runStatus, 'prepared');
      expect(trip.started, isFalse);
      expect(tripFromJson(_detail(status: 'in_progress')).started, isTrue);
    });
  });

  group('ApiTripStarter', () {
    late TripInfo trip;
    setUp(() => trip = tripFromJson(_detail()));

    ApiTripStarter starter(MockClient client, {_Auth? auth}) => ApiTripStarter(client: client, baseUrl: 'http://api/', auth: auth ?? _Auth());

    test('acknowledges the current plan version, then starts with a stable idempotency key', () async {
      final seen = <http.Request>[];
      final result = await starter(MockClient((request) async {
        seen.add(request);
        return _json({});
      })).start(trip, operationId: 'op-start-1');
      expect(result.status, TripStartStatus.started);
      expect(seen.map((r) => r.url.path), ['/api/v1/planning/plans/plan-1/acknowledgements', '/api/v1/delivery/trips/trip-1/start']);
      expect(jsonDecode(seen.first.body), {'version': 3});
      expect(seen.last.headers['Idempotency-Key'], 'op-start-1');
      expect(seen.every((r) => r.headers['Authorization'] == 'Bearer tok'), isTrue);
    });

    test('a trip that is already started makes no calls', () async {
      var calls = 0;
      final result = await starter(MockClient((_) async {
        calls++;
        return _json({});
      })).start(tripFromJson(_detail(status: 'in_progress')), operationId: 'op');
      expect(result.status, TripStartStatus.started);
      expect(calls, 0);
    });

    test('a failed acknowledgement stops before the start call', () async {
      final paths = <String>[];
      final result = await starter(MockClient((request) async {
        paths.add(request.url.path);
        return _json({'detail': 'stale_version'}, 409);
      })).start(trip, operationId: 'op');
      expect(result.status, TripStartStatus.refused);
      expect(result.message, contains('plan changed'));
      expect(paths, hasLength(1));
    });

    test('a start the server refuses says why', () async {
      final result = await starter(MockClient((request) async {
        if (request.url.path.endsWith('/start')) return _json({'detail': 'conflict: run already completed'}, 409);
        return _json({});
      })).start(trip, operationId: 'op');
      expect(result.status, TripStartStatus.refused);
      expect(result.message, contains('run already completed'));
    });

    test('no connection or a server error means try again later', () async {
      expect((await starter(MockClient((_) async => throw const SocketException('down'))).start(trip, operationId: 'op')).status, TripStartStatus.offline);
      expect((await starter(MockClient((_) async => http.Response('', 503))).start(trip, operationId: 'op')).status, TripStartStatus.offline);
    });

    test('a missing or rejected token means signing in again', () async {
      expect((await starter(MockClient((_) async => _json({})), auth: _Auth()..token = null).start(trip, operationId: 'op')).status, TripStartStatus.signInNeeded);
      expect((await starter(MockClient((_) async => http.Response('', 401))).start(trip, operationId: 'op')).status, TripStartStatus.signInNeeded);
    });

    test('a forbidden account is told it may not start trips', () async {
      final result = await starter(MockClient((_) async => http.Response('', 403))).start(trip, operationId: 'op');
      expect(result.status, TripStartStatus.refused);
      expect(result.message, contains('not allowed'));
    });
  });

  group('DriverSession starting the trip', () {
    DriverSession session(_Starter starter, {Map<String, Object?>? detail, _Auth? auth}) => DriverSession(
          database: InMemoryLocalDatabase(),
          queue: InMemorySyncQueue(),
          auth: auth ?? _Auth(),
          trips: _Source(detail ?? _detail()),
          starter: starter,
        );

    test('confirming the load starts the trip once, with the same key on a retry', () async {
      final starter = _Starter([const TripStartResult(TripStartStatus.offline), const TripStartResult(TripStartStatus.started)]);
      final s = session(starter);
      await s.signIn();
      expect(starter.calls, isEmpty, reason: 'signing in alone does not start the trip');
      await s.confirmLoad();
      expect(s.tripStartState, TripStartState.waiting);
      expect(s.tripStartMessage, contains('when Waypoint can be reached'));
      await s.startTrip();
      expect(s.tripStartState, TripStartState.started);
      expect(starter.calls.length, 2);
      expect(starter.calls[0].$2, starter.calls[1].$2);
      expect(starter.calls.first.$3, 3);
      await s.startTrip();
      expect(starter.calls.length, 2, reason: 'a started trip is not started again');
    });

    test('a refused start is kept with its reason and the driver stays signed in', () async {
      final starter = _Starter([const TripStartResult(TripStartStatus.refused, 'The plan changed.')]);
      final s = session(starter);
      await s.signIn();
      await s.confirmLoad();
      expect(s.tripStartState, TripStartState.refused);
      expect(s.tripStartMessage, 'The plan changed.');
      expect(s.signedIn, isTrue);
    });

    test('departing anyway also starts the trip', () async {
      final starter = _Starter([const TripStartResult(TripStartStatus.started)]);
      final s = session(starter);
      await s.signIn();
      await s.reportLoadDiscrepancy();
      expect(starter.calls, isEmpty);
      await s.overrideLoadCheck();
      expect(starter.calls, hasLength(1));
      expect(s.tripStartState, TripStartState.started);
    });

    test('a rejected sign-in during the start returns to the sign-in screen', () async {
      final auth = _Auth();
      final s = session(_Starter([const TripStartResult(TripStartStatus.signInNeeded, 'Your sign-in expired. Sign in again.')]), auth: auth);
      await s.signIn();
      await s.confirmLoad();
      expect(s.signedIn, isFalse);
      expect(s.signInError, contains('Sign in again'));
      expect(auth.signOuts, 1);
    });

    test('a run that is already in progress skips the load check and counts as started', () async {
      final starter = _Starter([const TripStartResult(TripStartStatus.started)]);
      final s = session(starter, detail: _detail(status: 'in_progress'));
      await s.signIn();
      expect(s.loadResolved, isTrue);
      expect(s.tripStartState, TripStartState.started);
      expect(starter.calls, isEmpty);
    });

    test('reloading after a refusal tries again with the new plan version', () async {
      final starter = _Starter([const TripStartResult(TripStartStatus.refused, 'The plan changed.'), const TripStartResult(TripStartStatus.started)]);
      final source = _Source(_detail(version: 3));
      final s = DriverSession(database: InMemoryLocalDatabase(), queue: InMemorySyncQueue(), auth: _Auth(), trips: source, starter: starter);
      await s.signIn();
      await s.confirmLoad();
      expect(s.tripStartState, TripStartState.refused);
      source.detail = _detail(version: 4);
      await s.loadTrips();
      expect(starter.calls.map((c) => c.$3), [3, 4]);
      expect(s.tripStartState, TripStartState.started);
    });

    test('sessions without a starter (demo and tests) just confirm the load', () async {
      final s = DriverSession(database: InMemoryLocalDatabase(), queue: InMemorySyncQueue(), auth: _Auth(), trips: _Source(_detail()));
      await s.signIn();
      await s.confirmLoad();
      expect(s.loadResolved, isTrue);
      expect(s.tripStartState, TripStartState.notStarted);
    });
  });
  group('screens', () {
    Future<void> confirmLoad(WidgetTester tester, TripStarter starter) async {
      await pumpScreen(tester, WaypointDriverApp(database: InMemoryLocalDatabase(), queue: InMemorySyncQueue(), auth: _Auth(), trips: _Source(_detail()), starter: starter));
      await tester.tap(find.text('Continue to sign in'));
      await tester.pumpAndSettle();
      await tester.tap(find.text('Confirm load on board'));
      await tester.pumpAndSettle();
    }

    testWidgets('says the trip started once the server accepted it', (tester) async {
      await confirmLoad(tester, _Starter([const TripStartResult(TripStartStatus.started)]));
      expect(find.text('Load confirmed and trip started.'), findsOneWidget);
    });

    testWidgets('says the trip is not started when there is no connection', (tester) async {
      await confirmLoad(tester, _Starter([const TripStartResult(TripStartStatus.offline)]));
      expect(find.textContaining('The trip is not started yet'), findsOneWidget);
      expect(find.textContaining('trip started.'), findsNothing);
    });

    testWidgets('says why the server refused', (tester) async {
      await confirmLoad(tester, _Starter([const TripStartResult(TripStartStatus.refused, 'The plan changed, so the trip could not start.')]));
      expect(find.textContaining('the trip did not start'), findsOneWidget);
      expect(find.textContaining('The plan changed'), findsOneWidget);
    });
  });
}
