import 'package:flutter/material.dart';
import 'package:flutter/semantics.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:waypoint_driver/data/driver_models.dart';
import 'package:waypoint_driver/data/sample_data.dart';
import 'package:waypoint_driver/screens/route/report_problem_sheet.dart';
import 'package:waypoint_driver/screens/route/route_home_screen.dart';
import 'package:waypoint_driver/screens/route/safe_stop_screen.dart';
import 'package:waypoint_driver/screens/route/truck_checkout_sheet.dart';
import 'package:waypoint_driver/widgets/driver_shell.dart';

import 'helpers/render.dart';

void _noop() {}

TripInfo _tripWithCompleted(int completed) => TripInfo(
      vehicleCode: sampleTrip.vehicleCode,
      plate: sampleTrip.plate,
      tripRef: sampleTrip.tripRef,
      depot: sampleTrip.depot,
      window: sampleTrip.window,
      stops: sampleTrip.stops,
      completedStops: completed,
    );

class _SheetHost extends StatelessWidget {
  const _SheetHost({required this.open});

  final void Function(BuildContext context) open;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: Center(
        child: ElevatedButton(
            onPressed: () => open(context), child: const Text('open sheet')),
      ),
    );
  }
}

void main() {
  setUpAll(loadAppFonts);

  group('RouteHomeScreen', () {
    testWidgets('shows progress, the next stop and the whole route',
        (tester) async {
      await pumpScreen(
          tester,
          const RouteHomeScreen(
              trip: sampleTrip, onViewStop: _noop, onReportProblem: _noop));

      expect(find.text('0 of 3 stops completed'), findsOneWidget);
      expect(find.text('VEH017 - WP LB-4521'), findsOneWidget);
      expect(find.text('OUT108 - Dehiwala'), findsOneWidget);
      expect(find.text('12 cartons'), findsOneWidget);
      // The next stop's window appears in the card and in the route list.
      expect(find.text('08:00 - 09:00'), findsNWidgets(2));
      for (final name in ['Dehiwala', 'Nugegoda', 'Kirulapone']) {
        expect(find.text(name), findsOneWidget);
      }
    });

    testWidgets('View Stop and Report a problem call back', (tester) async {
      var viewed = 0;
      var reported = 0;
      await pumpScreen(
        tester,
        RouteHomeScreen(
            trip: sampleTrip,
            onViewStop: () => viewed++,
            onReportProblem: () => reported++),
      );

      await tester.tap(find.text('View Stop'));
      await tester.tap(find.text('⚠ Report a problem'));

      expect(viewed, 1);
      expect(reported, 1);
    });

    testWidgets('the planned-order hint can be dismissed', (tester) async {
      await pumpScreen(
          tester,
          const RouteHomeScreen(
              trip: sampleTrip, onViewStop: _noop, onReportProblem: _noop));
      expect(find.text('Follow the planned stop order.'), findsOneWidget);

      await tester.tap(find.byTooltip('Dismiss'));
      await tester.pumpAndSettle();

      expect(find.text('Follow the planned stop order.'), findsNothing);
      // The safety note is not dismissible.
      expect(find.text('Use only when safely parked.'), findsOneWidget);
    });

    testWidgets('progress and next stop follow completedStops', (tester) async {
      await pumpScreen(
          tester,
          RouteHomeScreen(
              trip: _tripWithCompleted(1),
              onViewStop: _noop,
              onReportProblem: _noop));

      expect(find.text('1 of 3 stops completed'), findsOneWidget);
      expect(find.text('OUT061 - Nugegoda'), findsOneWidget);
      expect(find.text('8 cartons'), findsOneWidget);
    });

    testWidgets('a finished route has no View Stop action', (tester) async {
      await pumpScreen(
          tester,
          RouteHomeScreen(
              trip: _tripWithCompleted(3),
              onViewStop: _noop,
              onReportProblem: _noop));

      expect(find.text('3 of 3 stops completed'), findsOneWidget);
      expect(find.text('All stops are completed.'), findsOneWidget);
      expect(find.text('View Stop'), findsNothing);
    });

    testWidgets('bottom navigation reports the selected tab', (tester) async {
      DriverTab? selected;
      await pumpScreen(
        tester,
        RouteHomeScreen(
            trip: sampleTrip,
            onViewStop: _noop,
            onReportProblem: _noop,
            onTabSelected: (tab) => selected = tab),
      );

      await tester.tap(find.text('Summary'));
      expect(selected, DriverTab.summary);
    });

    testWidgets('exposes the route as labelled semantics', (tester) async {
      final handle = tester.ensureSemantics();
      await pumpScreen(
          tester,
          const RouteHomeScreen(
              trip: sampleTrip, onViewStop: _noop, onReportProblem: _noop));

      expect(
          find.bySemanticsLabel('Stop 1, Dehiwala, 08:00 - 09:00, next stop'),
          findsOneWidget);
      expect(
          find.bySemanticsLabel('Stop 3, Kirulapone, 10:00 - 11:00, upcoming'),
          findsOneWidget);
      expect(find.bySemanticsLabel('0 of 3 stops completed'), findsOneWidget);
      handle.dispose();
    });
  });

  group('SafeStopScreen', () {
    testWidgets('shows arrival guidance and calls back', (tester) async {
      var stopped = 0;
      var maps = 0;
      await pumpScreen(
        tester,
        SafeStopScreen(
          trip: sampleTrip,
          stop: sampleStops[2],
          onStoppedSafely: () => stopped++,
          onOpenMaps: () => maps++,
          earliestAccess: '05:45',
          planVersion: 'v4',
        ),
      );

      expect(find.text('Stop 3 · Kirulapone'), findsOneWidget);
      expect(find.text('Trip TRP02801 · Plan v4'), findsOneWidget);
      expect(find.text('12 min early'), findsOneWidget);
      expect(find.text('Earliest access 05:45 · estimated wait 12 min'),
          findsOneWidget);
      expect(find.text('Rear entrance · van-only access'), findsOneWidget);

      await tester.tap(find.text('I’ve stopped safely'));
      await tester.tap(find.text('Open in maps'));
      expect(stopped, 1);
      expect(maps, 1);
    });

    testWidgets('skips the wait guidance when the driver is on time',
        (tester) async {
      await pumpScreen(
        tester,
        SafeStopScreen(
            trip: sampleTrip,
            stop: sampleStops[0],
            onStoppedSafely: _noop,
            earlyMinutes: 0),
      );

      expect(find.text('Wait for the receiving window'), findsNothing);
      expect(find.textContaining('min early'), findsNothing);
      expect(find.text('Receiving window is open'), findsOneWidget);
      expect(find.text('I’ve stopped safely'), findsOneWidget);
    });
  });

  group('showTruckCheckoutSheet', () {
    Future<void> openSheet(
      WidgetTester tester, {
      VoidCallback? onConfirm,
      VoidCallback? onMissing,
      List<LoadCheckLine>? lines,
      Size size = const Size(390, 844),
    }) async {
      await pumpScreen(
        tester,
        _SheetHost(
          open: (context) => showTruckCheckoutSheet(
            context,
            trip: sampleTrip,
            onConfirm: onConfirm ?? _noop,
            onMissingItem: onMissing ?? _noop,
            lines: lines,
            planVersion: 'v4',
            sealNumber: 'WP-5521',
          ),
        ),
        size: size,
      );
      await tester.tap(find.text('open sheet'));
      await tester.pumpAndSettle();
    }

    testWidgets('lists a check per stop from the trip', (tester) async {
      await openSheet(tester);

      expect(find.text('Check the load before you leave'), findsOneWidget);
      expect(
          find.text('VEH017 · WP LB-4521 · TRP02801 · Plan v4 · Seal WP-5521'),
          findsOneWidget);
      expect(
          find.text('Stop 1 · Dehiwala · 12 cartons on board'), findsOneWidget);
      expect(find.text('Stop 3 · Kirulapone · 120 cartons on board'),
          findsOneWidget);
      expect(find.text('✓'), findsNWidgets(3));
    });

    testWidgets('warning lines show an exclamation mark and detail',
        (tester) async {
      await openSheet(
        tester,
        lines: const [
          LoadCheckLine(
              title: 'Stop 3 · Kirulapone', detail: '24 short', warning: true),
          LoadCheckLine(title: 'Chilled zone 2 to 4 °C'),
        ],
      );

      expect(find.text('!'), findsOneWidget);
      expect(find.text('✓'), findsOneWidget);
      expect(find.text('24 short'), findsOneWidget);
    });

    testWidgets('confirming closes the sheet and calls onConfirm',
        (tester) async {
      var confirmed = 0;
      await openSheet(tester, onConfirm: () => confirmed++);

      await tester.tap(find.text('Confirm load on board'));
      await tester.pumpAndSettle();

      expect(confirmed, 1);
      expect(find.text('Check the load before you leave'), findsNothing);
    });

    testWidgets(
        'reporting a missing item closes the sheet and calls onMissingItem',
        (tester) async {
      var missing = 0;
      await openSheet(tester, onMissing: () => missing++);

      await tester.tap(find.text('Something isn\'t on my list'));
      await tester.pumpAndSettle();

      expect(missing, 1);
      expect(find.text('Check the load before you leave'), findsNothing);
    });

    testWidgets('scrolls on a small phone without overflowing', (tester) async {
      await openSheet(tester, size: const Size(320, 480));

      expect(tester.takeException(), isNull);
      expect(find.text('Confirm load on board'), findsOneWidget);
    });
  });

  group('showReportProblemSheet', () {
    Future<void> openSheet(
      WidgetTester tester, {
      required ValueChanged<ProblemReport> onSend,
      StopInfo? stop,
      Size size = const Size(390, 844),
    }) async {
      await pumpScreen(
        tester,
        _SheetHost(
            open: (context) => showReportProblemSheet(context,
                trip: sampleTrip, stop: stop, onSend: onSend)),
        size: size,
      );
      await tester.tap(find.text('open sheet'));
      await tester.pumpAndSettle();
    }

    testWidgets('names the stop and vehicle that will be attached',
        (tester) async {
      await openSheet(tester, onSend: (_) {}, stop: sampleStops[2]);

      expect(find.text('Report a problem'), findsOneWidget);
      expect(
          find.text(
              'Stop 3 · Kirulapone · VEH017. Your stop, vehicle and time are attached.'),
          findsOneWidget);
      for (final title in [
        'Vehicle breakdown',
        'Road blocked or flooded',
        'Outlet closed or no access',
        'Load wrong, missing or damaged',
        'Safety concern',
      ]) {
        expect(find.text(title), findsOneWidget);
      }
      expect(
          find.text(
              'No signal? It is saved on this phone and sent when you reconnect.'),
          findsOneWidget);
    });

    testWidgets('falls back to the vehicle when no stop is given',
        (tester) async {
      await openSheet(tester, onSend: (_) {});

      expect(find.text('VEH017. Your stop, vehicle and time are attached.'),
          findsOneWidget);
    });

    testWidgets('sends the default choice when nothing else is picked',
        (tester) async {
      ProblemReport? sent;
      await openSheet(tester, onSend: (report) => sent = report);

      await tester.tap(find.text('Send to dispatcher'));
      await tester.pumpAndSettle();

      expect(sent?.kind, ProblemKind.vehicleBreakdown);
      expect(sent?.note, '');
      expect(find.text('Report a problem'), findsNothing);
    });

    testWidgets('sends the chosen kind with the note', (tester) async {
      ProblemReport? sent;
      await openSheet(tester, onSend: (report) => sent = report);

      await tester.tap(find.text('Road blocked or flooded'));
      await tester.pumpAndSettle();
      await tester.enterText(
          find.byType(TextField), '  Flooded near the bridge  ');
      await tester.tap(find.text('Send to dispatcher'));
      await tester.pumpAndSettle();

      expect(sent?.kind, ProblemKind.roadBlocked);
      expect(sent?.note, 'Flooded near the bridge');
    });

    testWidgets('only one option is selected at a time', (tester) async {
      final handle = tester.ensureSemantics();
      await openSheet(tester, onSend: (_) {});

      SemanticsNode option(String title) =>
          tester.getSemantics(find.bySemanticsLabel(RegExp('^$title')));

      expect(
          option('Vehicle breakdown'),
          isSemantics(
              hasCheckedState: true,
              isChecked: true,
              isInMutuallyExclusiveGroup: true));

      await tester.tap(find.text('Safety concern'));
      await tester.pumpAndSettle();

      expect(option('Safety concern'), isSemantics(isChecked: true));
      expect(option('Vehicle breakdown'),
          isSemantics(hasCheckedState: true, isChecked: false));
      handle.dispose();
    });

    testWidgets('Cancel closes without sending', (tester) async {
      var sent = 0;
      await openSheet(tester, onSend: (_) => sent++);

      await tester.tap(find.text('Cancel'));
      await tester.pumpAndSettle();

      expect(sent, 0);
      expect(find.text('Report a problem'), findsNothing);
    });

    testWidgets('scrolls on a small phone without overflowing', (tester) async {
      await openSheet(tester, onSend: (_) {}, size: const Size(320, 480));

      expect(tester.takeException(), isNull);
      await tester.dragUntilVisible(find.text('Send to dispatcher'),
          find.byType(SingleChildScrollView).last, const Offset(0, -80));
      expect(find.text('Send to dispatcher'), findsOneWidget);
    });
  });
}
