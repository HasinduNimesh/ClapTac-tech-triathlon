import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:waypoint_driver/data/driver_models.dart';
import 'package:waypoint_driver/data/sample_data.dart';
import 'package:waypoint_driver/screens/states/end_of_day_screen.dart';
import 'package:waypoint_driver/screens/states/plan_conflict_screen.dart';
import 'package:waypoint_driver/screens/states/sync_state_screen.dart';
import 'package:waypoint_driver/widgets/driver_shell.dart';

import 'helpers/render.dart';

void main() {
  setUpAll(loadAppFonts);

  final stop = sampleStops.first;

  Future<void> tapVisible(WidgetTester tester, Finder finder) async {
    await tester.ensureVisible(finder);
    await tester.pumpAndSettle();
    await tester.tap(finder);
    await tester.pumpAndSettle();
  }

  bool hasLiveRegion(WidgetTester tester) => tester
      .widgetList<Semantics>(find.byType(Semantics))
      .any((s) => s.properties.liveRegion == true);

  group('SyncStateScreen', () {
    testWidgets('saved offline shows what is stored locally and goes back to the route', (tester) async {
      var back = 0;
      await pumpScreen(tester, SyncStateScreen(kind: SyncStateKind.savedOffline, stop: stop, onPrimary: () => back++));

      expect(find.text('Offline'), findsOneWidget);
      expect(find.text('Saved on this device'), findsOneWidget);
      expect(find.text('OUT108 - Dehiwala'), findsOneWidget);
      expect(find.text('Delivered - 12 cartons'), findsOneWidget);
      expect(find.text('Saved at 08:42'), findsOneWidget);
      expect(find.text('Pending: Outcome + 1 photo'), findsOneWidget);
      expect(hasLiveRegion(tester), isTrue);

      await tapVisible(tester, find.text('Back to route'));
      expect(back, 1);
    });

    testWidgets('the planned-order note can be dismissed', (tester) async {
      await pumpScreen(tester, SyncStateScreen(kind: SyncStateKind.savedOffline, stop: stop));
      expect(find.text('Follow the planned stop order.'), findsOneWidget);

      await tapVisible(tester, find.byTooltip('Dismiss'));
      expect(find.text('Follow the planned stop order.'), findsNothing);
    });

    testWidgets('syncing reports progress as text, bar width and semantics', (tester) async {
      final handle = tester.ensureSemantics();
      var back = 0;
      await pumpScreen(
        tester,
        SyncStateScreen(kind: SyncStateKind.syncing, stop: stop, uploadProgress: 0.4, onPrimary: () => back++),
      );

      expect(find.text('Connection restored'), findsOneWidget);
      expect(find.text('Syncing'), findsOneWidget);
      expect(find.text('Uploading photo - 1 of 1'), findsOneWidget);
      expect(find.text('40%'), findsOneWidget);
      expect(find.text('1 upload remaining'), findsOneWidget);
      expect(find.text('You can return to your route while this uploads.'), findsOneWidget);
      expect(tester.widget<FractionallySizedBox>(find.byType(FractionallySizedBox)).widthFactor, 0.4);
      expect(find.bySemanticsLabel('Uploading photo, 1 of 1'), findsOneWidget);

      await tapVisible(tester, find.text('Back to route'));
      expect(back, 1);
      handle.dispose();
    });

    testWidgets('progress outside 0..1 is clamped', (tester) async {
      await pumpScreen(tester, SyncStateScreen(kind: SyncStateKind.syncing, stop: stop, uploadProgress: 1.7));
      expect(find.text('100%'), findsOneWidget);
      expect(tester.widget<FractionallySizedBox>(find.byType(FractionallySizedBox)).widthFactor, 1.0);
    });

    testWidgets('synced confirms nothing is pending and continues the route', (tester) async {
      var next = 0;
      await pumpScreen(
        tester,
        SyncStateScreen(kind: SyncStateKind.synced, stop: stop, savedAt: '08:47', onPrimary: () => next++),
      );

      expect(find.text('You’re online'), findsOneWidget);
      expect(find.text('Synced'), findsOneWidget);
      expect(find.text('Saved at 08:47'), findsOneWidget);
      expect(find.text('Photo uploaded'), findsOneWidget);
      expect(find.text('No pending updates'), findsOneWidget);
      expect(find.text('Back to route'), findsNothing);

      await tapVisible(tester, find.text('Continue route'));
      expect(next, 1);
    });

    testWidgets('upload failure offers retry and a way back', (tester) async {
      var retry = 0;
      var back = 0;
      await pumpScreen(
        tester,
        SyncStateScreen(kind: SyncStateKind.uploadFailed, stop: stop, onPrimary: () => retry++, onSecondary: () => back++),
      );

      expect(find.text('Upload needs attention'), findsOneWidget);
      expect(find.text('Photo couldn’t be uploaded'), findsOneWidget);
      expect(find.textContaining('Upload failed', findRichText: true), findsOneWidget);
      expect(find.textContaining('Connection dropped during upload.', findRichText: true), findsOneWidget);
      expect(find.textContaining('Retry when you have a stable connection.'), findsOneWidget);

      await tapVisible(tester, find.text('Retry photo upload'));
      await tapVisible(tester, find.text('Back to route'));
      expect(retry, 1);
      expect(back, 1);

      await tapVisible(tester, find.byTooltip('Dismiss'));
      expect(find.textContaining('Retry when you have a stable connection.'), findsNothing);
    });

    testWidgets('every kind announces its connectivity banner', (tester) async {
      for (final kind in SyncStateKind.values) {
        await pumpScreen(tester, SyncStateScreen(kind: kind, stop: stop));
        expect(hasLiveRegion(tester), isTrue, reason: '$kind banner should be a live region');
      }
    });

    testWidgets('bottom navigation reports the selected tab', (tester) async {
      DriverTab? picked;
      await pumpScreen(
        tester,
        SyncStateScreen(kind: SyncStateKind.synced, stop: stop, onTabSelected: (tab) => picked = tab),
      );
      await tester.tap(find.text('Updates'));
      expect(picked, DriverTab.updates);
    });
  });

  group('EndOfDayScreen', () {
    const tall = Size(390, 875);

    testWidgets('shows the Figma sample and finishes the trip', (tester) async {
      var finished = 0;
      var retried = 0;
      await pumpScreen(
        tester,
        EndOfDayScreen(summary: sampleEndOfDay, onFinishTrip: () => finished++, onRetryUpload: () => retried++),
        size: tall,
      );

      expect(find.text('Route Complete'), findsOneWidget);
      expect(find.text('Dehiwala'), findsOneWidget);
      expect(find.text('Nugegoda'), findsOneWidget);
      expect(find.text('Kirulapone'), findsOneWidget);
      expect(find.text('Partial delivery'), findsOneWidget);
      expect(find.text('2 cartons short - Dispatcher notified'), findsOneWidget);
      expect(find.text('1 upload waiting to be sent'), findsOneWidget);
      expect(find.text('Saved on this phone. One upload has not been sent yet.'), findsOneWidget);

      await tapVisible(tester, find.text('Retry Upload'));
      await tapVisible(tester, find.text('Finish trip'));
      expect(retried, 1);
      expect(finished, 1);
    });

    testWidgets('tile counts follow the completed and unresolved lists', (tester) async {
      const summary = EndOfDaySummary(
        totalStops: 5,
        delivered: 3,
        partial: 1,
        failedOrRefused: 1,
        completed: [
          StopResult(name: 'A', window: '08:00 - 09:00', status: 'Delivered'),
          StopResult(name: 'B', window: '09:00 - 10:00', status: 'Delivered'),
          StopResult(name: 'C', window: '10:00 - 11:00', status: 'Delivered'),
        ],
        unresolved: [
          StopResult(name: 'D', window: '11:00 - 12:00', status: 'Partial delivery'),
          StopResult(name: 'E', window: '12:00 - 13:00', status: 'Refused'),
        ],
      );
      final handle = tester.ensureSemantics();
      await pumpScreen(tester, EndOfDayScreen(summary: summary, onFinishTrip: () {}), size: tall);

      expect(find.bySemanticsLabel('Completed 3'), findsOneWidget);
      expect(find.bySemanticsLabel('Unresolved 2'), findsOneWidget);
      // The stops box shows the supplied totals, independent of the tile counts.
      expect(find.text('5'), findsOneWidget);
      expect(find.text('Stops:'), findsOneWidget);
      expect(find.text('Failed / Refused:'), findsOneWidget);
      handle.dispose();
    });

    testWidgets('hides the pending uploads section when everything is uploaded', (tester) async {
      const summary = EndOfDaySummary(
        totalStops: 1,
        delivered: 1,
        partial: 0,
        failedOrRefused: 0,
        completed: [StopResult(name: 'A', window: '08:00 - 09:00', status: 'Delivered')],
        unresolved: [],
      );
      await pumpScreen(tester, EndOfDayScreen(summary: summary, onFinishTrip: () {}), size: tall);

      expect(find.text('Pending uploads'), findsNothing);
      expect(find.text('Unresolved stops'), findsNothing);
      expect(find.text('Retry Upload'), findsNothing);
      expect(find.text('Finish trip'), findsOneWidget);
    });

    testWidgets('pluralises several pending uploads', (tester) async {
      const summary = EndOfDaySummary(
        totalStops: 1,
        delivered: 1,
        partial: 0,
        failedOrRefused: 0,
        completed: [StopResult(name: 'A', window: '08:00 - 09:00', status: 'Delivered')],
        unresolved: [],
        pendingUploads: 3,
      );
      await pumpScreen(tester, EndOfDayScreen(summary: summary, onFinishTrip: () {}), size: tall);

      expect(find.text('3 uploads waiting to be sent'), findsOneWidget);
      expect(find.text('Saved on this phone. 3 uploads have not been sent yet.'), findsOneWidget);
    });
  });

  group('PlanConflictScreen', () {
    testWidgets('shows both plan versions and routes the two actions', (tester) async {
      var sent = 0;
      var returned = 0;
      await pumpScreen(
        tester,
        PlanConflictScreen(info: samplePlanConflict, onSendForReview: () => sent++, onReturnToRoute: () => returned++),
      );

      expect(find.text('Plan update needs review'), findsOneWidget);
      expect(find.text('Offline record is safe'), findsOneWidget);
      expect(find.text('Saved on this phone'), findsOneWidget);
      expect(find.text('PLAN V3'), findsOneWidget);
      expect(find.text('Stop 3 · 96 of 120 received · 06:32'), findsOneWidget);
      expect(find.text('New dispatcher plan'), findsOneWidget);
      expect(find.text('PLAN V4'), findsOneWidget);
      expect(find.text('What happens next'), findsOneWidget);

      await tapVisible(tester, find.text('Send for dispatcher review'));
      await tapVisible(tester, find.text('Return to route'));
      expect(sent, 1);
      expect(returned, 1);
    });

    testWidgets('renders custom plan details', (tester) async {
      const info = PlanConflictInfo(savedPlan: 'PLAN V7', savedDetail: 'Stop 1 · 10 of 10', newPlan: 'PLAN V8', newDetail: 'Trip reassigned');
      await pumpScreen(tester, PlanConflictScreen(info: info, onSendForReview: () {}, onReturnToRoute: () {}));

      expect(find.text('PLAN V7'), findsOneWidget);
      expect(find.text('Stop 1 · 10 of 10'), findsOneWidget);
      expect(find.text('PLAN V8'), findsOneWidget);
      expect(find.text('Trip reassigned'), findsOneWidget);
    });
  });
}
