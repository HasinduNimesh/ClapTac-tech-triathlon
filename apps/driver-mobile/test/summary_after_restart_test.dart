import 'package:flutter_test/flutter_test.dart';
import 'package:waypoint_driver/app/driver_session.dart';
import 'package:waypoint_driver/auth/auth_gateway.dart';
import 'package:waypoint_driver/auth/profile_api.dart';
import 'package:waypoint_driver/data/driver_models.dart';
import 'package:waypoint_driver/offline/local_database.dart';
import 'package:waypoint_driver/sync/sync.dart';
import 'package:waypoint_driver/trips/trip_source.dart';
import 'package:waypoint_driver/trips/trip_start.dart';
import 'package:waypoint_driver/trips/trips_api.dart';

const _driver = DriverProfile(userId: 'USR519D339C1561', subject: 'waypoint-person-x', roles: ['DRIVER'], vehicleId: 'VEH035');

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
  _Source(this.detail);
  final Map<String, Object?> detail;
  @override
  Future<TripLoad> loadToday() async => TripLoad.loaded(tripFromJson(detail));
}

class _Starter implements TripStarter {
  @override
  Future<TripStartResult> start(TripInfo trip, {required String operationId, List<String>? confirmedOrderIds}) async => const TripStartResult(TripStartStatus.started);
}

// The trip as the server reports it after both stops were finished, which is what a restarted app loads.
Map<String, Object?> _finishedTrip(List<Map<String, Object?>> stops) => {
      'tripId': 'trip-1',
      'currentPlanVersion': 1,
      'run': {'id': 'run-1', 'tripId': 'trip-1', 'planId': 'plan-1', 'planRef': 'PLAN000003', 'vehicleId': 'VEH035', 'status': 'in_progress'},
      'stops': stops,
    };

Map<String, Object?> _stop(String id, int seq, {required int expected, String? outcome, int? delivered}) => {
      'id': id,
      'orderId': 'order-$seq',
      'outletId': 'OUT001',
      'outletName': 'Fresh OUT001',
      'stopSequence': seq,
      'expectedUnits': expected,
      'status': outcome == null ? 'pending' : 'completed',
      if (outcome != null) 'outcomeCode': outcome,
      if (delivered != null) 'deliveredUnits': delivered,
    };

void main() {
  DriverSession session(Map<String, Object?> detail) => DriverSession(
        database: InMemoryLocalDatabase(),
        queue: InMemorySyncQueue(),
        auth: _Auth(),
        trips: _Source(detail),
        starter: _Starter(),
      );

  test('a restarted app reports the outcomes the server recorded instead of zeros', () async {
    final s = session(_finishedTrip([
      _stop('stop-1', 1, expected: 34, outcome: 'PARTIAL', delivered: 33),
      _stop('stop-2', 2, expected: 40, outcome: 'DELIVERED', delivered: 40),
    ]));
    await s.signIn();
    final summary = s.summary;
    expect(summary.totalStops, 2);
    expect(summary.delivered, 1);
    expect(summary.partial, 1);
    expect(summary.failedOrRefused, 0);
    expect(summary.completed.single.status, 'Delivered');
    expect(summary.unresolved.single.status, 'Partial delivery');
    expect(summary.unresolved.single.detail, '1 unit short - sent to Waypoint', reason: 'it came from the server, so it cannot be waiting on this phone');
  });

  test('failed and refused stops come from the server too, and a stop still to do counts as nothing', () async {
    final s = session(_finishedTrip([
      _stop('stop-1', 1, expected: 10, outcome: 'FAILED'),
      _stop('stop-2', 2, expected: 10, outcome: 'REFUSED'),
      _stop('stop-3', 3, expected: 10),
    ]));
    await s.signIn();
    final summary = s.summary;
    expect(summary.failedOrRefused, 2);
    expect(summary.delivered, 0);
    expect(summary.unresolved.map((r) => r.status), ['Failed delivery', 'Refused']);
    expect(summary.unresolved.every((r) => r.detail == 'Sent to Waypoint'), isTrue);
  });

  test('the outcome survives saving the route and reading it back', () {
    final stop = tripFromJson(_finishedTrip([_stop('stop-1', 1, expected: 34, outcome: 'PARTIAL', delivered: 33)])).stops.single;
    expect(stop.outcomeCode, 'PARTIAL');
    expect(stop.deliveredUnits, 33);
    final again = StopInfo.fromJson(stop.toJson());
    expect(again.outcomeCode, 'PARTIAL');
    expect(again.deliveredUnits, 33);
    expect(again.orderId, 'order-1');
  });
}
