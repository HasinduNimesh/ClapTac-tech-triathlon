import 'package:flutter/material.dart';

import 'api/api_client.dart';
import 'auth/auth.dart';
import 'auth/sign_in_screen.dart';
import 'driver/driver_controller.dart';
import 'driver/driver_home.dart';
import 'loader/loader_controller.dart';
import 'loader/loader_home.dart';
import 'offline/store.dart';
import 'sync/sync.dart';
import 'theme/tokens.dart';

void main() {
  WidgetsFlutterBinding.ensureInitialized();
  final store = PrefsStore();
  runApp(WaypointFieldApp(store: store, auth: AuthService(store: store)..restore()));
}

/// Waypoint field app: the driver's phone route and the loader's dock tablet
/// (with a phone layout). The signed-in account decides which one opens.
class WaypointFieldApp extends StatelessWidget {
  const WaypointFieldApp({super.key, required this.store, required this.auth});

  final LocalStore store;
  final AuthService auth;

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'Waypoint',
      debugShowCheckedModeBanner: false,
      theme: waypointTheme(),
      home: ListenableBuilder(listenable: auth, builder: (context, _) => _RoleHome(store: store, auth: auth)),
    );
  }
}

class _RoleHome extends StatefulWidget {
  const _RoleHome({required this.store, required this.auth});
  final LocalStore store;
  final AuthService auth;

  @override
  State<_RoleHome> createState() => _RoleHomeState();
}

class _RoleHomeState extends State<_RoleHome> {
  String? _builtFor;
  DriverController? _driver;
  LoaderController? _loader;
  String? _handedOverBy;

  void _build(AuthSession s) {
    if (_builtFor == s.subject) return;
    _builtFor = s.subject;
    final api = ApiClient(tokenProvider: () => widget.auth.session?.accessToken ?? '');
    final sync = SyncEngine(store: widget.store, api: api, ownerId: s.subject);
    _driver = s.profile.role == 'DRIVER' ? DriverController(api: api, store: widget.store, sync: sync, ownerId: s.subject) : null;
    _loader = s.profile.role == 'LOADER' ? LoaderController(api: api, store: widget.store, sync: sync, ownerId: s.subject) : null;
  }

  Future<void> _signOut({bool handover = false}) async {
    final name = widget.auth.session?.profile.displayName ?? widget.auth.session?.profile.userId;
    _builtFor = null;
    _driver = null;
    _loader = null;
    await widget.auth.signOut();
    if (mounted) setState(() => _handedOverBy = handover ? name : null);
  }

  @override
  Widget build(BuildContext context) {
    final s = widget.auth.session;
    if (s == null) return SignInScreen(auth: widget.auth, switchingFrom: _handedOverBy);
    _build(s);
    if (_loader != null) return LoaderHome(controller: _loader!, profile: s.profile, onSwitchUser: () => _signOut(handover: true));
    if (_driver != null) return DriverHome(controller: _driver!, onSignOut: _signOut);
    return Scaffold(body: Center(child: Padding(padding: const EdgeInsets.all(24), child: Column(mainAxisSize: MainAxisSize.min, children: [
      const Text('This app is for drivers and loaders.', style: TextStyle(fontSize: 18, fontWeight: FontWeight.w600)),
      const SizedBox(height: 12),
      FilledButton(onPressed: _signOut, child: const Text('Sign out')),
    ]))));
  }
}
