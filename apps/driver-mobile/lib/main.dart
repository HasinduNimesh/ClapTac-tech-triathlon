import 'package:flutter/material.dart';
import 'package:http/http.dart' as http;

import 'app/demo_flags.dart';
import 'app/driver_flow.dart';
import 'app/driver_session.dart';
import 'auth/auth_config.dart';
import 'auth/auth_gateway.dart';
import 'auth/auth_store.dart';
import 'auth/oidc_client.dart';
import 'auth/profile_api.dart';
import 'offline/local_database.dart';
import 'sync/sync.dart';
import 'theme/app_theme.dart';

AuthGateway? _buildAuthGateway() {
  final config = AuthConfig.fromEnvironment();
  if (!config.isConfigured) return null;
  return OidcAuthGateway(
    config: config,
    client: AppAuthOidcClient(config),
    profiles: ProfileApi(client: http.Client(), baseUrl: config.apiBaseUrl),
    store: SecureAuthStore(),
  );
}

void main() {
  final demo = DemoFlags.fromEnvironment();
  runApp(WaypointDriverApp(
    // Real sign-in when OIDC_ISSUER and API_BASE_URL are provided; otherwise sign-in stays disabled.
    auth: _buildAuthGateway(),
    database: InMemoryLocalDatabase(),
    queue: InMemorySyncQueue(),
    // Demo switches are forced off in release builds (see DemoFlags).
    demoAuth: demo.auth,
    demoRoute: demo.route,
    demoUpdates: demo.updates,
  ));
}

class WaypointDriverApp extends StatefulWidget {
  const WaypointDriverApp({
    super.key,
    required this.database,
    required this.queue,
    this.demoUpdates = false,
    this.demoAuth = false,
    this.demoRoute = false,
    this.auth,
  });

  final LocalDatabase database;
  final SyncQueue queue;
  final bool demoUpdates;
  final bool demoAuth;
  final bool demoRoute;
  final AuthGateway? auth;

  @override
  State<WaypointDriverApp> createState() => _WaypointDriverAppState();
}

class _WaypointDriverAppState extends State<WaypointDriverApp> {
  late final DriverSession _session = DriverSession(
    database: widget.database,
    queue: widget.queue,
    demoUpdates: widget.demoUpdates,
    demoAuth: widget.demoAuth,
    demoRoute: widget.demoRoute,
    auth: widget.auth,
  );

  @override
  void initState() {
    super.initState();
    _session.restore();
  }

  @override
  void dispose() {
    _session.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'Waypoint Driver',
      debugShowCheckedModeBanner: false,
      theme: buildAppTheme(),
      home: DriverFlow(session: _session),
    );
  }
}
