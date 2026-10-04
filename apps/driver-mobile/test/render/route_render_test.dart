import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:waypoint_driver/data/driver_models.dart';
import 'package:waypoint_driver/data/sample_data.dart';
import 'package:waypoint_driver/screens/route/report_problem_sheet.dart';
import 'package:waypoint_driver/screens/route/route_home_screen.dart';
import 'package:waypoint_driver/screens/route/safe_stop_screen.dart';
import 'package:waypoint_driver/screens/route/truck_checkout_sheet.dart';

import '../helpers/render.dart';

void _noop() {}

// The safe-stop frame uses its own stop (OUT047, 05:30–07:30).
const _safeStop = StopInfo(
  sequence: 3,
  outletCode: 'OUT047',
  name: 'Kirulapone',
  windowStart: '05:30',
  windowEnd: '07:30',
  units: 120,
  accessNote: 'Rear entrance · van-only access',
  contactNote: 'Call the store on arrival · shared loading bay',
  goods: 'Chilled dairy',
);

void main() {
  setUpAll(loadAppFonts);

  testWidgets('render: route home', (tester) async {
    await pumpScreen(
        tester,
        const RouteHomeScreen(
            trip: sampleTrip, onViewStop: _noop, onReportProblem: _noop));
    await expectLater(
        find.byType(MaterialApp), matchesGoldenFile('out/route_home.png'));
  }, skip: !renderScreensEnabled);

  testWidgets('render: route home with one stop completed', (tester) async {
    const trip = TripInfo(
      vehicleCode: 'VEH017',
      plate: 'WP LB-4521',
      tripRef: 'TRP02801',
      depot: 'Peliyagoda Depot',
      window: '08:00-11:00',
      stops: sampleStops,
      completedStops: 2,
    );
    await pumpScreen(
        tester,
        const RouteHomeScreen(
            trip: trip, onViewStop: _noop, onReportProblem: _noop));
    await expectLater(find.byType(MaterialApp),
        matchesGoldenFile('out/route_home_progress.png'));
  }, skip: !renderScreensEnabled);

  testWidgets('render: safe stop', (tester) async {
    await pumpScreen(
      tester,
      const SafeStopScreen(
        trip: sampleTrip,
        stop: _safeStop,
        onStoppedSafely: _noop,
        onOpenMaps: _noop,
        earliestAccess: '05:45',
        planVersion: 'v4',
      ),
    );
    await expectLater(
        find.byType(MaterialApp), matchesGoldenFile('out/safe_stop.png'));
  }, skip: !renderScreensEnabled);

  testWidgets('render: truck check-out sheet', (tester) async {
    await pumpScreen(
        tester,
        const RouteHomeScreen(
            trip: sampleTrip, onViewStop: _noop, onReportProblem: _noop));
    showTruckCheckoutSheet(
      tester.element(find.byType(RouteHomeScreen)),
      trip: sampleTrip,
      onConfirm: _noop,
      onMissingItem: _noop,
      planVersion: 'v4',
      sealNumber: 'WP-5521',
      lines: const [
        LoadCheckLine(title: 'Stop 1 · Dehiwala · 3 lines on board'),
        LoadCheckLine(
            title: 'Stop 2 · Nugegoda · 2 lines on board',
            detail: 'Moved up from Stop 4 in plan v3'),
        LoadCheckLine(
          title: 'Stop 3 · Kirulapone · FR-4821',
          detail:
              'Yoghurt cup 80g: 96 of 120 on board. 24 short, approved by dispatcher at 03:05',
          warning: true,
        ),
        LoadCheckLine(title: 'Chilled zone 2 to 4 °C, seal number matches'),
      ],
    );
    await tester.pumpAndSettle();
    await expectLater(
        find.byType(MaterialApp), matchesGoldenFile('out/truck_checkout.png'));
  }, skip: !renderScreensEnabled);

  testWidgets('render: report a problem sheet', (tester) async {
    await pumpScreen(
        tester,
        const RouteHomeScreen(
            trip: sampleTrip, onViewStop: _noop, onReportProblem: _noop));
    showReportProblemSheet(
      tester.element(find.byType(RouteHomeScreen)),
      trip: sampleTrip,
      stop: _safeStop,
      onSend: (_) {},
    );
    await tester.pumpAndSettle();
    await expectLater(
        find.byType(MaterialApp), matchesGoldenFile('out/report_problem.png'));
  }, skip: !renderScreensEnabled);
}
