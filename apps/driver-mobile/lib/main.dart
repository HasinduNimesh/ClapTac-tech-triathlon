import 'package:flutter/material.dart';

import 'app/driver_flow.dart';
import 'app/driver_session.dart';
import 'offline/local_database.dart';
import 'sync/sync.dart';
import 'theme/app_theme.dart';

void main() {
  runApp(WaypointDriverApp(
    database: InMemoryLocalDatabase(),
    queue: InMemorySyncQueue(),
    // flutter run --dart-define=DEMO_UPDATES=true adds a sample "plan update" so that screen can be reached.
    demoUpdates: const bool.fromEnvironment('DEMO_UPDATES'),
    // Off by default: staff accounts are not connected, so a normal build cannot sign in.
    // flutter run --dart-define=DEMO_AUTH=true accepts any credentials for demos.
    demoAuth: const bool.fromEnvironment('DEMO_AUTH'),
  ));
}

class WaypointDriverApp extends StatefulWidget {
  const WaypointDriverApp({
    super.key,
    required this.database,
    required this.queue,
    this.demoUpdates = false,
    this.demoAuth = false,
  });

  final LocalDatabase database;
  final SyncQueue queue;
  final bool demoUpdates;
  final bool demoAuth;

  @override
  State<WaypointDriverApp> createState() => _WaypointDriverAppState();
}

class _WaypointDriverAppState extends State<WaypointDriverApp> {
  late final DriverSession _session = DriverSession(
    database: widget.database,
    queue: widget.queue,
    demoUpdates: widget.demoUpdates,
    demoAuth: widget.demoAuth,
  );

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
