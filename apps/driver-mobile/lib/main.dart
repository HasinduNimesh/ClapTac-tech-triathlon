import 'package:flutter/material.dart';

import 'offline/local_database.dart';
import 'sync/sync.dart';

void main() {
  runApp(WaypointDriverApp(
    database: InMemoryLocalDatabase(),
    queue: InMemorySyncQueue(),
  ));
}

class WaypointDriverApp extends StatelessWidget {
  const WaypointDriverApp({
    super.key,
    required this.database,
    required this.queue,
  });

  final LocalDatabase database;
  final SyncQueue queue;

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'Waypoint Driver',
      home: Scaffold(
        appBar: AppBar(title: const Text('Waypoint Driver')),
        body: const Padding(
          padding: EdgeInsets.all(16),
          child: Text(
            'Offline-first driver shell. The AI agent is not required for route execution. '
            'Sync uses client-generated idempotency keys.',
          ),
        ),
      ),
    );
  }
}
