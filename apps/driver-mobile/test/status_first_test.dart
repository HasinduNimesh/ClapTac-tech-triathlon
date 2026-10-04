import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:sqflite_common_ffi/sqflite_ffi.dart';
import 'package:waypoint_driver/app/driver_prefs.dart';
import 'package:waypoint_driver/data/driver_models.dart';
import 'package:waypoint_driver/sync/operations.dart';
import 'package:waypoint_driver/sync/sqlite_sync_queue.dart';

const _a = StopInfo(stopId: 'stop-a', sequence: 1, outletCode: 'OUT1', name: 'Dehiwala', windowStart: '08:00', windowEnd: '09:00', units: 10, accessNote: '', contactNote: '', goods: 'Ambient');
const _b = StopInfo(stopId: 'stop-b', sequence: 2, outletCode: 'OUT2', name: 'Nugegoda', windowStart: '09:00', windowEnd: '10:00', units: 8, accessNote: '', contactNote: '', goods: 'Ambient');
const _trip = TripInfo(tripId: 'trip-1', runId: 'run-1', vehicleCode: 'VEH001', tripRef: 'PLAN1', depot: 'D', window: '', stops: [_a, _b]);
final _at = DateTime.utc(2026, 10, 4, 8);

void main() {
  sqfliteFfiInit();
  late Directory directory;
  late SqliteSyncQueue queue;

  setUp(() async {
    directory = await Directory.systemTemp.createTemp('waypoint-status-first-');
    queue = SqliteSyncQueue(open: () => SqliteSyncQueue.openAt('${directory.path}/queue.db', factory: databaseFactoryFfi));
    await queue.useOwner('driver-one');
  });
  tearDown(() => directory.delete(recursive: true));

  Future<void> add(OfflineOperation op) => queue.enqueue(op.toSyncEvent());
  CapturedProof photo() => CapturedProof(kind: ProofKind.photo, path: '${directory.path}/a.jpg', mimeType: 'image/jpeg', capturedAt: _at);

  test('the next stop\'s arrival goes ahead of the previous stop\'s photo; that stop\'s outcome waits for it', () async {
    await add(Operations.arrived(operationId: 'arr-a', trip: _trip, stop: _a, occurredAt: _at));
    await add(Operations.proofUpload(operationId: 'photo-a', trip: _trip, stop: _a, proof: photo(), occurredAt: _at));
    await add(Operations.stopOutcome(operationId: 'out-a', trip: _trip, stop: _a, draft: const DeliveryDraft(outcome: DeliveryOutcome.delivered), occurredAt: _at, proofOperationId: 'photo-a'));
    await add(Operations.arrived(operationId: 'arr-b', trip: _trip, stop: _b, occurredAt: _at));

    expect((await queue.next())!.event.idempotencyKey, 'arr-a', reason: 'the oldest item is a status update');
    await queue.markSynced('arr-a');
    expect((await queue.next())!.event.idempotencyKey, 'arr-b', reason: 'status before the waiting photo');
    await queue.markSynced('arr-b');
    expect((await queue.next())!.event.idempotencyKey, 'photo-a', reason: 'the delivered outcome needs its photo first');
    await queue.markSynced('photo-a');
    expect((await queue.next())!.event.idempotencyKey, 'out-a');
  });

  test('finishing the route never overtakes a photo, and a blocked item still holds the queue', () async {
    await add(Operations.proofUpload(operationId: 'photo-a', trip: _trip, stop: _a, proof: photo(), occurredAt: _at));
    await add(Operations.routeCompleted(operationId: 'done', trip: _trip, occurredAt: _at));
    expect((await queue.next())!.event.idempotencyKey, 'photo-a');
    await queue.markBlocked('photo-a', 'file missing');
    expect((await queue.next())!.state, 'blocked');
  });

  test('low-data mode takes smaller photos and checks messages less often, and is remembered', () async {
    final store = MemoryDriverPrefsStore();
    final prefs = DriverPrefs(store: store);
    await prefs.load();
    expect(prefs.photoSize.maxSide, 1280);
    expect(prefs.messagePoll(const Duration(seconds: 30)), const Duration(seconds: 30));
    await prefs.setLowData(true);
    expect(prefs.photoSize.maxSide, 960);
    expect(prefs.messagePoll(const Duration(seconds: 30)), const Duration(minutes: 2));
    await prefs.markIntroSeen();
    final again = DriverPrefs(store: store);
    await again.load();
    expect(again.lowData, isTrue);
    expect(again.introSeen, isTrue);
  });
}
