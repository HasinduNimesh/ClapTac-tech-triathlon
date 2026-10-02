import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:waypoint_driver/screens/sign_in/sign_in_screen.dart';

import '../helpers/render.dart';

void main() {
  setUpAll(loadAppFonts);

  testWidgets('render: sign in', (tester) async {
    await pumpScreen(tester, SignInScreen(onSignIn: (_, __) {}));
    await expectLater(find.byType(MaterialApp), matchesGoldenFile('out/sign_in.png'));
  }, skip: !renderScreensEnabled);

  testWidgets('render: sign in, no signal', (tester) async {
    await pumpScreen(
      tester,
      SignInScreen(
        noSignal: true,
        onSignIn: (_, __) {},
        onContinueOffline: () {},
        savedSession: const SavedSession(
          name: 'Tharindu Perera',
          role: 'Driver',
          signedInAt: '05:12',
          runSummary: 'Sat 26 Sep (VEH014, 9 stops)',
        ),
      ),
      size: const Size(390, 959),
    );
    await tester.enterText(find.byKey(const Key('sign_in_staff_id')), 'DRV-0318');
    await tester.enterText(find.byKey(const Key('sign_in_password')), '••••••••••');
    await tester.pumpAndSettle();
    await expectLater(find.byType(MaterialApp), matchesGoldenFile('out/sign_in_no_signal.png'));
  }, skip: !renderScreensEnabled);
}
