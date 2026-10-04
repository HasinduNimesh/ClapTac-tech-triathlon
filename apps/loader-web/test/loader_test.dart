import 'package:flutter_test/flutter_test.dart';
import 'package:waypoint_loader/api/api_client.dart';
import 'package:waypoint_loader/shared/models.dart';

void main() {
  test('a shortfall only allows departure after a partial-load decision', () {
    LoadingOrder order(String? decision) => LoadingOrder({
          'orderId': 'o1',
          'status': 'shortfall',
          'issues': [
            {'id': 'i1', 'type': 'MISSING', 'affectedUnits': 2, if (decision != null) 'decision': decision},
          ],
        });
    expect(order(null).unresolved, isTrue);
    expect(order('HOLD').unresolved, isTrue);
    expect(order('PARTIAL_LOAD').unresolved, isFalse);
    // Moving the line to the next run only releases the trip once the new plan is
    // acknowledged and the line has left this trip, so until then it stays open.
    expect(order('MOVE_TO_NEXT_RUN').unresolved, isTrue);
  });

  test('server problem types become loader-facing messages', () {
    final e = ApiException(409, '{"type":"dispatcher_decision_required","detail":"x"}');
    expect(e.type, 'dispatcher_decision_required');
  });

  test('a revised plan explains each changed line', () {
    final trip = LoadingTrip({
      'tripId': 't1', 'planVersion': 3, 'preparedPlanVersion': 2, 'acknowledgedVersion': 0, 'planChanged': true,
      'changes': [
        {'kind': 'MOVED', 'outletId': 'OUT009', 'fromStop': 4, 'toStop': 2, 'loaded': true},
        {'kind': 'ADDED', 'orderRef': 'ORD7', 'outletId': 'OUT005', 'toStop': 3},
      ],
    });
    expect(trip.needsAck, isTrue);
    expect(trip.changes.first.text, 'Stop 4 OUT009 moved to Stop 2 — already loaded, reposition it');
    expect(trip.changes.last.text, 'ORD7 (OUT005) added at Stop 3');
  });

  test('a partial load counts what goes on the truck', () {
    final o = LoadingOrder({
      'orderId': 'o1', 'expectedUnits': 120, 'status': 'shortfall',
      'issues': [{'id': 'i1', 'type': 'MISSING', 'affectedUnits': 24, 'decision': 'PARTIAL_LOAD'}],
    });
    expect(o.loadedUnits, 96);
    expect(o.issues.first.typeLabel, 'missing');
  });

  test('planned times show in Sri Lanka time', () {
    expect(clock(DateTime.utc(2026, 10, 1, 22, 0)), '03:30');
    expect(dockLabel('mall_bay'), 'Mall bay');
  });
}
