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
import 'sync/sqlite_sync_queue.dart';
import 'sync/sync_worker.dart';
import 'trips/trip_source.dart';
import 'trips/trips_api.dart';
import 'theme/app_theme.dart';

({AuthGateway? auth, TripSource? trips, SyncQueue queue, DeliverySyncWorker? worker}) _buildServices() {
  final config = AuthConfig.fromEnvironment();
  if (!config.isConfigured) return (auth: null, trips: null, queue: InMemorySyncQueue(), worker: null);
  final auth = OidcAuthGateway(
    config: config,
    client: AppAuthOidcClient(config),
    profiles: ProfileApi(client: http.Client(), baseUrl: config.apiBaseUrl),
    store: SecureAuthStore(),
  );
  final queue = SqliteSyncQueue();
  return (
    auth: auth,
    trips: ApiTripSource(api: TripsApi(client: http.Client(), baseUrl: config.apiBaseUrl), auth: auth),
    queue: queue,
    worker: DeliverySyncWorker(queue: queue, auth: auth, client: http.Client(), baseUrl: config.apiBaseUrl),
  );
}

void main() {
  final demo = DemoFlags.fromEnvironment();
  final services = _buildServices();
  runApp(WaypointDriverApp(
    // Real sign-in when OIDC_ISSUER and API_BASE_URL are provided; otherwise sign-in stays disabled.
    auth: services.auth,
    trips: services.trips,
    database: InMemoryLocalDatabase(),
    queue: services.queue,
    worker: services.worker,
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
    this.trips,
    this.worker,
  });

  final LocalDatabase database;
  final SyncQueue queue;
  final bool demoUpdates;
  final bool demoAuth;
  final bool demoRoute;
  final AuthGateway? auth;
  final TripSource? trips;
  final DeliverySyncWorker? worker;

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
    trips: widget.trips,
    worker: widget.worker,
  );

  @override
  void initState() {
    super.initState();
    _session.restore();
  }

  @override
  void dispose() {
    widget.worker?.stop();
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
