import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:waypoint_loader/api/api_client.dart';
import 'package:waypoint_loader/loader/loader_controller.dart';

void main() {
  // The server starts with no trips; the dispatcher then publishes a plan.
  late List<Map<String, dynamic>> published;
  late int tripListCalls;

  LoaderController controller({bool failing = false}) {
    published = [];
    tripListCalls = 0;
    final client = MockClient((http.Request request) async {
      final path = request.url.path;
      if (failing) return http.Response('{"detail":"down"}', 500);
      if (path.endsWith('/loading/trips')) {
        tripListCalls++;
        return http.Response(jsonEncode({'items': published}), 200);
      }
      if (path.endsWith('/loading/alerts')) return http.Response(jsonEncode({'items': []}), 200);
      final id = path.split('/').last;
      return http.Response(jsonEncode({'tripId': id, 'vehicleId': 'VEH035', 'orders': []}), 200);
    });
    return LoaderController(api: ApiClient(tokenProvider: () async => 't', client: client, baseUrl: 'http://test/api/v1'));
  }

  test('a plan published after the screen opened appears without a manual reload', () async {
    final c = controller();
    await c.load();
    expect(c.trips, isEmpty);

    published = [
      {'tripId': 'trip-1', 'vehicleId': 'VEH035', 'tripNumber': 1, 'loadingStatus': 'pending'},
    ];
    await c.refreshQuietly();
    expect(c.trips.map((t) => t.tripId), ['trip-1']);
    expect(c.details.containsKey('trip-1'), isTrue);
    expect(c.loading, isFalse, reason: 'a background refresh must not show the spinner');
  });

  test('a background refresh stays out of the way while the loader is mid-action', () async {
    final c = controller();
    await c.load();
    final before = tripListCalls;
    c.busy = true;
    await c.refreshQuietly();
    expect(tripListCalls, before, reason: 'no request while an action is on its way');

    c.busy = false;
    final attempt = ReportAttempt();
    c.reportFormOpened(attempt);
    await c.refreshQuietly();
    expect(tripListCalls, before, reason: 'no request while a report form is open');
  });

  test('a failed background refresh keeps what the loader is looking at', () async {
    final c = controller();
    published = [
      {'tripId': 'trip-1', 'vehicleId': 'VEH035', 'tripNumber': 1, 'loadingStatus': 'pending'},
    ];
    await c.load();
    expect(c.trips, hasLength(1));

    final broken = controller(failing: true);
    await broken.refreshQuietly();
    expect(broken.error, isEmpty);
    expect(c.trips, hasLength(1));
  });
}
