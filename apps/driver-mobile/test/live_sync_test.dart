import 'dart:async';
import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:sqflite_common_ffi/sqflite_ffi.dart';
import 'package:waypoint_driver/auth/auth_failure.dart';
import 'package:waypoint_driver/auth/auth_gateway.dart';
import 'package:waypoint_driver/auth/profile_api.dart';
import 'package:waypoint_driver/data/driver_models.dart';
import 'package:waypoint_driver/sync/operations.dart';
import 'package:waypoint_driver/sync/sqlite_sync_queue.dart';
import 'package:waypoint_driver/sync/sync_worker.dart';

/// Opt-in check against a disposable local stack. It creates one real ROAD
/// incident, so never point it at production. No credentials are stored here.
void main() {
  final tokenFile = Platform.environment['WAYPOINT_LIVE_DRIVER_TOKEN_FILE'];
  final tripId = Platform.environment['WAYPOINT_LIVE_TRIP_ID'];
  final operationId = Platform.environment['WAYPOINT_LIVE_OPERATION_ID'];
  final apiBase = Platform.environment['WAYPOINT_LIVE_API_BASE_URL'];
  final enabled = tokenFile != null && tripId != null && operationId != null && apiBase != null;

  test('SQLite worker applies one incident through the local delivery API', () async {
    final target = Uri.parse(apiBase!);
    expect(target.scheme, 'http');
    expect(target.host, anyOf('127.0.0.1', 'localhost'), reason: 'This test may mutate only a local stack');
    sqfliteFfiInit();
    final directory = await Directory.systemTemp.createTemp('waypoint-live-sync-');
    final client = http.Client();
    final queue = SqliteSyncQueue(
        open: () => SqliteSyncQueue.openAt('${directory.path}/queue.db', factory: databaseFactoryFfi));
    await queue.useOwner('usr-driver');
    final event = Operations.incident(
      operationId: operationId!,
      trip: _trip(tripId!),
      category: 'ROAD',
      description: 'Temporary local sync verification',
      occurredAt: DateTime.now().toUtc(),
    ).toSyncEvent();
    await queue.enqueue(event);

    final done = Completer<void>();
    final worker = DeliverySyncWorker(
      queue: queue,
      auth: _TokenAuth(await File(tokenFile!).readAsString()),
      client: client,
      baseUrl: apiBase,
      interval: const Duration(hours: 1),
      onProgress: (progress, detail) {
        if (progress == SyncProgress.idle && !done.isCompleted) done.complete();
        if ((progress == SyncProgress.needsAttention || progress == SyncProgress.waitingForSignIn) && !done.isCompleted) {
          done.completeError(StateError('$progress: $detail'));
        }
      },
    );
    try {
      worker.start();
      await done.future.timeout(const Duration(seconds: 15));
      expect(await queue.pending(), isEmpty);
    } finally {
      worker.stop();
      client.close();
      await directory.delete(recursive: true);
    }
  }, skip: !enabled);
}

TripInfo _trip(String tripId) => TripInfo(
      tripId: tripId,
      runId: '',
      vehicleCode: 'VEH001',
      tripRef: tripId,
      depot: '',
      window: '',
      stops: const [],
    );

class _TokenAuth implements AuthGateway {
  _TokenAuth(this.token);
  final String token;

  @override
  Future<String?> accessToken() async => token.trim();

  @override
  Future<DriverProfile?> restore() async => null;

  @override
  Future<AuthOutcome> signIn() async => const AuthOutcome.failed(AuthFailure(AuthFailureKind.cancelled));

  @override
  Future<void> signOut() async {}
}
