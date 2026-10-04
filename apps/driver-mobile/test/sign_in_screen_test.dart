import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:waypoint_driver/screens/sign_in/sign_in_screen.dart';

import 'helpers/render.dart';

const _session = SavedSession(
  name: 'Tharindu Perera',
  role: 'Driver',
  signedInAt: '05:12',
  runSummary: 'Sat 26 Sep (VEH014, 9 stops)',
);

Finder _button(String label) => find.ancestor(of: find.text(label), matching: find.byType(InkWell));

void main() {
  setUpAll(loadAppFonts);

  testWidgets('sign in stays disabled until both fields are filled, then submits trimmed values', (tester) async {
    String? id;
    String? pw;
    await pumpScreen(tester, SignInScreen(onSignIn: (a, b) {
      id = a;
      pw = b;
    }));

    await tester.tap(_button('Sign in').last);
    expect(id, isNull);

    await tester.enterText(find.byKey(const Key('sign_in_staff_id')), '  DRV-0318 ');
    await tester.pump();
    await tester.tap(_button('Sign in').last);
    expect(id, isNull, reason: 'password still empty');

    await tester.enterText(find.byKey(const Key('sign_in_password')), 'secret');
    await tester.pump();
    await tester.tap(_button('Sign in').last);
    expect(id, 'DRV-0318');
    expect(pw, 'secret');
  });

  testWidgets('submitting from the keyboard works once the form is valid', (tester) async {
    var calls = 0;
    await pumpScreen(tester, SignInScreen(onSignIn: (_, __) => calls++));
    await tester.enterText(find.byKey(const Key('sign_in_staff_id')), 'DRV-0318');
    await tester.enterText(find.byKey(const Key('sign_in_password')), 'secret');
    await tester.testTextInput.receiveAction(TextInputAction.done);
    await tester.pump();
    expect(calls, 1);
  });

  testWidgets('the eye button shows and hides the password', (tester) async {
    await pumpScreen(tester, SignInScreen(onSignIn: (_, __) {}));
    TextField field() => tester.widget<TextField>(find.byKey(const Key('sign_in_password')));
    expect(field().obscureText, isTrue);
    await tester.tap(find.byKey(const Key('sign_in_toggle_password')));
    await tester.pump();
    expect(field().obscureText, isFalse);
    await tester.tap(find.byKey(const Key('sign_in_toggle_password')));
    await tester.pump();
    expect(field().obscureText, isTrue);
  });

  testWidgets('keep me signed in toggles and forgot password calls back', (tester) async {
    var forgot = 0;
    await pumpScreen(tester, SignInScreen(onSignIn: (_, __) {}, onForgotPassword: () => forgot++));
    expect(find.byKey(const Key('sign_in_checkmark')), findsOneWidget);
    await tester.tap(find.byKey(const Key('sign_in_keep_signed_in')));
    await tester.pump();
    expect(find.byKey(const Key('sign_in_checkmark')), findsNothing);
    await tester.tap(find.byKey(const Key('sign_in_forgot_password')));
    expect(forgot, 1);
  });

  testWidgets('no signal: shows the notice, a disabled button and the saved session', (tester) async {
    var offline = 0;
    var signIns = 0;
    await pumpScreen(
      tester,
      SignInScreen(noSignal: true, savedSession: _session, onSignIn: (_, __) => signIns++, onContinueOffline: () => offline++),
    );

    expect(find.text('No signal, so we can’t check your password'), findsOneWidget);
    expect(find.text('Waiting for signal…'), findsOneWidget);
    expect(find.text('Tharindu Perera'), findsOneWidget);
    expect(find.text('Driver · signed in on this phone today at 05:12'), findsOneWidget);
    expect(find.textContaining('Your run for Sat 26 Sep (VEH014, 9 stops) is saved here.'), findsOneWidget);
    expect(find.text('Keep me signed in'), findsNothing);
    expect(find.byKey(const Key('sign_in_toggle_password')), findsNothing);

    await tester.enterText(find.byKey(const Key('sign_in_staff_id')), 'DRV-0318');
    await tester.enterText(find.byKey(const Key('sign_in_password')), 'secret');
    await tester.pump();
    await tester.tap(_button('Waiting for signal…'));
    expect(signIns, 0, reason: 'cannot sign in without signal');

    await tester.tap(find.text('Continue offline'));
    expect(offline, 1);
  });

  testWidgets('no signal without a saved session hides the offline card', (tester) async {
    await pumpScreen(tester, SignInScreen(noSignal: true, onSignIn: (_, __) {}));
    expect(find.text('Continue offline'), findsNothing);
    expect(find.text('No signal, so we can’t check your password'), findsOneWidget);
  });

  testWidgets('the form scrolls when the keyboard leaves little room', (tester) async {
    await pumpScreen(tester, SignInScreen(onSignIn: (_, __) {}), size: const Size(390, 400));
    expect(tester.takeException(), isNull);
    await tester.drag(find.byType(SingleChildScrollView), const Offset(0, -200));
    await tester.pump();
    expect(tester.takeException(), isNull);
  });
}
