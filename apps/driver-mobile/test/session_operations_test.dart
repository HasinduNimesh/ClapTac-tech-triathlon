import 'package:flutter_test/flutter_test.dart';
import 'package:waypoint_driver/app/demo_flags.dart';
import 'package:waypoint_driver/app/driver_session.dart';
import 'package:waypoint_driver/auth/auth_failure.dart';
import 'package:waypoint_driver/auth/auth_gateway.dart';
import 'package:waypoint_driver/auth/profile_api.dart';
import 'package:waypoint_driver/data/driver_models.dart';
import 'package:waypoint_driver/data/sample_data.dart';
import 'package:waypoint_driver/offline/local_database.dart';
import 'package:waypoint_driver/sync/sync.dart';

final _at = DateTime.utc(2026, 10, 3, 4, 30);

DriverSession _session({TripInfo? trip, InMemorySyncQueue? queue, bool demoRoute = true}) {
  var n = 0;
  return DriverSession(
    database: InMemoryLocalDatabase(),
    queue: queue ?? InMemorySyncQueue(),
    trip: trip,
    demoRoute: demoRoute,
    clock: () => _at,
    newId: () => 'id-${++n}',
  );
}

Map<String, Object?> _payload(SyncEvent e) => (e.payload['payload'] as Map).cast<String, Object?>();

/// A trip that visits the same outlet twice, which must not make the two stops collide.
const _twice = TripInfo(
  tripId: 'trip-1',
  runId: 'run-1',
  vehicleCode: 'VEH001',
  plate: 'WP AB-1234',
  tripRef: 'TRP1',
  depot: 'Depot',
  window: '08:00-11:00',
  stops: [
    StopInfo(stopId: 'stop-a', sequence: 1, outletCode: 'OUT1', name: 'Fresh', windowStart: '08:00', windowEnd: '09:00', units: 10, accessNote: '', contactNote: '', goods: 'Ambient'),
    StopInfo(stopId: 'stop-b', sequence: 2, outletCode: 'OUT1', name: 'Fresh', windowStart: '10:00', windowEnd: '11:00', units: 4, accessNote: '', contactNote: '', goods: 'Chilled'),
  ],
);

void main() {
  group('operations are keyed by the server ids', () {
    test('two visits to one outlet are separate records', () async {
      final queue = InMemorySyncQueue();
      final session = _session(trip: _twice, queue: queue);
      await session.markArrived(_twice.stops[0]);
      await session.recordDelivery(_twice.stops[0], const DeliveryDraft(outcome: DeliveryOutcome.delivered));
      await session.markArrived(_twice.stops[1]);
      await session.recordDelivery(_twice.stops[1], const DeliveryDraft(outcome: DeliveryOutcome.refused));

      final events = await queue.pending();
      expect(events.map((e) => '${e.action}:${e.payload['stopId']}'), [
        'ARRIVED:stop-a',
        'STOP_OUTCOME:stop-a',
        'ARRIVED:stop-b',
        'STOP_OUTCOME:stop-b',
      ]);
      expect(session.trip.completedStops, 2);
      expect(session.routeComplete, isTrue);
      expect(events.every((e) => e.payload['tripId'] == 'trip-1' && e.payload['runId'] == 'run-1'), isTrue);
    });

    test('nothing is keyed by the outlet code', () async {
      final queue = InMemorySyncQueue();
      final session = _session(trip: _twice, queue: queue);
      await session.recordDelivery(_twice.stops[0], const DeliveryDraft(outcome: DeliveryOutcome.delivered));
      final json = (await queue.pending()).single.payload.toString();
      expect(json, isNot(contains('OUT1')));
    });

    test('the queue holds no invented proof and no pre-contract actions', () async {
      final queue = InMemorySyncQueue();
      final session = _session(queue: queue);
      await session.recordDelivery(sampleStops.first, const DeliveryDraft(outcome: DeliveryOutcome.delivered, hasPhoto: true, hasSignature: true));
      final events = await queue.pending();
      expect(events.map((e) => e.action), ['STOP_OUTCOME']);
      expect(events.single.payload.containsKey('dependsOnOperationId'), isFalse);
      expect(events.map((e) => e.action), isNot(contains('proof.create')));
      expect(events.map((e) => e.action), isNot(contains('delivery.outcome_changed')));
      expect(events.single.payload.toString(), isNot(contains('local/')));
    });
  });

  group('operation ids are stable', () {
    test('arriving twice at a stop is one operation', () async {
      final queue = InMemorySyncQueue();
      final session = _session(queue: queue);
      await session.markArrived(sampleStops.first);
      await session.markArrived(sampleStops.first);
      expect(await queue.pending(), hasLength(1));
    });

    test('recording a stop again keeps the same operation id', () async {
      final queue = InMemorySyncQueue();
      final session = _session(queue: queue);
      await session.recordDelivery(sampleStops.first, const DeliveryDraft(outcome: DeliveryOutcome.delivered));
      final first = (await queue.pending()).single.idempotencyKey;
      await session.recordDelivery(sampleStops.first, const DeliveryDraft(outcome: DeliveryOutcome.delivered));
      final events = await queue.pending();
      expect(events, hasLength(1));
      expect(events.single.idempotencyKey, first);
    });

    test('each problem report is its own operation', () async {
      final queue = InMemorySyncQueue();
      final session = _session(queue: queue);
      await session.reportProblem(const ProblemReport(kind: ProblemKind.roadBlocked));
      await session.reportProblem(const ProblemReport(kind: ProblemKind.roadBlocked));
      expect((await queue.pending()).map((e) => e.idempotencyKey).toSet(), hasLength(2));
    });
  });

  group('incidents', () {
    test('a report away from a stop has no stop id, and category and description inside', () async {
      final queue = InMemorySyncQueue();
      final session = _session(queue: queue);
      await session.reportProblem(const ProblemReport(kind: ProblemKind.roadBlocked, note: 'Flooded at the bridge'));
      final event = (await queue.pending()).single;
      expect(event.payload.containsKey('stopId'), isFalse);
      expect(_payload(event), {'category': 'ROAD', 'description': 'Road blocked or flooded: Flooded at the bridge'});
    });

    test('a report at a stop carries the stop id at the top level', () async {
      final queue = InMemorySyncQueue();
      final session = _session(queue: queue);
      await session.reportProblem(const ProblemReport(kind: ProblemKind.outletClosed), stop: sampleStops.first);
      final event = (await queue.pending()).single;
      expect(event.payload['stopId'], 'sample-stop-1');
      expect(_payload(event).containsKey('stopId'), isFalse);
    });
  });

  group('finishing the trip', () {
    test('queues route completion when every stop is recorded, and keeps what is unsent', () async {
      final queue = InMemorySyncQueue();
      final session = _session(trip: _twice, queue: queue);
      for (final stop in _twice.stops) {
        await session.recordDelivery(stop, const DeliveryDraft(outcome: DeliveryOutcome.delivered));
      }
      await session.finishTrip();
      final actions = (await queue.pending()).map((e) => e.action).toList();
      expect(actions, ['STOP_OUTCOME', 'STOP_OUTCOME', 'ROUTE_COMPLETED']);
    });

    test('does not claim completion when stops are missing', () async {
      final queue = InMemorySyncQueue();
      final session = _session(trip: _twice, queue: queue);
      await session.recordDelivery(_twice.stops.first, const DeliveryDraft(outcome: DeliveryOutcome.delivered));
      await session.finishTrip();
      expect((await queue.pending()).map((e) => e.action), isNot(contains('ROUTE_COMPLETED')));
    });
  });

  group('the app only claims what happened', () {
    test('the summary says saved on this phone, never that dispatch was notified', () async {
      final session = _session(trip: _twice);
      await session.recordDelivery(_twice.stops[0], const DeliveryDraft(outcome: DeliveryOutcome.partial, quantity: 6));
      await session.recordDelivery(_twice.stops[1], const DeliveryDraft(outcome: DeliveryOutcome.refused, notes: 'Store closed'));
      final details = session.summary.unresolved.map((r) => r.detail).toList();
      expect(details, ['4 units short - saved on this phone, not sent yet', 'Store closed - saved on this phone, not sent yet']);
      expect(details.join(), isNot(contains('notified')));
    });

    test('updates describe delivery records as not sent and never mention a photo', () async {
      final session = _session();
      await session.recordDelivery(sampleStops.first, const DeliveryDraft(outcome: DeliveryOutcome.delivered, hasPhoto: true));
      final item = session.updates.first;
      expect(item.title, contains('not sent yet'));
      expect('${item.title} ${item.detail}'.toLowerCase(), isNot(contains('photo')));
    });

    test('the pending count covers delivery records and reports, not photos', () async {
      final session = _session();
      await session.recordDelivery(sampleStops.first, const DeliveryDraft(outcome: DeliveryOutcome.delivered, hasPhoto: true));
      await session.reportProblem(const ProblemReport(kind: ProblemKind.safetyConcern));
      expect(session.pendingUploads, 2);
      expect(session.summary.pendingUploads, 2);
    });
  });

  group('sample data is only for explicit demos', () {
    test('a real sign-in with no demo route has no route', () {
      final session = DriverSession(database: InMemoryLocalDatabase(), queue: InMemorySyncQueue(), auth: _NoAuth());
      expect(session.hasRoute, isFalse);
    });

    test('the demo route and demo sign-in show the sample trip', () {
      expect(_session().hasRoute, isTrue);
      expect(DriverSession(database: InMemoryLocalDatabase(), queue: InMemorySyncQueue(), demoAuth: true).hasRoute, isTrue);
    });

    test('a build with no demo switches and no trip has no route', () {
      expect(DriverSession(database: InMemoryLocalDatabase(), queue: InMemorySyncQueue()).hasRoute, isFalse);
    });

    test('sample ids can never be mistaken for server ids', () {
      expect(sampleTrip.tripId, startsWith('sample-'));
      expect(sampleTrip.runId, startsWith('sample-'));
      expect(sampleStops.every((s) => s.stopId.startsWith('sample-')), isTrue);
    });
  });

  group('demo switches', () {
    test('are all forced off in a release build, whatever was requested', () {
      final flags = DemoFlags.forBuild(releaseMode: true, auth: true, route: true, updates: true);
      expect(flags.auth, isFalse);
      expect(flags.route, isFalse);
      expect(flags.updates, isFalse);
    });

    test('pass through in a debug build', () {
      final flags = DemoFlags.forBuild(releaseMode: false, auth: true, route: true, updates: true);
      expect([flags.auth, flags.route, flags.updates], [true, true, true]);
    });

    test('default to off', () {
      final flags = DemoFlags.forBuild(releaseMode: false);
      expect([flags.auth, flags.route, flags.updates], [false, false, false]);
    });
  });
}

class _NoAuth implements AuthGateway {
  @override
  Future<AuthOutcome> signIn() async => const AuthOutcome.failed(AuthFailure(AuthFailureKind.cancelled));

  @override
  Future<DriverProfile?> restore() async => null;

  @override
  Future<void> signOut() async {}

  @override
  Future<String?> accessToken() async => 'token';
}
