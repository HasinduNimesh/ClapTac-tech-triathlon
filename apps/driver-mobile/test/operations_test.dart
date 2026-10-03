import 'dart:math';

import 'package:flutter_test/flutter_test.dart';
import 'package:waypoint_driver/data/driver_models.dart';
import 'package:waypoint_driver/data/sample_data.dart';
import 'package:waypoint_driver/sync/operations.dart';

final _at = DateTime.utc(2026, 10, 3, 4, 30);
final _stop = sampleStops.first;

void main() {
  group('operation ids', () {
    test('look like version-4 UUIDs', () {
      final id = newOperationId(Random(7));
      expect(RegExp(r'^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$').hasMatch(id), isTrue, reason: id);
    });

    test('are unique', () {
      final ids = {for (var i = 0; i < 200; i++) newOperationId()};
      expect(ids, hasLength(200));
    });
  });

  group('the sync object follows the delivery contract', () {
    test('arrival is keyed by the server stop id, not the outlet code', () {
      final json = Operations.arrived(operationId: 'op-1', trip: sampleTrip, stop: _stop, occurredAt: _at).toSyncJson();
      expect(json, {
        'operationId': 'op-1',
        'type': 'ARRIVED',
        'tripId': 'sample-trip-TRP02801',
        'runId': 'sample-run-TRP02801',
        'stopId': 'sample-stop-1',
        'occurredAt': '2026-10-03T04:30:00.000Z',
        'payload': <String, Object?>{},
      });
      expect(json.values, isNot(contains(_stop.outletCode)));
    });

    test('a delivered outcome carries only the code, and no invented proof dependency', () {
      final op = Operations.stopOutcome(
        operationId: 'op-2',
        trip: sampleTrip,
        stop: _stop,
        draft: const DeliveryDraft(outcome: DeliveryOutcome.delivered),
        occurredAt: _at,
      );
      expect(op.payload, {'code': 'DELIVERED'});
      expect(op.toSyncJson().containsKey('dependsOnOperationId'), isFalse);
    });

    test('a proof operation id is passed through as the dependency', () {
      final op = Operations.stopOutcome(
        operationId: 'op-2',
        trip: sampleTrip,
        stop: _stop,
        draft: const DeliveryDraft(outcome: DeliveryOutcome.delivered),
        occurredAt: _at,
        proofOperationId: 'proof-1',
      );
      expect(op.toSyncJson()['dependsOnOperationId'], 'proof-1');
    });

    test('a partial delivery keeps the quantity in the note because the API has no field for it', () {
      final op = Operations.stopOutcome(
        operationId: 'op-3',
        trip: sampleTrip,
        stop: _stop,
        draft: const DeliveryDraft(outcome: DeliveryOutcome.partial, quantity: 5, notes: 'Two cartons damaged'),
        occurredAt: _at,
      );
      expect(op.payload['code'], 'PARTIAL');
      expect(op.payload['note'], 'Received 5 of 12 cartons. Two cartons damaged');
      expect(op.payload.containsKey('reason'), isFalse);
    });

    test('failed and refused deliveries always carry a valid reason code', () {
      const valid = {'OUTLET_CLOSED', 'ACCESS_BLOCKED', 'RECEIVER_UNAVAILABLE', 'GOODS_REJECTED', 'VEHICLE_ISSUE', 'OTHER'};
      for (final outcome in [DeliveryOutcome.failed, DeliveryOutcome.refused]) {
        final reason = Operations.stopOutcome(
          operationId: 'op',
          trip: sampleTrip,
          stop: _stop,
          draft: DeliveryDraft(outcome: outcome),
          occurredAt: _at,
        ).payload['reason'];
        expect(valid, contains(reason), reason: outcome.name);
      }
    });

    test('the default reasons are goods rejected for a refusal and other for a failure', () {
      expect(reasonFor(const DeliveryDraft(outcome: DeliveryOutcome.refused)), 'GOODS_REJECTED');
      expect(reasonFor(const DeliveryDraft(outcome: DeliveryOutcome.failed)), 'OTHER');
      expect(reasonFor(const DeliveryDraft(outcome: DeliveryOutcome.failed, reason: 'OUTLET_CLOSED')), 'OUTLET_CLOSED');
      expect(reasonFor(const DeliveryDraft(outcome: DeliveryOutcome.delivered)), isNull);
    });

    test('outcome codes match the API', () {
      expect(DeliveryOutcome.values.map(outcomeCode), ['DELIVERED', 'PARTIAL', 'FAILED', 'REFUSED']);
    });

    test('route completion has no stop', () {
      final json = Operations.routeCompleted(operationId: 'op-4', trip: sampleTrip, occurredAt: _at).toSyncJson();
      expect(json['type'], 'ROUTE_COMPLETED');
      expect(json.containsKey('stopId'), isFalse);
    });

    test('an incident has the stop at the top level and only category and description inside', () {
      final json = Operations.incident(
        operationId: 'op-5',
        trip: sampleTrip,
        category: 'VEHICLE',
        description: 'Vehicle breakdown',
        occurredAt: _at,
        stop: _stop,
      ).toSyncJson();
      expect(json['stopId'], 'sample-stop-1');
      expect(json['payload'], {'category': 'VEHICLE', 'description': 'Vehicle breakdown'});
    });

    test('an incident away from a stop has no stop id', () {
      final json = Operations.incident(operationId: 'op-6', trip: sampleTrip, category: 'ROAD', description: 'Flooded', occurredAt: _at).toSyncJson();
      expect(json.containsKey('stopId'), isFalse);
    });

    test('times are sent in UTC', () {
      final local = DateTime(2026, 10, 3, 10, 0);
      final json = Operations.routeCompleted(operationId: 'op', trip: sampleTrip, occurredAt: local).toSyncJson();
      expect(json['occurredAt'], local.toUtc().toIso8601String());
    });
  });

  test('a queued event carries the full sync object so a worker can send it unchanged', () {
    final op = Operations.arrived(operationId: 'op-1', trip: sampleTrip, stop: _stop, occurredAt: _at);
    final event = op.toSyncEvent();
    expect(event.idempotencyKey, 'op-1');
    expect(event.action, 'ARRIVED');
    expect(event.resourceType, 'stop');
    expect(event.resourceId, 'sample-stop-1');
    expect(event.payload, op.toSyncJson());
    expect(event.occurredAt, _at);
  });

  test('a trip-level event is keyed by the trip', () {
    final event = Operations.routeCompleted(operationId: 'op-9', trip: sampleTrip, occurredAt: _at).toSyncEvent();
    expect(event.resourceType, 'trip');
    expect(event.resourceId, 'sample-trip-TRP02801');
  });
}
