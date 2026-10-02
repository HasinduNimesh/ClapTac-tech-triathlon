import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:waypoint_driver/data/driver_models.dart';
import 'package:waypoint_driver/data/sample_data.dart';
import 'package:waypoint_driver/screens/delivery/record_delivery_screen.dart';
import 'package:waypoint_driver/screens/delivery/stop_details_screen.dart';
import 'package:waypoint_driver/screens/delivery/take_back_sheet.dart';
import 'package:waypoint_driver/widgets/app_buttons.dart';

import 'package:waypoint_driver/screens/delivery/delivery_assets.dart';

import 'helpers/render.dart';

Future<void> pumpDelivery(WidgetTester tester, Widget screen, {Size size = const Size(390, 844)}) async {
  await pumpScreen(tester, screen, size: size);
  await tester.runAsync(() => precacheImage(const AssetImage(DeliveryAssets.photoThumb), tester.element(find.byType(MaterialApp))));
}

void main() {
  setUpAll(loadAppFonts);

  final stop = sampleStops.first;

  group('StopDetailsScreen', () {
    testWidgets('shows the stop summary and defaults to Delivered', (tester) async {
      await pumpScreen(tester, StopDetailsScreen(stop: stop, onSave: (_) {}));
      expect(find.text('OUT108 - Dehiwala'), findsOneWidget);
      expect(find.text('08:00 - 09:00'), findsOneWidget);
      expect(find.text('12 cartons'), findsOneWidget);
      expect(find.text('Rear entrance - Van-only access'), findsOneWidget);
      expect(find.text('Expected: 12'), findsOneWidget);
      expect(find.text('Save delivery'), findsOneWidget);
      expect(find.text('Complete this form only while parked.'), findsOneWidget);
      expect(find.text('No connection'), findsNothing);
    });

    testWidgets('saves the chosen outcome, notes and proof', (tester) async {
      DeliveryDraft? saved;
      await pumpScreen(tester, StopDetailsScreen(stop: stop, onSave: (d) => saved = d));

      await tester.tap(find.text('Refused'));
      await tester.tap(find.text('Take photo'));
      await tester.tap(find.text('Add signature'));
      await tester.pump();
      expect(find.text('Photo saved'), findsOneWidget);
      expect(find.text('Signature added'), findsOneWidget);

      await tester.enterText(find.byType(TextField), '  Store closed  ');
      await tester.tap(find.text('Save delivery'));

      expect(saved, isNotNull);
      expect(saved!.outcome, DeliveryOutcome.refused);
      expect(saved!.notes, 'Store closed');
      expect(saved!.hasPhoto, isTrue);
      expect(saved!.hasSignature, isTrue);
      expect(saved!.quantity, isNull);
    });

    testWidgets('a delivered outcome carries the expected cartons', (tester) async {
      DeliveryDraft? saved;
      await pumpScreen(tester, StopDetailsScreen(stop: stop, onSave: (d) => saved = d));
      await tester.tap(find.text('Save delivery'));
      expect(saved!.outcome, DeliveryOutcome.delivered);
      expect(saved!.quantity, 12);
      expect(saved!.hasPhoto, isFalse);
    });

    testWidgets('only one outcome is selected at a time', (tester) async {
      await pumpScreen(tester, StopDetailsScreen(stop: stop, onSave: (_) {}));
      final handle = tester.ensureSemantics();
      Matcher checked(bool value) => isSemantics(isChecked: value, hasCheckedState: true, isInMutuallyExclusiveGroup: true, isButton: true);
      expect(tester.getSemantics(find.bySemanticsLabel('Delivered')), checked(true));
      await tester.tap(find.text('Partial'));
      await tester.pump();
      expect(tester.getSemantics(find.bySemanticsLabel('Delivered')), checked(false));
      expect(tester.getSemantics(find.bySemanticsLabel('Partial')), checked(true));
      handle.dispose();
    });

    testWidgets('report an issue row calls back', (tester) async {
      var reported = 0;
      await pumpScreen(tester, StopDetailsScreen(stop: stop, onSave: (_) {}, onReportIssue: () => reported++));
      await tester.ensureVisible(find.text('Report an issue').last);
      await tester.tap(find.text('Report an issue').last);
      expect(reported, 1);
    });

    testWidgets('the parked note can be dismissed', (tester) async {
      await pumpScreen(tester, StopDetailsScreen(stop: stop, onSave: (_) {}));
      final dismiss = find.byTooltip('Dismiss');
      await tester.ensureVisible(dismiss);
      await tester.tap(dismiss);
      await tester.pump();
      expect(find.text('Complete this form only while parked.'), findsNothing);
    });

    testWidgets('offline variant shows the connection banner and local save', (tester) async {
      DeliveryDraft? saved;
      await pumpDelivery(tester, StopDetailsScreen(stop: stop, offline: true, onSave: (d) => saved = d));
      expect(find.text('No connection'), findsOneWidget);
      expect(find.text('You can still record this delivery'), findsOneWidget);
      expect(find.text('Save on this device'), findsOneWidget);
      expect(find.text('Photo proof'), findsOneWidget);
      expect(find.text('Updates will be uploaded when you’re online.'), findsOneWidget);
      expect(find.text('Use only while parked.'), findsOneWidget);

      await tester.tap(find.text('Take photo'));
      await tester.pump();
      expect(find.text('Photo saved locally'), findsOneWidget);

      await tester.tap(find.text('Save on this device'));
      expect(saved!.hasPhoto, isTrue);
    });
  });

  group('RecordDeliveryScreen', () {
    Widget screen({ValueChanged<DeliveryDraft>? onSave, VoidCallback? onRejected}) => RecordDeliveryScreen(
          stop: sampleStops.last,
          orderRef: 'FR-4821',
          expectedQuantity: 120,
          onSave: onSave ?? (_) {},
          onRejected: onRejected,
        );

    bool saveEnabled(WidgetTester tester) => tester.widget<AppButton>(find.widgetWithText(AppButton, 'Save delivery offline')).onPressed != null;

    testWidgets('renders the Figma copy', (tester) async {
      await pumpScreen(tester, screen());
      expect(find.text('Record delivery'), findsOneWidget);
      expect(find.text('OUT047 · FR-4821 · 120 of 120 received'), findsOneWidget);
      expect(find.text('SAVED ON THIS PHONE'), findsOneWidget);
      for (final text in ['Full quantity received', 'Record exact quantity', 'Store declined goods', 'Store closed / no access']) {
        expect(find.text(text), findsOneWidget);
      }
      expect(find.text('Saved locally first · uploads when connected'), findsOneWidget);
      expect(find.text('Only complete this form when the vehicle is stopped.'), findsOneWidget);
    });

    testWidgets('partial delivery records the typed quantity', (tester) async {
      DeliveryDraft? saved;
      await pumpScreen(tester, screen(onSave: (d) => saved = d));
      await tester.tap(find.text('Partial'));
      await tester.pump();
      await tester.enterText(find.byType(TextField), '96');
      await tester.pump();
      expect(find.text('OUT047 · FR-4821 · 96 of 120 received'), findsOneWidget);

      await tester.tap(find.text('Add photo'));
      await tester.tap(find.text('Save delivery offline'));
      expect(saved!.outcome, DeliveryOutcome.partial);
      expect(saved!.quantity, 96);
      expect(saved!.hasPhoto, isTrue);
      expect(saved!.hasSignature, isFalse);
    });

    testWidgets('quantity is clamped to the expected amount', (tester) async {
      await pumpScreen(tester, screen());
      await tester.tap(find.text('Partial'));
      await tester.pump();
      await tester.enterText(find.byType(TextField), '999');
      await tester.pump();
      expect(tester.widget<TextField>(find.byType(TextField)).controller!.text, '120');
      expect(saveEnabled(tester), isTrue);
    });

    testWidgets('an empty quantity blocks saving', (tester) async {
      await pumpScreen(tester, screen());
      await tester.tap(find.text('Partial'));
      await tester.pump();
      await tester.enterText(find.byType(TextField), '');
      await tester.pump();
      expect(find.text('Enter a quantity from 0 to 120'), findsOneWidget);
      expect(saveEnabled(tester), isFalse);
    });

    testWidgets('the quantity is fixed unless the outcome is partial', (tester) async {
      await pumpScreen(tester, screen());
      expect(tester.widget<TextField>(find.byType(TextField)).enabled, isFalse);
      await tester.tap(find.text('Partial'));
      await tester.pump();
      expect(tester.widget<TextField>(find.byType(TextField)).enabled, isTrue);
    });

    testWidgets('rejected notifies the caller and records nothing received', (tester) async {
      var rejected = 0;
      DeliveryDraft? saved;
      await pumpScreen(tester, screen(onRejected: () => rejected++, onSave: (d) => saved = d));
      await tester.tap(find.text('Rejected'));
      await tester.pump();
      expect(rejected, 1);
      await tester.tap(find.text('Save delivery offline'));
      expect(saved!.outcome, DeliveryOutcome.refused);
      expect(saved!.quantity, 0);
    });

    testWidgets('unavailable maps to a failed delivery with zero received', (tester) async {
      DeliveryDraft? saved;
      await pumpScreen(tester, screen(onSave: (d) => saved = d));
      await tester.tap(find.text('Unavailable'));
      await tester.pump();
      await tester.tap(find.text('Save delivery offline'));
      expect(saved!.outcome, DeliveryOutcome.failed);
      expect(saved!.quantity, 0);
    });
  });

  group('showTakeBackSheet', () {
    Future<void> open(WidgetTester tester, ValueChanged<bool> onSave) async {
      await pumpScreen(
        tester,
        Builder(
          builder: (context) => Scaffold(
            body: Center(
              child: TextButton(
                onPressed: () => showTakeBackSheet(context, stop: sampleStops.last, orderRef: 'FR-4821', onSave: onSave),
                child: const Text('open sheet'),
              ),
            ),
          ),
        ),
      );
      await tester.tap(find.text('open sheet'));
      await tester.pumpAndSettle();
    }

    testWidgets('lists what goes back and re-attempts by default', (tester) async {
      bool? choice;
      await open(tester, (value) => choice = value);
      expect(find.text('Rejected: record what goes back'), findsOneWidget);
      expect(find.text('Stop 3 · Kirulapone · FR-4821'), findsOneWidget);
      expect(find.text('Yoghurt cup 80g · taking back 96'), findsOneWidget);
      expect(find.text('Fresh milk 1L · taking back 24'), findsOneWidget);

      await tester.tap(find.text('Save take-back offline'));
      await tester.pumpAndSettle();
      expect(choice, isTrue);
      expect(find.text('Rejected: record what goes back'), findsNothing);
    });

    testWidgets('asking dispatch to defer reports false', (tester) async {
      bool? choice;
      await open(tester, (value) => choice = value);
      await tester.tap(find.text('Ask dispatcher to defer'));
      await tester.pump();
      await tester.tap(find.text('Save take-back offline'));
      await tester.pumpAndSettle();
      expect(choice, isFalse);
    });

    testWidgets('Back closes the sheet without saving', (tester) async {
      bool? choice;
      await open(tester, (value) => choice = value);
      await tester.tap(find.text('Back'));
      await tester.pumpAndSettle();
      expect(choice, isNull);
      expect(find.text('Rejected: record what goes back'), findsNothing);
    });
  });
}
