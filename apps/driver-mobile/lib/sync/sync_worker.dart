import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:http/http.dart' as http;
import 'package:http_parser/http_parser.dart';

import '../auth/auth_failure.dart';
import '../auth/auth_gateway.dart';
import 'operations.dart';
import 'sqlite_sync_queue.dart';

enum SyncProgress { idle, syncing, waitingForSignIn, waitingForTripStart, waitingForProof, offline, needsAttention }

/// Sends one durable operation at a time. A crash after server acceptance is
/// safe: the same operation ID is retried and an APPLIED duplicate is accepted.
class DeliverySyncWorker {
  DeliverySyncWorker({
    required SqliteSyncQueue queue,
    required AuthGateway auth,
    required http.Client client,
    required String baseUrl,
    this.onProgress,
    this.interval = const Duration(seconds: 15),
    this.timeout = const Duration(seconds: 20),
  })  : _queue = queue,
        _auth = auth,
        _client = client,
        _baseUrl = baseUrl.replaceAll(RegExp(r'/+$'), '');

  final SqliteSyncQueue _queue;
  final AuthGateway _auth;
  final http.Client _client;
  final String _baseUrl;
  void Function(SyncProgress progress, String? detail)? onProgress;
  final Duration interval;
  final Duration timeout;
  SyncProgress? _lastProgress;
  String? _lastDetail;
  Timer? _timer;
  Future<void>? _current;
  bool _again = false;
  bool _enabled = false;

  void _report(SyncProgress progress, String? detail) {
    _lastProgress = progress;
    _lastDetail = detail;
    onProgress?.call(progress, detail);
  }

  void start() {
    if (_enabled) return;
    _enabled = true;
    _timer = Timer.periodic(interval, (_) => unawaited(syncNow()));
    unawaited(syncNow());
  }

  void stop() {
    _enabled = false;
    _timer?.cancel();
    _timer = null;
  }

  /// Sends what is waiting. When a pass is already running this does not start a second one; it
  /// asks for one more pass afterwards and completes when that is done, so a caller that needs
  /// everything sent (finishing the trip) never returns while updates queued meanwhile are waiting.
  Future<void> syncNow() {
    if (!_enabled) return Future.value();
    final running = _current;
    if (running != null) {
      _again = true;
      return running;
    }
    return _current = _passes().whenComplete(() => _current = null);
  }

  Future<void> _passes() async {
    do {
      _again = false;
      await _pass();
    } while (_again && _enabled);
  }

  Future<void> _pass() async {
    // Trips whose updates cannot go right now. Updates stay in order within a trip, but a trip that is stuck
    // (not started, already finished, blocked, waiting for a proof) must not hold up the others: an old trip
    // left on the phone used to freeze today's delivery for good.
    final held = <String, ({SyncProgress progress, String? detail})>{};
    try {
      while (_enabled) {
        final item = await _queue.next(skipTrips: held.keys.toSet());
        if (item == null) {
          _reportHeld(held);
          return;
        }
        final trip = SqliteSyncQueue.tripOf(item);
        if (item.state == 'blocked') {
          held[trip] = (progress: SyncProgress.needsAttention, detail: item.failure);
          continue;
        }
        final token = await _auth.accessToken();
        if (token == null) {
          _report(SyncProgress.waitingForSignIn, null);
          return;
        }
        final operation = item.event.payload;
        final type = operation['type'] as String?;
        final fields = operation['payload'];
        final payload = fields is Map ? fields.cast<String, Object?>() : const <String, Object?>{};
        final code = payload['code'] as String?;
        if (type == OperationType.stopOutcome &&
            (code == 'DELIVERED' || code == 'PARTIAL') &&
            (operation['dependsOnOperationId'] as String?)?.isNotEmpty != true) {
          // The server permanently rejects a successful outcome without a
          // finalized proof. Keep its ID unused until proof capture exists.
          held[trip] = (progress: SyncProgress.waitingForProof, detail: null);
          continue;
        }
        if (type == OperationType.arrived ||
            type == OperationType.stopOutcome ||
            type == OperationType.routeCompleted ||
            type == OperationType.proofUpload) {
          final tripId = operation['tripId'] as String?;
          if (tripId == null || tripId.isEmpty) {
            await _queue.markBlocked(item.event.idempotencyKey, 'Missing server trip ID');
            held[trip] = (progress: SyncProgress.needsAttention, detail: 'Missing server trip ID');
            continue;
          }
          final status = await _tripStatus(tripId, token);
          if (status == null) {
            _report(SyncProgress.offline, null);
            return;
          }
          if (status == 'unauthorized') {
            _report(SyncProgress.waitingForSignIn, null);
            return;
          }
          if (status == 'completed') {
            // The server will never take updates for a finished trip, so this is not "waiting for the trip to start".
            held[trip] = (progress: SyncProgress.needsAttention, detail: 'A trip on this phone is already finished on Waypoint, so its saved updates cannot be sent. Ask your dispatcher.');
            continue;
          }
          if (status != 'in_progress') {
            held[trip] = (progress: SyncProgress.waitingForTripStart, detail: null);
            continue;
          }
        }
        if (type == OperationType.proofUpload) {
          _report(SyncProgress.syncing, null);
          if (await _uploadProof(item, operation, payload, token)) continue;
          // It has reported why it stopped. Only a problem with this trip sets the trip aside; no connection or
          // no sign-in stops the whole pass as before.
          final stopped = _lastProgress;
          if (stopped == SyncProgress.waitingForTripStart || stopped == SyncProgress.needsAttention || stopped == SyncProgress.waitingForProof) {
            held[trip] = (progress: stopped!, detail: _lastDetail);
            continue;
          }
          return;
        }
        _report(SyncProgress.syncing, null);
        final response = await _client.post(
          Uri.parse('$_baseUrl/api/v1/delivery/sync'),
          headers: {'Authorization': 'Bearer $token', 'Content-Type': 'application/json', 'Accept': 'application/json'},
          body: jsonEncode({'operations': [operation]}),
        ).timeout(timeout);
        if (response.statusCode == 401) {
          _report(SyncProgress.waitingForSignIn, null);
          return;
        }
        if (response.statusCode != 200) {
          _report(response.statusCode >= 500 ? SyncProgress.offline : SyncProgress.needsAttention,
              'Waypoint returned HTTP ${response.statusCode}; the update remains saved on this phone');
          return;
        }
        final body = jsonDecode(response.body);
        final results = body is Map ? body['results'] : null;
        if (results is! List || results.length != 1 || results.first is! Map) {
          _report(SyncProgress.offline, 'Waypoint returned an unreadable sync result');
          return;
        }
        final result = (results.first as Map).cast<String, Object?>();
        if (result['operationId'] != item.event.idempotencyKey) {
          _report(SyncProgress.offline, 'Waypoint returned a different operation ID');
          return;
        }
        final status = result['status'];
        if (status == 'APPLIED' || (status == 'DUPLICATE' && result['originalStatus'] == 'APPLIED')) {
          await _queue.markSynced(item.event.idempotencyKey);
          continue;
        }
        final detail = result['detail']?.toString() ?? 'Waypoint rejected this update';
        await _queue.markBlocked(item.event.idempotencyKey, '$status: $detail');
        held[trip] = (progress: SyncProgress.needsAttention, detail: '$status: $detail');
        continue;
      }
    } on AuthFailure {
      // Refreshing the access token needed a connection that is not there; the queue is untouched.
      _report(SyncProgress.offline, null);
    } on SocketException {
      _report(SyncProgress.offline, null);
    } on TimeoutException {
      _report(SyncProgress.offline, null);
    } on http.ClientException {
      _report(SyncProgress.offline, null);
    } on FormatException {
      _report(SyncProgress.offline, 'Waypoint returned unreadable data');
    } on Object catch (error) {
      _report(SyncProgress.needsAttention, 'Sync stopped: $error');
    }
  }

  /// Reports the most important reason a trip was set aside, or that everything is sent.
  void _reportHeld(Map<String, ({SyncProgress progress, String? detail})> held) {
    if (held.isEmpty) {
      _report(SyncProgress.idle, null);
      return;
    }
    const order = [SyncProgress.needsAttention, SyncProgress.waitingForProof, SyncProgress.waitingForTripStart];
    for (final progress in order) {
      for (final entry in held.values) {
        if (entry.progress == progress) {
          _report(entry.progress, entry.detail);
          return;
        }
      }
    }
  }

  /// Uploads one captured proof through the multipart endpoint, which is not part of `/sync`.
  /// Returns true when it is accepted (the queue moves on); otherwise it has reported why it
  /// stopped. The operation id is the Idempotency-Key, so a retry after a lost response is safe.
  Future<bool> _uploadProof(QueuedOperation item, Map<String, Object?> operation, Map<String, Object?> payload, String token) async {
    final tripId = operation['tripId'] as String? ?? '';
    final stopId = operation['stopId'] as String? ?? '';
    final path = payload['filePath'] as String? ?? '';
    final mimeType = payload['mimeType'] as String? ?? '';
    final file = File(path);
    if (stopId.isEmpty || path.isEmpty || !file.existsSync()) {
      const detail = 'The photo or signature file is missing from this phone';
      await _queue.markBlocked(item.event.idempotencyKey, detail);
      _report(SyncProgress.needsAttention, detail);
      return false;
    }
    final request = http.MultipartRequest(
      'POST',
      Uri.parse('$_baseUrl/api/v1/delivery/trips/${Uri.encodeComponent(tripId)}/stops/${Uri.encodeComponent(stopId)}/proofs'),
    )
      ..headers.addAll({'Authorization': 'Bearer $token', 'Accept': 'application/json', 'Idempotency-Key': item.event.idempotencyKey})
      ..fields['type'] = payload['proofType'] as String? ?? ''
      ..fields['capturedAt'] = payload['capturedAt'] as String? ?? ''
      ..files.add(http.MultipartFile.fromBytes('file', await file.readAsBytes(), filename: path.split('/').last, contentType: MediaType.parse(mimeType)));
    final receiver = payload['receiverName'] as String? ?? '';
    if (receiver.isNotEmpty) request.fields['receiverName'] = receiver;
    final response = await http.Response.fromStream(await _client.send(request).timeout(timeout));
    final code = response.statusCode;
    if (code == 200 || code == 201) {
      await _queue.markSynced(item.event.idempotencyKey);
      // The server holds the proof now; the local copy is no longer needed.
      try {
        await file.delete();
      } on FileSystemException {
        // Nothing depends on the file any more.
      }
      return true;
    }
    if (code == 401) {
      _report(SyncProgress.waitingForSignIn, null);
    } else if (code >= 500 || code == 408 || code == 429) {
      _report(SyncProgress.offline, 'Waypoint returned HTTP $code; the photo or signature remains saved on this phone');
    } else if (code == 409) {
      // For example "run is not in progress": it can succeed once the trip starts, so keep it.
      _report(SyncProgress.waitingForTripStart, _problemDetail(response) ?? 'The proof is waiting for the trip to start');
    } else {
      final detail = _problemDetail(response) ?? 'Waypoint rejected this photo or signature (HTTP $code)';
      await _queue.markBlocked(item.event.idempotencyKey, 'HTTP $code: $detail');
      _report(SyncProgress.needsAttention, 'HTTP $code: $detail');
    }
    return false;
  }

  String? _problemDetail(http.Response response) {
    try {
      final body = jsonDecode(response.body);
      if (body is Map && body['detail'] is String) return body['detail'] as String;
    } on FormatException {
      // No readable detail.
    }
    return null;
  }

  Future<String?> _tripStatus(String tripId, String token) async {
    final response = await _client.get(
      Uri.parse('$_baseUrl/api/v1/delivery/trips/${Uri.encodeComponent(tripId)}'),
      headers: {'Authorization': 'Bearer $token', 'Accept': 'application/json'},
    ).timeout(timeout);
    if (response.statusCode == 401) return 'unauthorized';
    if (response.statusCode != 200) return null;
    final body = jsonDecode(response.body);
    if (body is! Map) return null;
    final run = body['run'];
    return run is Map ? run['status'] as String? : null;
  }
}
