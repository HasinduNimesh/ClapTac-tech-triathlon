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
import 'package:waypoint_driver/data/driver_models.dart';
import 'package:waypoint_driver/sync/operations.dart';
import 'package:waypoint_driver/sync/sqlite_sync_queue.dart';
import 'package:waypoint_driver/sync/sync_worker.dart';

const _stop = StopInfo(stopId: 'stop-1', sequence: 1, outletCode: 'OUT1', name: 'Dehiwala', windowStart: '08:00', windowEnd: '09:00', units: 10, accessNote: '', contactNote: '', goods: 'Ambient');
const _trip = TripInfo(tripId: 'trip-1', runId: 'run-1', vehicleCode: 'VEH001', tripRef: 'PLAN1', depot: 'D', window: '', stops: [_stop]);
final _at = DateTime.utc(2026, 10, 3, 8);

// The smallest bytes that carry a real JPEG / PNG signature.
final _jpeg = [0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46];
final _png = [0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A];

void main() {
  sqfliteFfiInit();

  late Directory directory;
  late SqliteSyncQueue queue;

  setUp(() async {
    directory = await Directory.systemTemp.createTemp('waypoint-proof-test-');
    queue = SqliteSyncQueue(open: () => SqliteSyncQueue.openAt('${directory.path}/queue.db', factory: databaseFactoryFfi));
    await queue.useOwner('driver-one');
  });

  tearDown(() => directory.delete(recursive: true));

  Future<CapturedProof> proofFile(ProofKind kind, {String receiver = ''}) async {
    final isPhoto = kind == ProofKind.photo;
    final file = File('${directory.path}/${isPhoto ? 'photo.jpg' : 'signature.png'}');
    await file.writeAsBytes(isPhoto ? _jpeg : _png);
    return CapturedProof(kind: kind, path: file.path, mimeType: isPhoto ? 'image/jpeg' : 'image/png', capturedAt: _at, receiverName: receiver);
  }

  Future<void> queueProof(String id, CapturedProof proof) => queue.enqueue(Operations.proofUpload(operationId: id, trip: _trip, stop: _stop, proof: proof, occurredAt: _at).toSyncEvent());

  group('the proof operation', () {
    test('describes the file and what the server needs to know about it', () async {
      final op = Operations.proofUpload(operationId: 'p1', trip: _trip, stop: _stop, proof: await proofFile(ProofKind.photo, receiver: 'S. Perera'), occurredAt: _at);
      expect(op.type, 'PROOF_UPLOAD');
      expect(op.stopId, 'stop-1');
      expect(op.payload['proofType'], 'PHOTO');
      expect(op.payload['mimeType'], 'image/jpeg');
      expect(op.payload['capturedAt'], '2026-10-03T08:00:00.000Z');
      expect(op.payload['receiverName'], 'S. Perera');
      final signature = Operations.proofUpload(operationId: 'p2', trip: _trip, stop: _stop, proof: await proofFile(ProofKind.signature), occurredAt: _at);
      expect(signature.payload['proofType'], 'SIGNATURE');
      expect(signature.payload.containsKey('receiverName'), isFalse);
    });
  });

  group('the sync worker uploading proof', () {
    test('sends arrival, then the proof as multipart, then the outcome that depends on it', () async {
      final photo = await proofFile(ProofKind.photo, receiver: 'S. Perera');
      await queue.enqueue(Operations.arrived(operationId: 'arr', trip: _trip, stop: _stop, occurredAt: _at).toSyncEvent());
      await queue.enqueue(Operations.proofUpload(operationId: 'proof-op', trip: _trip, stop: _stop, proof: photo, occurredAt: _at).toSyncEvent());
      await queue.enqueue(Operations.stopOutcome(
        operationId: 'out',
        trip: _trip,
        stop: _stop,
        draft: const DeliveryDraft(outcome: DeliveryOutcome.delivered),
        occurredAt: _at,
        proofOperationId: 'proof-op',
      ).toSyncEvent());

      final order = <String>[];
      late http.Request proofRequest;
      final worker = _worker(queue, MockClient((request) async {
        if (request.method == 'GET') return http.Response(jsonEncode({'run': {'status': 'in_progress'}}), 200);
        if (request.url.path.endsWith('/sync')) {
          final op = ((jsonDecode(request.body) as Map)['operations'] as List).single as Map;
          order.add('${op['type']}${op['dependsOnOperationId'] == null ? '' : '<-${op['dependsOnOperationId']}'}');
          return http.Response(jsonEncode({'results': [{'operationId': op['operationId'], 'status': 'APPLIED'}]}), 200);
        }
        proofRequest = request;
        order.add('PROOF');
        return http.Response(jsonEncode({'proof': {'id': 'x'}}), 201);
      }));
      final drained = _nextProgress(worker, SyncProgress.idle);
      worker.start();
      await drained;
      worker.stop();

      expect(order, ['ARRIVED', 'PROOF', 'STOP_OUTCOME<-proof-op']);
      expect(proofRequest.url.path, '/api/v1/delivery/trips/trip-1/stops/stop-1/proofs');
      expect(proofRequest.headers['Idempotency-Key'], 'proof-op');
      expect(proofRequest.headers['Authorization'], 'Bearer test-token');
      expect(proofRequest.headers['content-type'], startsWith('multipart/form-data; boundary='));
      final body = latin1.decode(proofRequest.bodyBytes);
      expect(body, contains('name="type"\r\n\r\nPHOTO'));
      expect(body, contains('name="capturedAt"\r\n\r\n2026-10-03T08:00:00.000Z'));
      expect(body, contains('name="receiverName"\r\n\r\nS. Perera'));
      expect(body, contains('name="file"; filename="photo.jpg"'));
      expect(body.toLowerCase(), contains('content-type: image/jpeg'));
      expect(_contains(proofRequest.bodyBytes, _jpeg), isTrue, reason: 'the exact file bytes are in the upload');
      expect(await queue.pending(), isEmpty);
      expect(File(photo.path).existsSync(), isFalse, reason: 'the local copy is removed once the server has the proof');
    });

    test('a signature is sent as a PNG', () async {
      await queueProof('sig', await proofFile(ProofKind.signature));
      late http.Request seen;
      final worker = _worker(queue, _ok((r) => seen = r));
      final drained = _nextProgress(worker, SyncProgress.idle);
      worker.start();
      await drained;
      worker.stop();
      final body = latin1.decode(seen.bodyBytes);
      expect(body, contains('name="type"\r\n\r\nSIGNATURE'));
      expect(body.toLowerCase(), contains('content-type: image/png'));
      expect(body, isNot(contains('name="receiverName"')));
    });

    test('a retry after a failed upload uses the same key and the file is still there', () async {
      final photo = await proofFile(ProofKind.photo);
      await queueProof('proof-op', photo);
      final keys = <String?>[];
      var calls = 0;
      final worker = _worker(queue, MockClient((request) async {
        if (request.method == 'GET') return http.Response(jsonEncode({'run': {'status': 'in_progress'}}), 200);
        keys.add(request.headers['Idempotency-Key']);
        return ++calls == 1 ? http.Response('unavailable', 503) : http.Response('{}', 201);
      }));
      final offline = _nextProgress(worker, SyncProgress.offline);
      worker.start();
      await offline;
      expect(File(photo.path).existsSync(), isTrue);
      expect((await queue.pending()).single.idempotencyKey, 'proof-op');
      final drained = _nextProgress(worker, SyncProgress.idle);
      await worker.syncNow();
      await drained;
      worker.stop();
      expect(keys, ['proof-op', 'proof-op']);
      expect(await queue.pending(), isEmpty);
    });

    test('a proof waits, and is kept, while the trip is not started', () async {
      final photo = await proofFile(ProofKind.photo);
      await queueProof('proof-op', photo);
      var uploads = 0;
      final worker = _worker(queue, MockClient((request) async {
        if (request.method == 'GET') return http.Response(jsonEncode({'run': {'status': 'prepared'}}), 200);
        uploads++;
        return http.Response('{}', 201);
      }));
      final waiting = _nextProgress(worker, SyncProgress.waitingForTripStart);
      worker.start();
      await waiting;
      worker.stop();
      expect(uploads, 0);
      expect(File(photo.path).existsSync(), isTrue);
    });

    test('a server 409 such as "run is not in progress" keeps the proof for later', () async {
      final photo = await proofFile(ProofKind.photo);
      await queueProof('proof-op', photo);
      final worker = _worker(queue, MockClient((request) async {
        if (request.method == 'GET') return http.Response(jsonEncode({'run': {'status': 'in_progress'}}), 200);
        return http.Response(jsonEncode({'detail': 'conflict: run is not in progress'}), 409);
      }));
      final waiting = _nextProgress(worker, SyncProgress.waitingForTripStart);
      worker.start();
      await waiting;
      worker.stop();
      expect((await queue.first())!.state, 'pending');
      expect(File(photo.path).existsSync(), isTrue);
    });

    test('a proof the server refuses is blocked with its reason, and later updates wait behind it', () async {
      final photo = await proofFile(ProofKind.photo);
      await queueProof('proof-op', photo);
      // DR-6: a status update queued after a photo that has not been sent yet goes first.
      await queue.enqueue(Operations.incident(operationId: 'earlier-status', trip: _trip, category: 'ROAD', description: 'Road closed', occurredAt: _at).toSyncEvent());
      final synced = <String>[];
      final worker = _worker(queue, MockClient((request) async {
        if (request.method == 'GET') return http.Response(jsonEncode({'run': {'status': 'in_progress'}}), 200);
        if (request.url.path.endsWith('/sync')) {
          final id = ((jsonDecode(request.body) as Map)['operations'] as List).single['operationId'] as String;
          synced.add(id);
          return http.Response(jsonEncode({'results': [{'operationId': id, 'status': 'APPLIED'}]}), 200);
        }
        return http.Response(jsonEncode({'detail': 'invalid: file content does not match PNG or JPEG'}), 400);
      }));
      final attention = _nextProgress(worker, SyncProgress.needsAttention);
      worker.start();
      await attention;
      expect(synced, ['earlier-status'], reason: 'the status update went ahead of the unsent photo');
      // Once the photo is refused it is blocked, and nothing queued after it overtakes it.
      await queue.enqueue(Operations.incident(operationId: 'later', trip: _trip, category: 'ROAD', description: 'Still closed', occurredAt: _at).toSyncEvent());
      await worker.syncNow();
      worker.stop();
      final first = (await queue.next())!;
      expect(first.state, 'blocked');
      expect(first.failure, contains('does not match PNG or JPEG'));
      expect(synced, ['earlier-status'], reason: 'the later incident is not sent past a blocked proof');
      expect(File(photo.path).existsSync(), isTrue);
    });

    test('a missing file is blocked instead of being sent empty', () async {
      final photo = await proofFile(ProofKind.photo);
      await queueProof('proof-op', photo);
      await File(photo.path).delete();
      var uploads = 0;
      final worker = _worker(queue, MockClient((request) async {
        if (request.method == 'GET') return http.Response(jsonEncode({'run': {'status': 'in_progress'}}), 200);
        uploads++;
        return http.Response('{}', 201);
      }));
      final attention = _nextProgress(worker, SyncProgress.needsAttention);
      worker.start();
      await attention;
      worker.stop();
      expect(uploads, 0);
      expect((await queue.first())!.failure, contains('missing from this phone'));
    });

    test('a rejected sign-in waits for sign-in without losing the proof', () async {
      final photo = await proofFile(ProofKind.photo);
      await queueProof('proof-op', photo);
      final worker = _worker(queue, MockClient((request) async {
        if (request.method == 'GET') return http.Response(jsonEncode({'run': {'status': 'in_progress'}}), 200);
        return http.Response('', 401);
      }));
      final waiting = _nextProgress(worker, SyncProgress.waitingForSignIn);
      worker.start();
      await waiting;
      worker.stop();
      expect((await queue.first())!.state, 'pending');
      expect(File(photo.path).existsSync(), isTrue);
    });
  });
}

bool _contains(List<int> haystack, List<int> needle) {
  for (var i = 0; i + needle.length <= haystack.length; i++) {
    var match = true;
    for (var j = 0; j < needle.length; j++) {
      if (haystack[i + j] != needle[j]) {
        match = false;
        break;
      }
    }
    if (match) return true;
  }
  return false;
}

MockClient _ok(void Function(http.Request) onProof) => MockClient((request) async {
      if (request.method == 'GET') return http.Response(jsonEncode({'run': {'status': 'in_progress'}}), 200);
      onProof(request);
      return http.Response('{}', 201);
    });

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
