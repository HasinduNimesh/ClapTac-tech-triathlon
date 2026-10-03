import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:http/http.dart' as http;
import 'package:http_parser/http_parser.dart';

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
  Timer? _timer;
  bool _running = false;
  bool _enabled = false;

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

  Future<void> syncNow() async {
    if (!_enabled || _running) return;
    _running = true;
    try {
      while (_enabled) {
        final item = await _queue.first();
        if (item == null) {
          onProgress?.call(SyncProgress.idle, null);
          return;
        }
        if (item.state == 'blocked') {
          onProgress?.call(SyncProgress.needsAttention, item.failure);
          return;
        }
        final token = await _auth.accessToken();
        if (token == null) {
          onProgress?.call(SyncProgress.waitingForSignIn, null);
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
          onProgress?.call(SyncProgress.waitingForProof, null);
          return;
        }
        if (type == OperationType.arrived ||
            type == OperationType.stopOutcome ||
            type == OperationType.routeCompleted ||
            type == OperationType.proofUpload) {
          final tripId = operation['tripId'] as String?;
          if (tripId == null || tripId.isEmpty) {
            await _queue.markBlocked(item.event.idempotencyKey, 'Missing server trip ID');
            onProgress?.call(SyncProgress.needsAttention, 'Missing server trip ID');
            return;
          }
          final status = await _tripStatus(tripId, token);
          if (status == null) {
            onProgress?.call(SyncProgress.offline, null);
            return;
          }
          if (status == 'unauthorized') {
            onProgress?.call(SyncProgress.waitingForSignIn, null);
            return;
          }
          if (status != 'in_progress') {
            onProgress?.call(SyncProgress.waitingForTripStart, null);
            return;
          }
        }
        if (type == OperationType.proofUpload) {
          onProgress?.call(SyncProgress.syncing, null);
          if (await _uploadProof(item, operation, payload, token)) continue;
          return;
        }
        onProgress?.call(SyncProgress.syncing, null);
        final response = await _client.post(
          Uri.parse('$_baseUrl/api/v1/delivery/sync'),
          headers: {'Authorization': 'Bearer $token', 'Content-Type': 'application/json', 'Accept': 'application/json'},
          body: jsonEncode({'operations': [operation]}),
        ).timeout(timeout);
        if (response.statusCode == 401) {
          onProgress?.call(SyncProgress.waitingForSignIn, null);
          return;
        }
        if (response.statusCode != 200) {
          onProgress?.call(response.statusCode >= 500 ? SyncProgress.offline : SyncProgress.needsAttention,
              'Waypoint returned HTTP ${response.statusCode}; the update remains saved on this phone');
          return;
        }
        final body = jsonDecode(response.body);
        final results = body is Map ? body['results'] : null;
        if (results is! List || results.length != 1 || results.first is! Map) {
          onProgress?.call(SyncProgress.offline, 'Waypoint returned an unreadable sync result');
          return;
        }
        final result = (results.first as Map).cast<String, Object?>();
        if (result['operationId'] != item.event.idempotencyKey) {
          onProgress?.call(SyncProgress.offline, 'Waypoint returned a different operation ID');
          return;
        }
        final status = result['status'];
        if (status == 'APPLIED' || (status == 'DUPLICATE' && result['originalStatus'] == 'APPLIED')) {
          await _queue.markSynced(item.event.idempotencyKey);
          continue;
        }
        final detail = result['detail']?.toString() ?? 'Waypoint rejected this update';
        await _queue.markBlocked(item.event.idempotencyKey, '$status: $detail');
        onProgress?.call(SyncProgress.needsAttention, '$status: $detail');
        return;
      }
    } on SocketException {
      onProgress?.call(SyncProgress.offline, null);
    } on TimeoutException {
      onProgress?.call(SyncProgress.offline, null);
    } on http.ClientException {
      onProgress?.call(SyncProgress.offline, null);
    } on FormatException {
      onProgress?.call(SyncProgress.offline, 'Waypoint returned unreadable data');
    } on Object catch (error) {
      onProgress?.call(SyncProgress.needsAttention, 'Sync stopped: $error');
    } finally {
      _running = false;
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
      onProgress?.call(SyncProgress.needsAttention, detail);
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
      onProgress?.call(SyncProgress.waitingForSignIn, null);
    } else if (code >= 500 || code == 408 || code == 429) {
      onProgress?.call(SyncProgress.offline, 'Waypoint returned HTTP $code; the photo or signature remains saved on this phone');
    } else if (code == 409) {
      // For example "run is not in progress": it can succeed once the trip starts, so keep it.
      onProgress?.call(SyncProgress.waitingForTripStart, _problemDetail(response) ?? 'The proof is waiting for the trip to start');
    } else {
      final detail = _problemDetail(response) ?? 'Waypoint rejected this photo or signature (HTTP $code)';
      await _queue.markBlocked(item.event.idempotencyKey, 'HTTP $code: $detail');
      onProgress?.call(SyncProgress.needsAttention, 'HTTP $code: $detail');
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
