import 'dart:async';

import 'package:flutter/material.dart';

import 'api/api_client.dart';
import 'auth/auth.dart';
import 'auth/inactivity_guard.dart';
import 'auth/inactivity_monitor.dart';
import 'auth/sign_in_screen.dart';
import 'loader/loader_controller.dart';
import 'loader/loader_home.dart';
import 'theme/tokens.dart';

void main() {
  WidgetsFlutterBinding.ensureInitialized();
  runApp(WaypointLoaderApp(auth: AuthService()..start()));
}

/// Waypoint loader workspace (Flutter web): dock tablet and phone layouts,
/// always online against the loading, planning and order services.
///
/// The dock tablet is shared (workflow W6): after [inactivityTimeout] without a touch or key
/// press the loader is signed out, and *Switch user* signs them out at once. Either way the
/// next person gets the sign-in screen with nothing of the previous loader's on it.
class WaypointLoaderApp extends StatefulWidget {
  const WaypointLoaderApp({
    super.key,
    required this.auth,
    this.inactivityTimeout = loaderInactivityTimeout,
    this.now,
    this.apiClientFor,
    this.navigatorKey,
  });

  final AuthService auth;
  final Duration inactivityTimeout;

  /// A clock for tests; defaults to the real one.
  final DateTime Function()? now;

  /// Builds the API client for a signed-in loader; tests pass one that talks to a fake server.
  final ApiClient Function(AuthService auth)? apiClientFor;

  /// Tests use their own key to open screens above the home screen.
  final GlobalKey<NavigatorState>? navigatorKey;

  @override
  State<WaypointLoaderApp> createState() => _WaypointLoaderAppState();
}

class _WaypointLoaderAppState extends State<WaypointLoaderApp> {
  final _ownNavigatorKey = GlobalKey<NavigatorState>();
  LoaderController? _controller;
  String? _userId;

  GlobalKey<NavigatorState> get _navigatorKey => widget.navigatorKey ?? _ownNavigatorKey;

  @override
  void initState() {
    super.initState();
    widget.auth.addListener(_onAuthChanged);
  }

  @override
  void dispose() {
    widget.auth.removeListener(_onAuthChanged);
    super.dispose();
  }

  // Signed out, for whatever reason: drop the previous person's data and close every screen,
  // dialog and sheet that was opened over the home screen, so only the sign-in screen is left.
  void _onAuthChanged() {
    if (widget.auth.status == AuthStatus.signedIn) return;
    _controller = null;
    _userId = null;
    _navigatorKey.currentState?.popUntil((route) => route.isFirst);
  }

  /// The controller belongs to the person, not to the access token: the token is renewed
  /// every few minutes and the open trip, date and readings must survive that.
  LoaderController _controllerFor(AuthSession s) {
    if (_userId != s.profile.userId || _controller == null) {
      _userId = s.profile.userId;
      final auth = widget.auth;
      final api = widget.apiClientFor?.call(auth) ?? ApiClient(tokenProvider: auth.validToken, onUnauthorized: auth.renewAfterRefusal);
      _controller = LoaderController(api: api);
    }
    return _controller!;
  }

  void _signOutForInactivity() {
    // Nobody is there to ask, so an unfinished report is discarded; the sign-in screen says so.
    final unsent = _controller?.hasUnsentWork ?? false;
    unawaited(widget.auth.signOut(reason: SignInException(SignInError.inactivity, unsent ? 'unsent' : '')));
  }

  @override
  Widget build(BuildContext context) {
    final auth = widget.auth;
    return MaterialApp(
      title: 'Waypoint Loader',
      debugShowCheckedModeBanner: false,
      theme: waypointTheme(),
      navigatorKey: _navigatorKey,
      // Above the Navigator, so every screen and dialog counts as "the loader is still here".
      builder: (context, child) => ListenableBuilder(
        listenable: auth,
        builder: (context, _) => InactivityGuard(
          active: auth.status == AuthStatus.signedIn,
          timeout: widget.inactivityTimeout,
          now: widget.now,
          onExpired: _signOutForInactivity,
          child: child ?? const SizedBox.shrink(),
        ),
      ),
      home: ListenableBuilder(listenable: auth, builder: (context, _) => _Home(auth: auth, controllerFor: _controllerFor)),
    );
  }
}

class _Home extends StatelessWidget {
  const _Home({required this.auth, required this.controllerFor});
  final AuthService auth;
  final LoaderController Function(AuthSession) controllerFor;

  @override
  Widget build(BuildContext context) {
    final s = auth.session;
    if (auth.status == AuthStatus.starting) {
      return const Scaffold(body: Center(child: CircularProgressIndicator()));
    }
    if (auth.status != AuthStatus.signedIn || s == null) {
      return SignInScreen(auth: auth);
    }
    return LoaderHome(controller: controllerFor(s), profile: s.profile, onSignOut: auth.signOut);
  }
}
