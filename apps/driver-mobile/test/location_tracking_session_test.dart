import 'dart:async';

import 'package:flutter_test/flutter_test.dart';
import 'package:waypoint_driver/app/driver_session.dart';
import 'package:waypoint_driver/auth/auth_gateway.dart';
import 'package:waypoint_driver/auth/profile_api.dart';
import 'package:waypoint_driver/data/driver_models.dart';
import 'package:waypoint_driver/location/location_reporter.dart';
import 'package:waypoint_driver/main.dart';
import 'package:waypoint_driver/offline/local_database.dart';
import 'package:waypoint_driver/sync/sync.dart';
import 'package:waypoint_driver/trips/trip_source.dart';
import 'package:waypoint_driver/trips/trip_start.dart';
import 'package:waypoint_driver/trips/trips_api.dart';

import 'helpers/render.dart';

const _driver = DriverProfile(userId: 'USR006', subject: 'usr-driver', roles: ['DRIVER'], vehicleId: 'VEH001');

Map<String, Object?> _detail({String status = 'prepared', String tripId = 'trip-1'}) => {
      'tripId': tripId,
      'currentPlanVersion': 1,
      'run': {'id': 'run-1', 'tripId': tripId, 'planId': 'plan-1', 'planRef': 'PLAN000001', 'vehicleId': 'VEH001', 'status': status},
      'stops': [
        {'id': 'stop-1', 'orderId': 'order-1', 'outletId': 'O1', 'outletName': 'Dehiwala', 'stopSequence': 1, 'expectedUnits': 10, 'status': 'pending'},
      ],
    };

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

class _Trips implements TripSource {
  _Trips(this.detail);
  Map<String, Object?> detail;
  @override
  Future<TripLoad> loadToday() async => TripLoad.loaded(tripFromJson(detail));
}

class _Starter implements TripStarter {
  @override
  Future<TripStartResult> start(TripInfo trip, {required String operationId, List<String>? confirmedOrderIds}) async => const TripStartResult(TripStartStatus.started);
}

class _Position implements PositionSource {
  LocationAccess current = LocationAccess.granted;
  final controller = StreamController<Fix>.broadcast();
  @override
  Future<LocationAccess> access() async => current;
  @override
  Future<LocationAccess> requestAccess() async => current = LocationAccess.granted;
  @override
  Stream<Fix> positions() => controller.stream;
}

class _Sink implements PositionSink {
  final sent = <String>[];
  @override
  Future<SendResult> send(String tripId, Fix fix) async {
    sent.add(tripId);
    return SendResult.sent;
  }
}

Future<void> _settle() async {
  for (var i = 0; i < 4; i++) {
    await Future<void>.delayed(Duration.zero);
  }
}

void main() {
  setUpAll(loadAppFonts);

  late _Position position;
  late _Sink sink;
  late LocationReporter reporter;

  setUp(() {
    position = _Position();
    sink = _Sink();
    reporter = LocationReporter(source: position, sink: sink);
  });

  DriverSession session({Map<String, Object?>? detail}) => DriverSession(
        database: InMemoryLocalDatabase(),
        queue: InMemorySyncQueue(),
        auth: _Auth(),
        trips: _Trips(detail ?? _detail()),
        starter: _Starter(),
        location: reporter,
      );

  group('position sharing follows the trip', () {
    test('nothing is shared before the trip has started', () async {
      final s = session();
      await s.signIn();
      await _settle();
      expect(s.tripStartState, isNot(TripStartState.started));
      expect(reporter.tripId, isNull);
      expect(reporter.state, LocationShareState.off);
    });

    test('starting the trip starts sharing for that trip, and a position reaches Waypoint', () async {
      final s = session();
      await s.signIn();
      await s.confirmLoad();
      await s.startTrip();
      await _settle();
      expect(s.tripStartState, TripStartState.started);
      expect(reporter.tripId, 'trip-1');
      expect(reporter.state, LocationShareState.sharing);
      position.controller.add(Fix(latitude: 6.93, longitude: 79.86, at: DateTime.now().toUtc()));
      await _settle();
      expect(sink.sent, ['trip-1']);
    });

    test('a trip that was already running when the app opened is shared too', () async {
      final s = session(detail: _detail(status: 'in_progress'));
      await s.signIn();
      await _settle();
      expect(s.tripStartState, TripStartState.started);
      expect(reporter.tripId, 'trip-1');
      expect(reporter.state, LocationShareState.sharing);
    });

    test('signing out stops sharing at once', () async {
      final s = session();
      await s.signIn();
      await s.confirmLoad();
      await s.startTrip();
      await _settle();
      expect(reporter.sharing, isTrue);
      await s.finishTrip(force: true);
      await _settle();
      expect(reporter.state, LocationShareState.off);
      expect(reporter.tripId, isNull);
      position.controller.add(Fix(latitude: 6.94, longitude: 79.87, at: DateTime.now().toUtc()));
      await _settle();
      expect(sink.sent, isEmpty);
    });

    test('a driver who has not allowed location is asked once the trip is running, and the trip works either way', () async {
      position.current = LocationAccess.denied;
      final s = session();
      await s.signIn();
      await s.confirmLoad();
      await s.startTrip();
      await _settle();
      expect(s.tripStartState, TripStartState.started);
      expect(reporter.state, LocationShareState.askFirst);
      reporter.decline();
      expect(s.tripStartState, TripStartState.started);
      expect(sink.sent, isEmpty);
    });

    test('sessions without position sharing (demo and tests) are unchanged', () async {
      final s = DriverSession(database: InMemoryLocalDatabase(), queue: InMemorySyncQueue(), auth: _Auth(), trips: _Trips(_detail()), starter: _Starter());
      await s.signIn();
      await s.confirmLoad();
      await s.startTrip();
      expect(s.tripStartState, TripStartState.started);
    });
  });

  group('the route screen', () {
    Future<void> openRoute(WidgetTester tester, {LocationAccess access = LocationAccess.granted}) async {
      position.current = access;
      await pumpScreen(
        tester,
        WaypointDriverApp(database: InMemoryLocalDatabase(), queue: InMemorySyncQueue(), auth: _Auth(), trips: _Trips(_detail()), starter: _Starter(), location: reporter),
      );
      await tester.tap(find.text('Continue to sign in'));
      await tester.pumpAndSettle();
      await tester.tap(find.text('Confirm load on board'));
      await tester.pumpAndSettle();
    }

    testWidgets('shows that location is being shared once the trip runs', (tester) async {
      await openRoute(tester);
      expect(find.text('Sharing your location'), findsOneWidget);
      expect(find.text('Share your location with dispatch?'), findsNothing);
    });

    testWidgets('explains before the phone asks, and shares after Share my location', (tester) async {
      await openRoute(tester, access: LocationAccess.denied);
      expect(find.text('Share your location with dispatch?'), findsOneWidget);
      expect(find.textContaining('stops when you finish the trip or sign out'), findsOneWidget);
      await tester.tap(find.text('Share my location'));
      await tester.pumpAndSettle();
      expect(find.text('Sharing your location'), findsOneWidget);
      expect(reporter.sharing, isTrue);
    });

    testWidgets('Not now keeps the trip going and says location is not shared', (tester) async {
      await openRoute(tester, access: LocationAccess.denied);
      await tester.tap(find.text('Not now'));
      await tester.pumpAndSettle();
      expect(find.text('Location is not shared'), findsOneWidget);
      expect(find.text('Share your location with dispatch?'), findsNothing);
      expect(reporter.sharing, isFalse);
    });

    testWidgets('location switched off on the phone says so and the trip still works', (tester) async {
      await openRoute(tester, access: LocationAccess.serviceOff);
      expect(find.text('Location is off on this phone'), findsOneWidget);
    });
  });
}
