import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:waypoint_driver/data/sample_data.dart';
import 'package:waypoint_driver/screens/delivery/record_delivery_screen.dart';
import 'package:waypoint_driver/screens/delivery/stop_details_screen.dart';
import 'package:waypoint_driver/screens/delivery/take_back_sheet.dart';

import 'package:waypoint_driver/screens/delivery/delivery_assets.dart';

import '../helpers/render.dart';

Future<void> pumpDelivery(WidgetTester tester, Widget screen, {Size size = const Size(390, 844)}) async {
  await pumpScreen(tester, screen, size: size);
  await tester.runAsync(() => precacheImage(const AssetImage(DeliveryAssets.photoThumb), tester.element(find.byType(MaterialApp))));
}

void main() {
  setUpAll(loadAppFonts);
  final skip = !renderScreensEnabled;

  testWidgets('render: stop details', (tester) async {
    await pumpScreen(tester, StopDetailsScreen(stop: sampleStops.first, onSave: (_) {}), size: const Size(390, 1000));
    await expectLater(find.byType(MaterialApp), matchesGoldenFile('out/stop_details.png'));
  }, skip: skip);

  testWidgets('render: stop details offline', (tester) async {
    await pumpDelivery(tester, StopDetailsScreen(stop: sampleStops.first, offline: true, onSave: (_) {}), size: const Size(390, 1000));
    await tester.tap(find.text('Take photo'));
    await tester.pumpAndSettle();
    await expectLater(find.byType(MaterialApp), matchesGoldenFile('out/stop_details_offline.png'));
  }, skip: skip);

  testWidgets('render: record delivery', (tester) async {
    await pumpScreen(
      tester,
      RecordDeliveryScreen(stop: sampleStops.last, orderRef: 'FR-4821', expectedQuantity: 120, onSave: (_) {}),
    );
    await tester.tap(find.text('Partial'));
    await tester.pump();
    await tester.enterText(find.byType(TextField), '96');
    await tester.pumpAndSettle();
    await expectLater(find.byType(MaterialApp), matchesGoldenFile('out/record_delivery.png'));
  }, skip: skip);

  testWidgets('render: take back sheet', (tester) async {
    await pumpScreen(
      tester,
      Builder(
        builder: (context) => RecordDeliveryScreen(
          stop: sampleStops.last,
          orderRef: 'FR-4821',
          expectedQuantity: 120,
          onSave: (_) {},
          onRejected: () => showTakeBackSheet(context, stop: sampleStops.last, orderRef: 'FR-4821', reason: takeBackReasonText('OUTLET_CLOSED'), onSave: (_) {}),
        ),
      ),
    );
    await tester.tap(find.text('Rejected'));
    await tester.pumpAndSettle();
    await expectLater(find.byType(MaterialApp), matchesGoldenFile('out/take_back.png'));
  }, skip: skip);
}
