import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:waypoint_driver/data/driver_models.dart';
import 'package:waypoint_driver/data/sample_data.dart';
import 'package:waypoint_driver/screens/states/end_of_day_screen.dart';
import 'package:waypoint_driver/screens/states/plan_conflict_screen.dart';
import 'package:waypoint_driver/screens/states/sync_state_screen.dart';

import '../helpers/render.dart';

void main() {
  setUpAll(loadAppFonts);

  final stop = sampleStops.first;

  Future<void> render(WidgetTester tester, Widget screen, String name, {Size size = const Size(390, 844)}) async {
    await pumpScreen(tester, screen, size: size);
    await expectLater(find.byType(MaterialApp), matchesGoldenFile('out/$name.png'));
  }

  testWidgets('render: saved offline', (tester) async {
    await render(tester, SyncStateScreen(kind: SyncStateKind.savedOffline, stop: stop), 'sync_saved');
  }, skip: !renderScreensEnabled);

  testWidgets('render: syncing', (tester) async {
    await render(tester, SyncStateScreen(kind: SyncStateKind.syncing, stop: stop), 'sync_syncing');
  }, skip: !renderScreensEnabled);

  testWidgets('render: synced', (tester) async {
    await render(tester, SyncStateScreen(kind: SyncStateKind.synced, stop: stop, savedAt: '08:47'), 'sync_synced');
  }, skip: !renderScreensEnabled);

  testWidgets('render: upload failed', (tester) async {
    await render(tester, SyncStateScreen(kind: SyncStateKind.uploadFailed, stop: stop), 'sync_error');
  }, skip: !renderScreensEnabled);

  testWidgets('render: end of day', (tester) async {
    await render(tester, EndOfDayScreen(summary: sampleEndOfDay, onFinishTrip: () {}), 'end_of_day', size: const Size(390, 875));
  }, skip: !renderScreensEnabled);

  testWidgets('render: plan conflict', (tester) async {
    await render(
      tester,
      PlanConflictScreen(info: samplePlanConflict, onSendForReview: () {}, onReturnToRoute: () {}),
      'plan_conflict',
    );
  }, skip: !renderScreensEnabled);
}
