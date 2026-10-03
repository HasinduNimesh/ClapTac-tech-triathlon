import 'dart:convert';
import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:waypoint_driver/app/driver_session.dart';
import 'package:waypoint_driver/auth/auth_gateway.dart';
import 'package:waypoint_driver/auth/profile_api.dart';
import 'package:waypoint_driver/main.dart';
import 'package:waypoint_driver/offline/local_database.dart';
import 'package:waypoint_driver/sync/sync.dart';
import 'package:waypoint_driver/trips/trip_source.dart';
import 'package:waypoint_driver/trips/trips_api.dart';

import 'helpers/render.dart';

const _driver = DriverProfile(userId: 'USR006', subject: 'usr-driver', roles: ['DRIVER'], vehicleId: 'VEH001');

/// The shape of `GET /api/v1/delivery/trips/{id}` after the expected-units change.
Map<String, Object?> _detail({List<Map<String, Object?>>? stops}) => {
      'tripId': 'TRP02801',
      'status': 'prepared',
      'run': {'id': 'run-1', 'tripId': 'TRP02801', 'vehicleId': 'VEH001', 'depot': 'Peliyagoda Depot', 'status': 'prepared'},
      'stops': stops ??
          [
            {
              'id': 'stop-2',
              'runId': 'run-1',
              'orderRef': 'FR-4811',
              'outletId': 'OUT061',
              'outletName': 'Nugegoda',
              'stopSequence': 2,
              'plannedWindowOpen': '09:00:00',
              'plannedWindowClose': '10:00:00',
              'temperatureRequirement': 'CHILLED',
              'expectedUnits': 8,
              'unitLabel': 'units',
              'status': 'pending',
            },
            {
              'id': 'stop-1',
              'runId': 'run-1',
              'orderRef': 'FR-4801',
              'outletId': 'OUT108',
              'outletName': 'Dehiwala',
              'accessInstructions': 'Rear entrance',
              'dockType': 'Rear dock',
              'parkingConstraint': 'Van only',
              'stopSequence': 1,
              'plannedWindowOpen': '08:00:00',
              'plannedWindowClose': '09:00:00',
              'expectedUnits': 12,
              'unitLabel': 'units',
              'status': 'pending',
            },
          ],
    };

Map<String, Object?> _list(List<Map<String, Object?>> items) => {'items': items};

http.Response _json(Object body, [int status = 200]) => http.Response(jsonEncode(body), status, headers: {'content-type': 'application/json'});

class _Auth implements AuthGateway {
  _Auth({this.token = 'tok'});

  String? token;
  DriverProfile? restored = _driver;
  int signOuts = 0;

  @override
  Future<AuthOutcome> signIn() async => const AuthOutcome.signedIn(_driver);

  @override
  Future<DriverProfile?> restore() async => restored;

  @override
  Future<void> signOut() async => signOuts++;

  @override
  Future<String?> accessToken() async => token;
}

class _Source implements TripSource {
  _Source(this.results);

  final List<TripLoad> results;
  int calls = 0;

  @override
  Future<TripLoad> loadToday() async => results[calls++ < results.length ? calls - 1 : results.length - 1];
}

void main() {
  setUpAll(loadAppFonts);

  group('tripFromJson', () {
    test('maps the server trip to the route, ordered by stop sequence', () {
      final trip = tripFromJson(_detail());
      expect(trip.tripId, 'TRP02801');
      expect(trip.runId, 'run-1');
      expect(trip.vehicleCode, 'VEH001');
      expect(trip.plate, isEmpty);
      expect(trip.vehicleLabel, 'VEH001');
      expect(trip.depot, 'Peliyagoda Depot');
      expect(trip.window, '08:00-10:00');
      expect([for (final s in trip.stops) s.stopId], ['stop-1', 'stop-2']);
      final first = trip.stops.first;
      expect(first.outletCode, 'OUT108');
      expect(first.name, 'Dehiwala');
      expect(first.window, '08:00 - 09:00');
      expect(first.units, 12);
      expect(first.unitsText, '12 units');
      expect(first.accessNote, 'Rear entrance');
      expect(first.contactNote, 'Rear dock · Van only');
      expect(first.orderRef, 'FR-4801');
    });

    test('a stop without a recorded quantity says so instead of showing 0', () {
      final stops = [
        {'id': 'stop-1', 'outletId': 'OUT1', 'stopSequence': 1, 'status': 'pending'},
      ];
      final stop = tripFromJson(_detail(stops: stops)).stops.single;
      expect(stop.units, isNull);
      expect(stop.unitsText, 'Quantity not recorded');
      expect(stop.accessNote, 'No access instructions recorded');
      expect(stop.name, 'OUT1');
    });

    test('stops the server already completed count as done', () {
      final stops = [
        {'id': 'a', 'outletId': 'O1', 'stopSequence': 1, 'status': 'completed'},
        {'id': 'b', 'outletId': 'O2', 'stopSequence': 2, 'status': 'pending'},
      ];
      final trip = tripFromJson(_detail(stops: stops));
      expect(trip.completedStops, 1);
      expect(trip.nextStop?.stopId, 'b');
    });

    test('refuses a trip without the ids needed to send operations', () {
      expect(() => tripFromJson({'tripId': 'T', 'run': <String, Object?>{}, 'stops': <Object?>[]}), throwsFormatException);
      expect(() => tripFromJson(_detail(stops: [{'outletId': 'O1', 'stopSequence': 1}])), throwsFormatException);
    });
  });

  group('TripsApi', () {
    test('lists the driver\'s trips for a date with the bearer token', () async {
      late http.Request seen;
      final api = TripsApi(
        baseUrl: 'http://api/',
        client: MockClient((request) async {
          seen = request;
          return _json(_list([
            {'tripId': 'T1', 'status': 'ready', 'tripNumber': 1},
            {'tripId': 'T2', 'status': 'completed', 'tripNumber': 2},
          ]));
        }),
      );
      final trips = await api.tripsFor('2026-10-03', 'abc');
      expect(seen.url.toString(), 'http://api/api/v1/delivery/drivers/me/trips?date=2026-10-03');
      expect(seen.headers['Authorization'], 'Bearer abc');
      expect([for (final t in trips) t.tripId], ['T1', 'T2']);
      expect(trips.last.isCompleted, isTrue);
    });

    test('maps HTTP and network errors to a failure kind', () async {
      Future<TripsFailureKind> kind(http.Client client) async {
        try {
          await TripsApi(baseUrl: 'http://api', client: client).tripsFor('2026-10-03', 't');
        } on TripsFailure catch (failure) {
          return failure.kind;
        }
        fail('expected a failure');
      }

      expect(await kind(MockClient((_) async => http.Response('', 401))), TripsFailureKind.unauthorized);
      expect(await kind(MockClient((_) async => http.Response('', 403))), TripsFailureKind.forbidden);
      expect(await kind(MockClient((_) async => http.Response('', 404))), TripsFailureKind.notFound);
      expect(await kind(MockClient((_) async => http.Response('', 500))), TripsFailureKind.unavailable);
      expect(await kind(MockClient((_) async => http.Response('not json', 200))), TripsFailureKind.unavailable);
      expect(await kind(MockClient((_) async => throw const SocketException('down'))), TripsFailureKind.unavailable);
    });

    test('reads the trip detail', () async {
      final api = TripsApi(baseUrl: 'http://api', client: MockClient((request) async {
        expect(request.url.path, '/api/v1/delivery/trips/TRP02801');
        return _json(_detail());
      }));
      final trip = await api.trip('TRP02801', 't');
      expect(trip.stops, hasLength(2));
    });
  });

  group('business date', () {
    test('is the Asia/Colombo calendar date whatever the phone timezone is', () {
      // 20:00 UTC on 3 Oct is already 01:30 on 4 Oct in Colombo.
      expect(ApiTripSource.dateKey(DateTime.utc(2026, 10, 3, 20)), '2026-10-04');
      // 18:29 UTC is 23:59 in Colombo, still the 3rd; 18:30 UTC is midnight, the 4th.
      expect(ApiTripSource.dateKey(DateTime.utc(2026, 10, 3, 18, 29)), '2026-10-03');
      expect(ApiTripSource.dateKey(DateTime.utc(2026, 10, 3, 18, 30)), '2026-10-04');
      // The same instant gives the same date from any local offset.
      final instant = DateTime.utc(2026, 1, 1, 0, 10);
      expect(ApiTripSource.dateKey(instant), '2026-01-01');
      expect(ApiTripSource.dateKey(instant.subtract(const Duration(hours: 6))), '2025-12-31');
    });
  });

  group('ApiTripSource', () {
    ApiTripSource source(MockClient client, {_Auth? auth}) =>
        ApiTripSource(api: TripsApi(baseUrl: 'http://api', client: client), auth: auth ?? _Auth(), clock: () => DateTime.utc(2026, 10, 3, 7));

    test('loads the first trip of today that is not finished', () async {
      final paths = <String>[];
      final load = await source(MockClient((request) async {
        paths.add(request.url.toString());
        if (request.url.path.endsWith('/trips')) {
          return _json(_list([
            {'tripId': 'DONE', 'status': 'completed', 'tripNumber': 1},
            {'tripId': 'LATER', 'status': 'ready', 'tripNumber': 3},
            {'tripId': 'TRP02801', 'status': 'prepared', 'tripNumber': 2},
          ]));
        }
        return _json(_detail());
      })).loadToday();
      expect(load.trip?.tripId, 'TRP02801');
      expect(paths.first, contains('date=2026-10-03'));
      expect(paths.last, endsWith('/trips/TRP02801'));
    });

    test('says there is no trip when nothing is open', () async {
      final none = await source(MockClient((_) async => _json(_list([])))).loadToday();
      expect(none.trip, isNull);
      expect(none.failure, isNull);
      final finished = await source(MockClient((_) async => _json(_list([{'tripId': 'D', 'status': 'completed', 'tripNumber': 1}])))).loadToday();
      expect(finished.trip, isNull);
      expect(finished.failure, isNull);
    });

    test('an expired token means signing in again, without calling the server', () async {
      var calls = 0;
      final load = await source(MockClient((_) async {
        calls++;
        return _json(_list([]));
      }), auth: _Auth(token: null)).loadToday();
      expect(load.signInExpired, isTrue);
      expect(calls, 0);
    });

    test('a rejected token and a server error are reported differently', () async {
      final rejected = await source(MockClient((_) async => http.Response('', 401))).loadToday();
      expect(rejected.signInExpired, isTrue);
      final down = await source(MockClient((_) async => http.Response('', 503))).loadToday();
      expect(down.signInExpired, isFalse);
      expect(down.failure, contains('Could not reach Waypoint'));
    });
  });

  group('DriverSession loading the route', () {
    DriverSession session(TripSource source, {_Auth? auth}) =>
        DriverSession(database: InMemoryLocalDatabase(), queue: InMemorySyncQueue(), auth: auth ?? _Auth(), trips: source);

    test('signing in loads the route', () async {
      final trip = tripFromJson(_detail());
      final s = session(_Source([TripLoad.loaded(trip)]));
      expect(s.hasRoute, isFalse);
      await s.signIn();
      expect(s.hasRoute, isTrue);
      expect(s.trip.stops, hasLength(2));
      expect(s.tripsError, isNull);
      expect(s.loadResolved, isFalse);
    });

    test('a failed load keeps the driver signed in and can be retried', () async {
      final trip = tripFromJson(_detail());
      final source = _Source([const TripLoad.failed('Could not reach Waypoint. Check your connection and try again.'), TripLoad.loaded(trip)]);
      final s = session(source);
      await s.signIn();
      expect(s.signedIn, isTrue);
      expect(s.hasRoute, isFalse);
      expect(s.tripsError, contains('Could not reach Waypoint'));
      await s.loadTrips();
      expect(s.hasRoute, isTrue);
      expect(s.tripsError, isNull);
    });

    test('a sign-in the server no longer accepts returns to the sign-in screen', () async {
      final auth = _Auth();
      final s = session(_Source([const TripLoad.failed('Your sign-in was not accepted. Sign in again.', signInExpired: true)]), auth: auth);
      await s.signIn();
      expect(s.signedIn, isFalse);
      expect(s.signInError, contains('Sign in again'));
      expect(auth.signOuts, 1);
    });

    test('restoring a sign-in loads the route too', () async {
      final s = session(_Source([TripLoad.loaded(tripFromJson(_detail()))]));
      await s.restore();
      expect(s.signedIn, isTrue);
      expect(s.hasRoute, isTrue);
    });

    test('a resumed run starts after the stops the server already completed', () async {
      final stops = [
        {'id': 'a', 'outletId': 'O1', 'stopSequence': 1, 'status': 'completed'},
        {'id': 'b', 'outletId': 'O2', 'stopSequence': 2, 'status': 'pending'},
      ];
      final s = session(_Source([TripLoad.loaded(tripFromJson(_detail(stops: stops)))]));
      await s.signIn();
      expect(s.trip.completedStops, 1);
      expect(s.trip.nextStop?.stopId, 'b');
      expect(s.routeComplete, isFalse);
    });

    test('signing out clears the route', () async {
      final s = session(_Source([TripLoad.loaded(tripFromJson(_detail()))]));
      await s.signIn();
      await s.finishTrip();
      expect(s.hasRoute, isFalse);
    });
  });

  group('screens', () {
    Future<void> boot(WidgetTester tester, TripSource source) async {
      await pumpScreen(tester, WaypointDriverApp(database: InMemoryLocalDatabase(), queue: InMemorySyncQueue(), auth: _Auth()..restored = null, trips: source));
      await tester.tap(find.text('Continue to sign in'));
      await tester.pumpAndSettle();
    }

    testWidgets('a loaded route opens the load check with server data', (tester) async {
      await boot(tester, _Source([TripLoad.loaded(tripFromJson(_detail()))]));
      expect(find.text('Check the load before you leave'), findsOneWidget);
      expect(find.text('Stop 1 · Dehiwala · 12 units on board'), findsOneWidget);
    });

    testWidgets('no trip today says so and offers a retry', (tester) async {
      await boot(tester, _Source([const TripLoad.none()]));
      expect(find.text('No route loaded'), findsOneWidget);
      expect(find.text('No trip for you today'), findsOneWidget);
      expect(find.text('Try again'), findsOneWidget);
    });

    testWidgets('a failed load shows the reason and Try again loads the route', (tester) async {
      final source = _Source([const TripLoad.failed('Could not reach Waypoint. Check your connection and try again.'), TripLoad.loaded(tripFromJson(_detail()))]);
      await boot(tester, source);
      expect(find.text('Could not load your route'), findsOneWidget);
      expect(find.textContaining('Could not reach Waypoint'), findsOneWidget);
      await tester.tap(find.text('Try again'));
      await tester.pumpAndSettle();
      expect(find.text('Check the load before you leave'), findsOneWidget);
      expect(source.calls, 2);
    });
    testWidgets('a partial delivery is not offered a made-up quantity when the server has none', (tester) async {
      final stops = [
        {'id': 'stop-1', 'outletId': 'OUT1', 'outletName': 'Dehiwala', 'stopSequence': 1, 'status': 'pending'},
      ];
      final queue = InMemorySyncQueue();
      await pumpScreen(tester, WaypointDriverApp(database: InMemoryLocalDatabase(), queue: queue, auth: _Auth()..restored = null, trips: _Source([TripLoad.loaded(tripFromJson(_detail(stops: stops)))])));
      await tester.tap(find.text('Continue to sign in'));
      await tester.pumpAndSettle();
      await tester.tap(find.text('Confirm load on board'));
      await tester.pumpAndSettle();
      await tester.tap(find.text('View Stop'));
      await tester.pumpAndSettle();
      await tester.tap(find.text('I’ve stopped safely'));
      await tester.pumpAndSettle();
      expect(find.text('Expected: not recorded'), findsOneWidget);
      await tester.tap(find.text('Partial'));
      await tester.pump();
      await tester.tap(find.text('Save delivery'));
      await tester.pumpAndSettle();
      expect(find.textContaining('expected quantity for this stop is not recorded'), findsOneWidget);
      expect(find.text('Saved on this device'), findsNothing);
      expect((await queue.pending()).map((e) => e.action), ['ARRIVED']);
    });
  });
}
