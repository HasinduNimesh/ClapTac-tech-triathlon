import 'package:flutter/material.dart';

import 'api/api_client.dart';
import 'auth/auth.dart';
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
class WaypointLoaderApp extends StatelessWidget {
  const WaypointLoaderApp({super.key, required this.auth});

  final AuthService auth;

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'Waypoint Loader',
      debugShowCheckedModeBanner: false,
      theme: waypointTheme(),
      home: ListenableBuilder(listenable: auth, builder: (context, _) => _Home(auth: auth)),
    );
  }
}

class _Home extends StatefulWidget {
  const _Home({required this.auth});
  final AuthService auth;

  @override
  State<_Home> createState() => _HomeState();
}

class _HomeState extends State<_Home> {
  LoaderController? _controller;
  String? _userId;

  @override
  Widget build(BuildContext context) {
    final auth = widget.auth;
    final s = auth.session;
    if (auth.status == AuthStatus.starting) {
      return const Scaffold(body: Center(child: CircularProgressIndicator()));
    }
    if (auth.status != AuthStatus.signedIn || s == null) {
      _controller = null;
      _userId = null;
      return SignInScreen(auth: auth);
    }
    // The controller belongs to the person, not to the access token: the token is renewed
    // every few minutes and the open trip, date and readings must survive that.
    if (_userId != s.profile.userId || _controller == null) {
      _userId = s.profile.userId;
      _controller = LoaderController(api: ApiClient(tokenProvider: auth.validToken, onUnauthorized: auth.renewAfterRefusal));
    }
    return LoaderHome(controller: _controller!, profile: s.profile, onSignOut: auth.signOut);
  }
}
