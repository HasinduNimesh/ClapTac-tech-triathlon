import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:waypoint_driver/app/driver_session.dart';
import 'package:waypoint_driver/auth/auth_failure.dart';
import 'package:waypoint_driver/auth/auth_gateway.dart';
import 'package:waypoint_driver/auth/profile_api.dart';
import 'package:waypoint_driver/main.dart';
import 'package:waypoint_driver/offline/local_database.dart';
import 'package:waypoint_driver/screens/sign_in/sign_in_screen.dart';
import 'package:waypoint_driver/sync/sync.dart';

import 'helpers/render.dart';

const _driver = DriverProfile(userId: 'USR006', subject: 'usr-driver', roles: ['DRIVER'], vehicleId: 'VEH001');

class FakeGateway implements AuthGateway {
  AuthOutcome Function() onSignIn = () => const AuthOutcome.signedIn(_driver);
  Completer<void>? hold;
  DriverProfile? restored;
  int signIns = 0;
  int signOuts = 0;

  @override
  Future<AuthOutcome> signIn() async {
    signIns++;
    await hold?.future;
    return onSignIn();
  }

  @override
  Future<DriverProfile?> restore() async => restored;

  @override
  Future<void> signOut() async => signOuts++;
}

Future<void> _boot(WidgetTester tester, FakeGateway gateway, {bool demoRoute = false}) async {
  await pumpScreen(tester, WaypointDriverApp(database: InMemoryLocalDatabase(), queue: InMemorySyncQueue(), auth: gateway, demoRoute: demoRoute));
}

DriverSession _session(FakeGateway gateway) => DriverSession(database: InMemoryLocalDatabase(), queue: InMemorySyncQueue(), auth: gateway);

void main() {
  setUpAll(loadAppFonts);

  group('sign-in screen in identity-provider mode', () {
    testWidgets('replaces the credential form with one button', (tester) async {
      var taps = 0;
      await pumpScreen(tester, SignInScreen(identityProviderMode: true, onSignIn: (_, __) => taps++));
      expect(find.byType(TextField), findsNothing);
      expect(find.text('Password'), findsNothing);
      expect(find.text('Forgot password?'), findsNothing);
      await tester.tap(find.text('Continue to sign in'));
      expect(taps, 1);
    });

    testWidgets('is disabled and says so while signing in', (tester) async {
      var taps = 0;
      await pumpScreen(tester, SignInScreen(identityProviderMode: true, busy: true, onSignIn: (_, __) => taps++));
      expect(find.text('Opening sign-in…'), findsOneWidget);
      await tester.tap(find.text('Opening sign-in…'));
      expect(taps, 0);
    });

    testWidgets('keeps the credential form when no identity provider is used', (tester) async {
      await pumpScreen(tester, SignInScreen(onSignIn: (_, __) {}));
      expect(find.byType(TextField), findsNWidgets(2));
      expect(find.text('Continue to sign in'), findsNothing);
    });
  });

  group('app flow with an identity provider', () {
    testWidgets('signing in without trips says there is no route instead of showing sample data', (tester) async {
      final gateway = FakeGateway();
      await _boot(tester, gateway);
      expect(find.byType(TextField), findsNothing);
      await tester.tap(find.text('Continue to sign in'));
      await tester.pumpAndSettle();
      expect(gateway.signIns, 1);
      expect(find.text('No route loaded'), findsOneWidget);
      expect(find.textContaining('USR006'), findsOneWidget);
      expect(find.textContaining('VEH001'), findsOneWidget);
      expect(find.text('Check the load before you leave'), findsNothing);
      expect(find.textContaining('Dehiwala'), findsNothing);
      expect(find.textContaining('OUT108'), findsNothing);
    });

    testWidgets('the demo route is shown after a real sign-in only when explicitly enabled', (tester) async {
      await _boot(tester, FakeGateway(), demoRoute: true);
      await tester.tap(find.text('Continue to sign in'));
      await tester.pumpAndSettle();
      expect(find.text('Check the load before you leave'), findsOneWidget);
      expect(find.text('No route loaded'), findsNothing);
    });

    testWidgets('a driver with no route can sign out, which clears the stored sign-in', (tester) async {
      final gateway = FakeGateway();
      await _boot(tester, gateway);
      await tester.tap(find.text('Continue to sign in'));
      await tester.pumpAndSettle();
      await tester.tap(find.text('Sign out'));
      await tester.pumpAndSettle();
      expect(gateway.signOuts, 1);
      expect(find.text('Continue to sign in'), findsOneWidget);
    });

    testWidgets('explains why a person without a Waypoint account cannot continue', (tester) async {
      final gateway = FakeGateway()..onSignIn = () => const AuthOutcome.failed(AuthFailure(AuthFailureKind.notProvisioned));
      await _boot(tester, gateway);
      await tester.tap(find.text('Continue to sign in'));
      await tester.pumpAndSettle();
      expect(find.textContaining('not set up in Waypoint'), findsOneWidget);
      expect(find.text('Check the load before you leave'), findsNothing);
    });

    testWidgets('a non-driver is told this app is for drivers', (tester) async {
      final gateway = FakeGateway()..onSignIn = () => const AuthOutcome.failed(AuthFailure(AuthFailureKind.accessDenied));
      await _boot(tester, gateway);
      await tester.tap(find.text('Continue to sign in'));
      await tester.pumpAndSettle();
      expect(find.textContaining('for drivers'), findsOneWidget);
    });

    testWidgets('cancelling the browser shows no error and allows another try', (tester) async {
      final gateway = FakeGateway()..onSignIn = () => const AuthOutcome.failed(AuthFailure(AuthFailureKind.cancelled));
      await _boot(tester, gateway);
      await tester.tap(find.text('Continue to sign in'));
      await tester.pumpAndSettle();
      expect(find.textContaining('not accepted'), findsNothing);
      expect(find.textContaining('Could not reach'), findsNothing);
      expect(find.text('Continue to sign in'), findsOneWidget);
    });

    testWidgets('the button is busy while the sign-in is in progress', (tester) async {
      final gateway = FakeGateway()..hold = Completer<void>();
      await _boot(tester, gateway);
      await tester.tap(find.text('Continue to sign in'));
      await tester.pump();
      expect(find.text('Opening sign-in…'), findsOneWidget);
      gateway.hold!.complete();
      await tester.pumpAndSettle();
      expect(find.text('No route loaded'), findsOneWidget);
    });

    testWidgets('a driver who is already signed in resumes without signing in again', (tester) async {
      final gateway = FakeGateway()..restored = _driver;
      await _boot(tester, gateway);
      expect(gateway.signIns, 0);
      expect(find.text('No route loaded'), findsOneWidget);
    });
  });

  group('session', () {
    test('keeps who signed in', () async {
      final session = _session(FakeGateway());
      expect(await session.signIn(), isTrue);
      expect(session.identity?.userId, 'USR006');
      expect(session.signedIn, isTrue);
    });

    test('a failed sign-in leaves the driver signed out with the reason', () async {
      final gateway = FakeGateway()..onSignIn = () => const AuthOutcome.failed(AuthFailure(AuthFailureKind.unavailable));
      final session = _session(gateway);
      expect(await session.signIn(), isFalse);
      expect(session.signedIn, isFalse);
      expect(session.signInError, contains('Could not reach Waypoint'));
    });

    test('a second tap while signing in does not start another sign-in', () async {
      final gateway = FakeGateway()..hold = Completer<void>();
      final session = _session(gateway);
      final first = session.signIn();
      expect(await session.signIn(), isFalse);
      gateway.hold!.complete();
      await first;
      expect(gateway.signIns, 1);
    });

    test('finishing the trip signs out and forgets the identity', () async {
      final gateway = FakeGateway();
      final session = _session(gateway);
      await session.signIn();
      await session.finishTrip();
      expect(gateway.signOuts, 1);
      expect(session.signedIn, isFalse);
      expect(session.identity, isNull);
    });

    test('a demo build without a gateway still only works when demo sign-in is on', () async {
      final off = DriverSession(database: InMemoryLocalDatabase(), queue: InMemorySyncQueue());
      expect(await off.signIn(), isFalse);
      final on = DriverSession(database: InMemoryLocalDatabase(), queue: InMemorySyncQueue(), demoAuth: true);
      expect(await on.signIn(), isTrue);
    });
  });
}
