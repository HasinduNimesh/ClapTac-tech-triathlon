import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:waypoint_driver/main.dart';
import 'package:waypoint_driver/offline/local_database.dart';
import 'package:waypoint_driver/sync/sync.dart';

import 'helpers/render.dart';

Future<InMemorySyncQueue> _boot(WidgetTester tester, {bool demo = false}) async {
  final queue = InMemorySyncQueue();
  await pumpScreen(tester, WaypointDriverApp(database: InMemoryLocalDatabase(), queue: queue, demoUpdates: demo));
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
}

void main() {
  setUpAll(loadAppFonts);

  testWidgets('boots to sign-in', (tester) async {
    await _boot(tester);
    expect(find.text('Sign in'), findsWidgets);
    expect(find.text('Drivers, Loaders and Store Manager'), findsOneWidget);
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
    expect((await queue.pending()).map((e) => e.action), contains('delivery.outcome_changed'));

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
    expect(find.textContaining('3 cartons short'), findsOneWidget);

    await tester.tap(find.text('Finish trip'));
    await tester.pumpAndSettle();
    expect(find.text('Drivers, Loaders and Store Manager'), findsOneWidget);
  });
}
