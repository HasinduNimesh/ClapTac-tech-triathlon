import 'dart:convert';

import 'package:flutter/gestures.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:waypoint_loader/api/api_client.dart';
import 'package:waypoint_loader/auth/auth.dart';
import 'package:waypoint_loader/auth/browser_stub.dart';
import 'package:waypoint_loader/auth/inactivity_guard.dart';
import 'package:waypoint_loader/auth/inactivity_monitor.dart';
import 'package:waypoint_loader/auth/oidc_config.dart';
import 'package:waypoint_loader/auth/sign_in_screen.dart';
import 'package:waypoint_loader/loader/loader_controller.dart';
import 'package:waypoint_loader/loader/switch_user.dart';
import 'package:waypoint_loader/main.dart';
import 'package:waypoint_loader/shared/models.dart';
import 'package:waypoint_loader/theme/tokens.dart';

const inactivityMessage = 'You were signed out after 5 minutes of inactivity.';
const apiBase = 'http://api.test/api/v1';
const reasonKey = 'waypoint.loader.signout.reason';
const sessionKey = 'waypoint.loader.session';
const idp = OidcConfig(issuer: 'https://id.example.test');

/// Real time does not pass in a widget test, so the tests move this clock and pump the same amount.
class TestTime {
  DateTime now = DateTime.utc(2026, 10, 5, 8);
  DateTime read() => now;
}

Future<void> elapse(WidgetTester tester, TestTime time, Duration d) async {
  time.now = time.now.add(d);
  await tester.pump(d);
}

/// Hides then restores the tab, the way a browser reports it.
Future<void> hideAndRestore(WidgetTester tester, {required void Function() whileHidden}) async {
  final binding = tester.binding;
  for (final s in [AppLifecycleState.inactive, AppLifecycleState.hidden, AppLifecycleState.paused]) {
    binding.handleAppLifecycleStateChanged(s);
  }
  whileHidden();
  for (final s in [AppLifecycleState.hidden, AppLifecycleState.inactive, AppLifecycleState.resumed]) {
    binding.handleAppLifecycleStateChanged(s);
  }
  await tester.pump();
}

String sessionJson() => jsonEncode(AuthSession(
      accessToken: 'tok',
      refreshToken: 'ref',
      profile: Profile(userId: 'USR004', roles: ['LOADER'], depot: 'DEPOT_NORTH', displayName: 'Nimal Perera'),
      expiresAt: DateTime.now().add(const Duration(hours: 2)),
    ).toJson());

MockClient fakeServer() => MockClient((req) async {
      final path = req.url.path;
      if (path.endsWith('/loading/trips') || path.endsWith('/loading/alerts')) return http.Response(jsonEncode({'items': <dynamic>[]}), 200);
      return http.Response('not found', 404); // also the identity server's discovery document: no end-session page
    });

void main() {
  group('InactivityGuard', () {
    late TestTime time;
    late int expiries;

    Future<void> pumpGuard(WidgetTester tester, {bool active = true}) async {
      await tester.pumpWidget(MaterialApp(
        home: InactivityGuard(active: active, now: time.read, onExpired: () => expiries++, child: const Scaffold(body: SizedBox.expand(child: Center(child: Text('workspace'))))),
      ));
    }

    setUp(() {
      time = TestTime();
      expiries = 0;
    });

    testWidgets('signs the loader out after five minutes of nobody touching the tablet', (tester) async {
      await pumpGuard(tester);
      await elapse(tester, time, const Duration(minutes: 4, seconds: 59));
      expect(expiries, 0);
      await elapse(tester, time, const Duration(seconds: 1));
      expect(expiries, 1);
      await elapse(tester, time, const Duration(minutes: 20));
      expect(expiries, 1, reason: 'once only');
    });

    testWidgets('touching the screen starts the five minutes again', (tester) async {
      await pumpGuard(tester);
      await elapse(tester, time, const Duration(minutes: 4));
      await tester.tapAt(const Offset(20, 20));
      await elapse(tester, time, const Duration(minutes: 4));
      expect(expiries, 0);
      await elapse(tester, time, const Duration(minutes: 1));
      expect(expiries, 1);
    });

    testWidgets('moving a mouse or scrolling counts as activity too', (tester) async {
      await pumpGuard(tester);
      final mouse = await tester.createGesture(kind: PointerDeviceKind.mouse);
      await mouse.addPointer(location: const Offset(10, 10));
      await elapse(tester, time, const Duration(minutes: 4));
      await mouse.moveTo(const Offset(50, 50));
      await elapse(tester, time, const Duration(minutes: 4));
      expect(expiries, 0, reason: 'moving the mouse moved the deadline');
      await tester.sendEventToBinding(const PointerScrollEvent(position: Offset(50, 50), scrollDelta: Offset(0, 40)));
      await elapse(tester, time, const Duration(minutes: 4));
      expect(expiries, 0, reason: 'a wheel scroll moved it again');
      await elapse(tester, time, const Duration(minutes: 1));
      expect(expiries, 1);
    });

    testWidgets('typing counts as activity', (tester) async {
      await pumpGuard(tester);
      await elapse(tester, time, const Duration(minutes: 4));
      await tester.sendKeyEvent(LogicalKeyboardKey.keyA);
      await elapse(tester, time, const Duration(minutes: 4));
      expect(expiries, 0);
      await elapse(tester, time, const Duration(minutes: 1));
      expect(expiries, 1);
    });

    testWidgets('does nothing while nobody is signed in, and counts from the sign-in', (tester) async {
      await pumpGuard(tester, active: false);
      await elapse(tester, time, const Duration(minutes: 30));
      expect(expiries, 0);
      await pumpGuard(tester, active: true);
      await elapse(tester, time, const Duration(minutes: 4));
      expect(expiries, 0, reason: 'the countdown started when the loader signed in, not before');
      await elapse(tester, time, const Duration(minutes: 1));
      expect(expiries, 1);
    });

    testWidgets('no countdown is left running after sign-out', (tester) async {
      await pumpGuard(tester);
      await elapse(tester, time, const Duration(minutes: 2));
      await pumpGuard(tester, active: false);
      await elapse(tester, time, const Duration(minutes: 30));
      expect(expiries, 0);
    });

    testWidgets('a tab that was hidden past the deadline signs out the moment it is shown again, without waiting for a timer', (tester) async {
      await pumpGuard(tester);
      await elapse(tester, time, const Duration(minutes: 1));
      await hideAndRestore(tester, whileHidden: () => time.now = time.now.add(const Duration(minutes: 6))); // the browser ran no timers
      expect(expiries, 1);
    });

    testWidgets('a tab hidden for less than the deadline stays signed in when shown again', (tester) async {
      await pumpGuard(tester);
      await hideAndRestore(tester, whileHidden: () => time.now = time.now.add(const Duration(minutes: 2)));
      expect(expiries, 0);
    });
  });

  group('the loader app on a shared tablet', () {
    late TestTime time;
    late MemoryBrowser browser;
    late AuthService auth;
    late GlobalKey<NavigatorState> navigatorKey;

    Future<void> pumpApp(WidgetTester tester, {Size size = const Size(1400, 900)}) async {
      tester.view.physicalSize = size;
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.reset);
      final server = fakeServer();
      browser = MemoryBrowser(location: Uri.parse('https://app.example.test/loader-app/'), origin: 'https://app.example.test')..storage[sessionKey] = sessionJson();
      auth = AuthService(config: idp, client: server, browser: browser, apiBase: apiBase, autoRenew: false);
      await auth.start();
      await tester.pumpWidget(WaypointLoaderApp(
        auth: auth,
        now: time.read,
        navigatorKey: navigatorKey,
        apiClientFor: (a) => ApiClient(tokenProvider: a.validToken, client: server, baseUrl: apiBase),
      ));
      await tester.pump();
    }

    setUp(() {
      time = TestTime();
      navigatorKey = GlobalKey<NavigatorState>();
    });

    testWidgets('five minutes of inactivity signs the loader out and the sign-in screen says why', (tester) async {
      await pumpApp(tester);
      expect(find.text('Nimal Perera'), findsOneWidget);
      await elapse(tester, time, const Duration(minutes: 4, seconds: 59));
      expect(auth.status, AuthStatus.signedIn);
      await elapse(tester, time, const Duration(seconds: 1));
      await tester.pump();

      expect(auth.status, AuthStatus.signedOut);
      expect(browser.storage.containsKey(sessionKey), isFalse, reason: 'the session is cleared');
      expect(find.byType(SignInScreen), findsOneWidget);
      expect(find.text(inactivityMessage), findsOneWidget);
      expect(find.text('Nimal Perera'), findsNothing, reason: 'nothing of the previous loader is left on screen');
      expect(find.text('Switch user'), findsNothing);
    });

    testWidgets('touching the app keeps the loader signed in', (tester) async {
      await pumpApp(tester);
      await elapse(tester, time, const Duration(minutes: 4));
      await tester.tapAt(const Offset(700, 450));
      await elapse(tester, time, const Duration(minutes: 4));
      expect(auth.status, AuthStatus.signedIn);
      await elapse(tester, time, const Duration(minutes: 1));
      await tester.pump();
      expect(auth.status, AuthStatus.signedOut);
    });

    testWidgets('a tab restored after the deadline shows the sign-in screen at once', (tester) async {
      await pumpApp(tester);
      await hideAndRestore(tester, whileHidden: () => time.now = time.now.add(const Duration(minutes: 7)));
      await tester.pump();
      expect(auth.status, AuthStatus.signedOut);
      expect(find.text(inactivityMessage), findsOneWidget);
    });

    testWidgets('the tablet header has a visible Switch user button that signs out straight away', (tester) async {
      final semantics = tester.ensureSemantics();
      await pumpApp(tester);
      expect(find.text('Sign out'), findsNothing, reason: 'on a shared tablet the action is Switch user');
      final button = find.widgetWithText(OutlinedButton, 'Switch user');
      expect(button, findsOneWidget);
      expect(find.bySemanticsLabel(RegExp('Switch user')), findsWidgets, reason: 'it has an accessible name');
      await tester.tap(button);
      await tester.pump();
      await tester.pump();

      expect(auth.status, AuthStatus.signedOut);
      expect(browser.storage.containsKey(sessionKey), isFalse);
      expect(find.byType(SignInScreen), findsOneWidget);
      expect(find.text('Nimal Perera'), findsNothing);
      expect(find.text(inactivityMessage), findsNothing, reason: 'a person who chose to switch is not told they were timed out');
      semantics.dispose();
    });

    testWidgets('the account menu offers Switch user as well', (tester) async {
      await pumpApp(tester);
      await tester.tap(find.byTooltip('Account'));
      await tester.pumpAndSettle();
      await tester.tap(find.descendant(of: find.byType(PopupMenuItem<String>), matching: find.text('Switch user')));
      await tester.pump();
      await tester.pump();
      expect(auth.status, AuthStatus.signedOut);
    });

    testWidgets('on a phone the bottom bar has Switch user', (tester) async {
      await pumpApp(tester, size: const Size(680, 900));
      expect(find.text('Sign out'), findsNothing);
      await tester.tap(find.descendant(of: find.byType(NavigationBar), matching: find.text('Switch user')));
      await tester.pump();
      await tester.pump();
      expect(auth.status, AuthStatus.signedOut);
      expect(find.byType(SignInScreen), findsOneWidget);
    });

    testWidgets('signing out closes every screen and dialog opened over the home screen', (tester) async {
      await pumpApp(tester);
      navigatorKey.currentState!.push(MaterialPageRoute<void>(builder: (_) => const Scaffold(body: Text("previous loader's open trip"))));
      await tester.pumpAndSettle();
      showDialog<void>(context: navigatorKey.currentState!.overlay!.context, builder: (_) => const AlertDialog(content: Text("previous loader's report")));
      await tester.pumpAndSettle();
      expect(find.text("previous loader's report"), findsOneWidget);

      await elapse(tester, time, const Duration(minutes: 5));
      await tester.pumpAndSettle();

      expect(find.text("previous loader's open trip"), findsNothing);
      expect(find.text("previous loader's report"), findsNothing);
      expect(find.byType(SignInScreen), findsOneWidget);
      expect(find.text(inactivityMessage), findsOneWidget);
    });

    testWidgets('activity in a dialog counts: the guard sits above the whole navigator', (tester) async {
      await pumpApp(tester);
      showDialog<void>(context: navigatorKey.currentState!.overlay!.context, builder: (_) => const AlertDialog(content: Text('a dialog')));
      await tester.pumpAndSettle();
      await elapse(tester, time, const Duration(minutes: 4));
      await tester.tap(find.text('a dialog'));
      await elapse(tester, time, const Duration(minutes: 4));
      expect(auth.status, AuthStatus.signedIn);
    });

    testWidgets("the reason survives a reload, as when the identity server's sign-out page returns the browser", (tester) async {
      await pumpApp(tester);
      await elapse(tester, time, const Duration(minutes: 5));
      await tester.pump();
      expect(browser.storage[reasonKey], isNotNull);

      final reloaded = AuthService(config: idp, client: fakeServer(), browser: browser, apiBase: apiBase, autoRenew: false);
      await reloaded.start();
      await tester.pumpWidget(MaterialApp(theme: waypointTheme(), home: SignInScreen(auth: reloaded)));
      expect(find.text(inactivityMessage), findsOneWidget);
      expect(browser.storage.containsKey(reasonKey), isFalse, reason: 'shown once');
    });
  });

  group('sign-out reasons', () {
    Future<(AuthService, MemoryBrowser)> signedIn() async {
      final browser = MemoryBrowser(location: Uri.parse('https://app.example.test/loader-app/'), origin: 'https://app.example.test')..storage[sessionKey] = sessionJson();
      final auth = AuthService(config: idp, client: fakeServer(), browser: browser, autoRenew: false);
      await auth.start();
      return (auth, browser);
    }

    test('a manual sign-out carries no message and clears an older one', () async {
      final (auth, browser) = await signedIn();
      browser.storage[reasonKey] = 'inactivity';
      await auth.signOut();
      expect(auth.error, isNull);
      expect(browser.storage.containsKey(reasonKey), isFalse);
    });

    testWidgets('an unsent report is named on the sign-in screen when inactivity discarded it', (tester) async {
      final (auth, _) = await signedIn();
      await auth.signOut(reason: SignInException(SignInError.inactivity, 'unsent'));
      await tester.pumpWidget(MaterialApp(theme: waypointTheme(), home: SignInScreen(auth: auth)));
      expect(find.textContaining(inactivityMessage), findsOneWidget);
      expect(find.textContaining('report that had not been sent was discarded'), findsOneWidget);
    });
  });

  group('Switch user with unfinished work', () {
    LoaderController controller() => LoaderController(api: ApiClient(tokenProvider: () async => 'tok', client: fakeServer(), baseUrl: apiBase));

    Future<void> pumpButton(WidgetTester tester, LoaderController c, void Function() signOut) => tester.pumpWidget(MaterialApp(home: Scaffold(body: SwitchUserButton(controller: c, onSignOut: signOut))));

    testWidgets('with nothing open it signs out without asking', (tester) async {
      var outs = 0;
      await pumpButton(tester, controller(), () => outs++);
      await tester.tap(find.text('Switch user'));
      await tester.pumpAndSettle();
      expect(outs, 1);
      expect(find.text('Switch user?'), findsNothing);
    });

    testWidgets('an open report is not discarded silently: it asks, and Stay signed in changes nothing', (tester) async {
      var outs = 0;
      final c = controller()..reportFormOpened(ReportAttempt());
      await pumpButton(tester, c, () => outs++);
      await tester.tap(find.text('Switch user'));
      await tester.pumpAndSettle();
      expect(find.text('Switch user?'), findsOneWidget);
      expect(outs, 0);
      await tester.tap(find.text('Stay signed in'));
      await tester.pumpAndSettle();
      expect(outs, 0);
      expect(find.text('Switch user?'), findsNothing);
    });

    testWidgets('and Discard and switch user signs out', (tester) async {
      var outs = 0;
      final c = controller()..reportFormOpened(ReportAttempt());
      await pumpButton(tester, c, () => outs++);
      await tester.tap(find.text('Switch user'));
      await tester.pumpAndSettle();
      await tester.tap(find.text('Discard and switch user'));
      await tester.pumpAndSettle();
      expect(outs, 1);
    });

    test('a closed report form no longer counts as unfinished', () {
      final c = controller();
      final a = ReportAttempt();
      expect(c.hasUnsentWork, isFalse);
      c.reportFormOpened(a);
      expect(c.hasUnsentWork, isTrue);
      c.reportFormClosed(a);
      expect(c.hasUnsentWork, isFalse);
    });
  });

  test('the guard defaults to the five-minute shared-tablet timeout', () {
    expect(const InactivityGuard(active: true, onExpired: _noop, child: SizedBox()).timeout, loaderInactivityTimeout);
  });
}

void _noop() {}
