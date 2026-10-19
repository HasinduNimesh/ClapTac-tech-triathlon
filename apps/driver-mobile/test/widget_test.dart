import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:waypoint_driver/main.dart';
import 'package:waypoint_driver/offline/local_database.dart';
import 'package:waypoint_driver/sync/sync.dart';

import 'helpers/render.dart';

Future<InMemorySyncQueue> _boot(WidgetTester tester, {bool demo = false, bool auth = true}) async {
  final queue = InMemorySyncQueue();
  await pumpScreen(tester, WaypointDriverApp(database: InMemoryLocalDatabase(), queue: queue, demoUpdates: demo, demoAuth: auth));
  return queue;
}

Future<void> _signInAndConfirmLoad(WidgetTester tester) async {
  await tester.enterText(find.byType(TextField).first, 'DRV-0318');
  await tester.enterText(find.byType(TextField).last, 'secret');
  await tester.pump();
  await tester.tap(find.text('Sign in').last);
  await tester.pumpAndSettle();
  await tester.tap(find.text('Confirm load on board'));
  await tester.pumpAndSettle();
  await _clearSnackBars(tester);
}

/// Confirmation messages float over the bottom of the screen for a few seconds.
Future<void> _clearSnackBars(WidgetTester tester) async {
  await tester.pump(const Duration(seconds: 4));
  await tester.pumpAndSettle();
}

void main() {
  setUpAll(loadAppFonts);

  testWidgets('boots to sign-in', (tester) async {
    await _boot(tester);
    expect(find.text('Sign in'), findsWidgets);
    expect(find.text('For drivers'), findsOneWidget);
  });

  testWidgets('sign in opens the route and asks to check the load first', (tester) async {
    await _boot(tester);
    await tester.enterText(find.byType(TextField).first, 'DRV-0318');
    await tester.enterText(find.byType(TextField).last, 'secret');
    await tester.pump();
    await tester.tap(find.text('Sign in').last);
    await tester.pumpAndSettle();
    expect(find.text('Check the load before you leave'), findsOneWidget);
    await tester.tap(find.text('Confirm load on board'));
    await tester.pumpAndSettle();
    expect(find.text('0 of 3 stops completed'), findsOneWidget);
    expect(find.text('OUT108 - Dehiwala'), findsOneWidget);
  });

  testWidgets('recording a delivered stop saves it locally and advances the route', (tester) async {
    final queue = await _boot(tester);
    await _signInAndConfirmLoad(tester);

    await tester.tap(find.text('View Stop'));
    await tester.pumpAndSettle();
    expect(find.textContaining('Stop 1'), findsWidgets);
    await tester.tap(find.text('I’ve stopped safely'));
    await tester.pumpAndSettle();

    await tester.tap(find.text('Save delivery'));
    await tester.pumpAndSettle();
    expect(find.text('Saved on this device'), findsOneWidget);
    final events = await queue.pending();
    expect(events.map((e) => e.action), ['ARRIVED', 'STOP_OUTCOME']);
    expect(events.last.payload['stopId'], 'sample-stop-1');
    expect(events.last.payload['stopId'], isNot('OUT108'));
    expect((events.last.payload['payload'] as Map)['code'], 'DELIVERED');
    expect(events.last.payload.containsKey('dependsOnOperationId'), isFalse);

    await tester.tap(find.text('Back to route'));
    await tester.pumpAndSettle();
    expect(find.text('1 of 3 stops completed'), findsOneWidget);
    expect(find.text('OUT061 - Nugegoda'), findsOneWidget);
  });

  testWidgets('a refused stop asks what happens to the goods before saving', (tester) async {
    await _boot(tester);
    await _signInAndConfirmLoad(tester);
    await tester.tap(find.text('View Stop'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('I’ve stopped safely'));
    await tester.pumpAndSettle();

    await tester.tap(find.text('Refused'));
    await tester.pump();
    await tester.tap(find.text('Save delivery'));
    await tester.pumpAndSettle();
    expect(find.textContaining('Rejected: record what goes back'), findsOneWidget);
    await tester.tap(find.text('Save take-back offline'));
    await tester.pumpAndSettle();
    expect(find.text('Saved on this device'), findsOneWidget);
  });

  testWidgets('the Updates tab lists what was saved on the phone', (tester) async {
    await _boot(tester);
    await _signInAndConfirmLoad(tester);
    await tester.tap(find.text('Updates'));
    await tester.pumpAndSettle();
    expect(find.textContaining('No updates yet'), findsOneWidget);
  });

  testWidgets('the demo plan update opens the plan review screen', (tester) async {
    await _boot(tester, demo: true);
    await _signInAndConfirmLoad(tester);
    await tester.tap(find.text('Updates'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Plan update needs review').first);
    await tester.pumpAndSettle();
    expect(find.text('Send for dispatcher review'), findsOneWidget);
    await tester.tap(find.text('Return to route'));
    await tester.pumpAndSettle();
    expect(find.text('0 of 3 stops completed'), findsOneWidget);
  });

  testWidgets('summary shows progress until every stop is recorded', (tester) async {
    await _boot(tester);
    await _signInAndConfirmLoad(tester);
    await tester.tap(find.text('Summary'));
    await tester.pumpAndSettle();
    expect(find.text('Route in progress'), findsOneWidget);
  });

  testWidgets('a full day ends with the summary and returns to sign-in', (tester) async {
    await _boot(tester);
    await _signInAndConfirmLoad(tester);

    Future<void> startStop() async {
      await tester.tap(find.text('View Stop'));
      await tester.pumpAndSettle();
      await tester.tap(find.text('I’ve stopped safely'));
      await tester.pumpAndSettle();
    }

    // Stop 1: delivered.
    await startStop();
    await tester.tap(find.text('Save delivery'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Back to route'));
    await tester.pumpAndSettle();

    // Stop 2: partial with a recorded quantity.
    await startStop();
    await tester.tap(find.text('Partial'));
    await tester.pump();
    await tester.tap(find.text('Save delivery'));
    await tester.pumpAndSettle();
    await tester.enterText(find.byType(TextField).first, '5');
    await tester.pump();
    await tester.tap(find.text('Save delivery offline'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Back to route'));
    await tester.pumpAndSettle();
    expect(find.text('2 of 3 stops completed'), findsOneWidget);

    // Stop 3: refused, goods go back.
    await startStop();
    await tester.tap(find.text('Refused'));
    await tester.pump();
    await tester.tap(find.text('Save delivery'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Save take-back offline'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Back to route'));
    await tester.pumpAndSettle();

    // The last stop opens the end-of-day summary with the real results.
    expect(find.text('Route Complete'), findsOneWidget);
    expect(find.text('Kirulapone'), findsOneWidget);
    expect(find.textContaining('3 units short'), findsOneWidget);

    await tester.ensureVisible(find.text('Finish trip'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Finish trip'));
    await tester.pumpAndSettle();
    expect(find.text('For drivers'), findsOneWidget);
  });

  testWidgets('a normal build refuses to sign in and explains why', (tester) async {
    await _boot(tester, auth: false);
    await tester.enterText(find.byType(TextField).first, 'DRV-0318');
    await tester.enterText(find.byType(TextField).last, 'secret');
    await tester.pump();
    await tester.tap(find.text('Sign in').last);
    await tester.pumpAndSettle();
    expect(find.textContaining('Sign-in is not available yet'), findsOneWidget);
    expect(find.text('Check the load before you leave'), findsNothing);
    expect(find.text('0 of 3 stops completed'), findsNothing);
  });

  testWidgets('a missing load item is queued, and the route stays locked until the driver decides', (tester) async {
    final queue = await _boot(tester);
    await tester.enterText(find.byType(TextField).first, 'DRV-0318');
    await tester.enterText(find.byType(TextField).last, 'secret');
    await tester.pump();
    await tester.tap(find.text('Sign in').last);
    await tester.pumpAndSettle();
    await tester.tap(find.text("Something isn't on my list"));
    await tester.pumpAndSettle();

    // The driver must make an explicit choice; nothing is confirmed or sent.
    expect(find.text('Missing item reported'), findsOneWidget);
    expect(find.textContaining('queued on this phone only'), findsOneWidget);
    var events = await queue.pending();
    expect(events.map((e) => e.action), ['INCIDENT_REPORT']);
    expect((events.single.payload['payload'] as Map)['category'], 'GOODS');
    expect(events.single.occurredAt, isNotNull);

    // Going back reopens the load check instead of unlocking the route.
    await tester.tap(find.text('Back to load check'));
    await tester.pumpAndSettle();
    expect(find.text('Check the load before you leave'), findsOneWidget);

    // Dismissing the sheet does not unlock the route either.
    await tester.tapAt(const Offset(10, 10));
    await tester.pumpAndSettle();
    await tester.tap(find.text('View Stop'));
    await tester.pumpAndSettle();
    expect(find.text('Check the load before you leave'), findsOneWidget);
    expect(find.text('I’ve stopped safely'), findsNothing);

    // Departing anyway is an explicit decision that is recorded too.
    await tester.tap(find.text("Something isn't on my list"));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Depart anyway'));
    await tester.pumpAndSettle();
    expect(find.textContaining('Departure recorded on this phone'), findsOneWidget);
    events = await queue.pending();
    expect(events.map((e) => e.idempotencyKey).toSet(), hasLength(2));
    expect(events.every((e) => e.action == 'INCIDENT_REPORT' && (e.payload['payload'] as Map)['category'] == 'GOODS'), isTrue);

    await tester.pump(const Duration(seconds: 6));
    await tester.pumpAndSettle();
    await tester.tap(find.text('View Stop'));
    await tester.pumpAndSettle();
    expect(find.text('I’ve stopped safely'), findsOneWidget);
  });

  testWidgets('confirming the load later resolves a reported discrepancy', (tester) async {
    await _boot(tester);
    await tester.enterText(find.byType(TextField).first, 'DRV-0318');
    await tester.enterText(find.byType(TextField).last, 'secret');
    await tester.pump();
    await tester.tap(find.text('Sign in').last);
    await tester.pumpAndSettle();
    await tester.tap(find.text("Something isn't on my list"));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Back to load check'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Confirm load on board'));
    await tester.pumpAndSettle();
    expect(find.textContaining('Load confirmed on this phone'), findsOneWidget);
  });

  testWidgets('a problem report is queued and the confirmation does not claim it was sent', (tester) async {
    final queue = await _boot(tester);
    await _signInAndConfirmLoad(tester);
    await tester.tap(find.textContaining('Report a problem'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Send to dispatcher'));
    await tester.pumpAndSettle();

    final events = await queue.pending();
    expect(events.where((e) => e.action == 'INCIDENT_REPORT'), hasLength(1));
    expect((events.single.payload['payload'] as Map)['category'], 'VEHICLE');
    expect(events.single.payload['stopId'], 'sample-stop-1');
    expect(events.single.occurredAt, isNotNull);
    expect(find.textContaining('has not been sent to dispatch yet'), findsOneWidget);

    await tester.pump(const Duration(seconds: 6));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Updates'));
    await tester.pumpAndSettle();
    expect(find.textContaining('saved on this phone, not sent yet'), findsOneWidget);
  });
}
