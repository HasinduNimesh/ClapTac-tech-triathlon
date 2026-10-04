import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:sqflite_common_ffi/sqflite_ffi.dart';
import 'package:waypoint_driver/auth/auth_failure.dart';
import 'package:waypoint_driver/auth/auth_gateway.dart';
import 'package:waypoint_driver/auth/profile_api.dart';
import 'package:waypoint_driver/sync/operations.dart';
import 'package:waypoint_driver/sync/sqlite_sync_queue.dart';
import 'package:waypoint_driver/sync/sync.dart';
import 'package:waypoint_driver/sync/sync_worker.dart';

void main() {
  sqfliteFfiInit();

  late Directory directory;
  late String path;
  late SqliteSyncQueue queue;

  Future<SqliteSyncQueue> openQueue(String owner) async {
    final next = SqliteSyncQueue(open: () => SqliteSyncQueue.openAt(path, factory: databaseFactoryFfi));
    await next.useOwner(owner);
    return next;
  }

  setUp(() async {
    directory = await Directory.systemTemp.createTemp('waypoint-sync-test-');
    path = '${directory.path}/queue.db';
    queue = await openQueue('driver-one');
  });

  tearDown(() async {
    await directory.delete(recursive: true);
  });

  test('SQLite restores the exact operations in FIFO order after restart', () async {
    await queue.enqueue(_incident('first'));
    await queue.enqueue(_incident('second'));
    await queue.enqueue(_incident('first')); // An accidental repeat keeps the original ID and body.

    final reopened = await openQueue('driver-one');
    final items = await reopened.pending();
    expect(items.map((item) => item.idempotencyKey), ['first', 'second']);
    expect(items.first.payload, _incident('first').payload);
    expect(items.first.occurredAt, DateTime.utc(2026, 10, 3, 8));

    await reopened.markSynced('first');
    expect((await (await openQueue('driver-one')).first())!.event.idempotencyKey, 'second');
    expect(await (await openQueue('driver-two')).pending(), isEmpty);
  });

  test('a blocked result and its reason survive restart and hold later operations', () async {
    await queue.enqueue(_incident('first'));
    await queue.enqueue(_incident('second'));
    await queue.markBlocked('first', 'CONFLICT: route changed');

    final reopened = await openQueue('driver-one');
    expect((await reopened.first())!.state, 'blocked');
    expect((await reopened.first())!.failure, 'CONFLICT: route changed');
    expect((await reopened.pending()).map((item) => item.idempotencyKey), ['first', 'second']);
  });

  test('APPLIED and applied DUPLICATE drain the queue in order', () async {
    await queue.enqueue(_incident('first'));
    await queue.enqueue(_incident('second'));
    final sent = <String>[];
    final client = MockClient((request) async {
      expect(request.method, 'POST');
      expect(request.url.path, '/api/v1/delivery/sync');
      expect(request.headers['Authorization'], 'Bearer test-token');
      final operation = ((jsonDecode(request.body) as Map)['operations'] as List).single as Map;
      final id = operation['operationId'] as String;
      sent.add(id);
      return http.Response(jsonEncode({'results': [
        {'operationId': id, 'status': id == 'first' ? 'APPLIED' : 'DUPLICATE', if (id == 'second') 'originalStatus': 'APPLIED'}
      ]}), 200);
    });
    final worker = _worker(queue, client);
    final done = _nextProgress(worker, SyncProgress.idle);
    worker.start();
    await done;
    worker.stop();

    expect(sent, ['first', 'second']);
    expect(await queue.pending(), isEmpty);
    expect(await (await openQueue('driver-one')).first(), isNull);
  });

  test('temporary HTTP failure retains the same operation ID for retry', () async {
    await queue.enqueue(_incident('one'));
    var calls = 0;
    final ids = <String>[];
    final client = MockClient((request) async {
      calls++;
      ids.add((((jsonDecode(request.body) as Map)['operations'] as List).single as Map)['operationId'] as String);
      if (calls == 1) return http.Response('unavailable', 503);
      return http.Response(jsonEncode({'results': [{'operationId': 'one', 'status': 'APPLIED'}]}), 200);
    });
    final worker = _worker(queue, client);
    final offline = _nextProgress(worker, SyncProgress.offline);
    worker.start();
    await offline;
    expect((await queue.pending()).single.idempotencyKey, 'one');

    final drained = _nextProgress(worker, SyncProgress.idle);
    await worker.syncNow();
    await drained;
    worker.stop();
    expect(ids, ['one', 'one']);
    expect(await queue.pending(), isEmpty);
  });

  for (final status in ['CONFLICT', 'REJECTED']) {
    test('$status blocks that operation and preserves later operations', () async {
      await queue.enqueue(_incident('first'));
      await queue.enqueue(_incident('second'));
      final sent = <String>[];
      final client = MockClient((request) async {
        sent.add((((jsonDecode(request.body) as Map)['operations'] as List).single as Map)['operationId'] as String);
        return http.Response(jsonEncode({'results': [{'operationId': 'first', 'status': status, 'detail': 'needs review'}]}), 200);
      });
      final worker = _worker(queue, client);
      final blocked = _nextProgress(worker, SyncProgress.needsAttention);
      worker.start();
      await blocked;
      await worker.syncNow();
      worker.stop();

      expect(sent, ['first']);
      expect((await queue.first())!.state, 'blocked');
      expect((await queue.first())!.failure, '$status: needs review');
      expect((await queue.pending()).map((item) => item.idempotencyKey), ['first', 'second']);
    });
  }

  test('successful outcome without finalized proof is kept on device', () async {
    await queue.enqueue(_event('outcome', OperationType.stopOutcome, payload: {'code': 'DELIVERED'}));
    var requests = 0;
    final worker = _worker(queue, MockClient((_) async {
      requests++;
      return http.Response('{}', 200);
    }));
    final waiting = _nextProgress(worker, SyncProgress.waitingForProof);
    worker.start();
    await waiting;
    worker.stop();
    expect(requests, 0);
    expect((await queue.pending()).single.idempotencyKey, 'outcome');
  });

  test('prepared trip is not sent as an arrival before server start', () async {
    await queue.enqueue(_event('arrival', OperationType.arrived));
    final paths = <String>[];
    final worker = _worker(queue, MockClient((request) async {
      paths.add(request.url.path);
      return http.Response(jsonEncode({'run': {'status': 'prepared'}}), 200);
    }));
    final waiting = _nextProgress(worker, SyncProgress.waitingForTripStart);
    worker.start();
    await waiting;
    worker.stop();
    expect(paths, ['/api/v1/delivery/trips/trip-1']);
    expect((await queue.pending()).single.idempotencyKey, 'arrival');
  });
}

SyncEvent _incident(String id) => _event(id, OperationType.incidentReport,
    payload: {'category': 'ROAD', 'description': 'Road closed'});

SyncEvent _event(String id, String type, {Map<String, Object?> payload = const {}}) {
  final occurredAt = DateTime.utc(2026, 10, 3, 8);
  return SyncEvent(
    eventId: id,
    idempotencyKey: id,
    action: type,
    resourceType: 'trip',
    resourceId: 'trip-1',
    occurredAt: occurredAt,
    payload: {
      'operationId': id,
      'type': type,
      'tripId': 'trip-1',
      'runId': 'run-1',
      'occurredAt': occurredAt.toIso8601String(),
      'payload': payload,
    },
  );
}

DeliverySyncWorker _worker(SqliteSyncQueue queue, http.Client client) => DeliverySyncWorker(
      queue: queue,
      auth: _Auth(),
      client: client,
      baseUrl: 'https://waypoint.example',
      interval: const Duration(hours: 1),
    );

Future<void> _nextProgress(DeliverySyncWorker worker, SyncProgress expected) {
  final done = Completer<void>();
  worker.onProgress = (progress, _) {
    if (progress == expected && !done.isCompleted) done.complete();
  };
  return done.future.timeout(const Duration(seconds: 5));
}

class _Auth implements AuthGateway {
  @override
  Future<String?> accessToken() async => 'test-token';

  @override
  Future<DriverProfile?> restore() async => null;

  @override
  Future<AuthOutcome> signIn() async => const AuthOutcome.failed(AuthFailure(AuthFailureKind.cancelled));

  @override
  Future<void> signOut() async {}
}
