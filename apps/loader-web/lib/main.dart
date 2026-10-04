import 'package:flutter/material.dart';

import 'api/api_client.dart';
import 'auth/auth.dart';
import 'auth/sign_in_screen.dart';
import 'loader/loader_controller.dart';
import 'loader/loader_home.dart';
import 'theme/tokens.dart';

void main() {
  WidgetsFlutterBinding.ensureInitialized();
  runApp(WaypointLoaderApp(auth: AuthService()..restore()));
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
  String? _token;

  @override
  Widget build(BuildContext context) {
    final s = widget.auth.session;
    if (s == null || s.expired) {
      _controller = null;
      _token = null;
      return SignInScreen(auth: widget.auth);
    }
    if (_token != s.accessToken) {
      _token = s.accessToken;
      _controller = LoaderController(api: ApiClient(tokenProvider: () => widget.auth.session?.accessToken ?? ''));
    }
    return LoaderHome(controller: _controller!, profile: s.profile, onSignOut: widget.auth.signOut);
  }
}
