import 'dart:async';
import 'dart:convert';

import 'package:flutter/foundation.dart';

import '../api/api_client.dart';
import '../offline/store.dart';

/// One queued write. Each has a client-generated idempotency key, so a retry
/// never creates a duplicate outcome, proof or loading record.
class QueuedOperation {
  QueuedOperation({required this.operationId, required this.type, this.tripId, this.stopId, this.orderId, this.dependsOn, this.payload = const {}, this.fileBase64, String? createdAt})
      : createdAt = createdAt ?? DateTime.now().toUtc().toIso8601String();

  final String operationId;
  /// Driver: START, ARRIVED, PROOF_UPLOAD, STOP_OUTCOME, INCIDENT_REPORT, ROUTE_COMPLETED, PLAN_ACK.
  /// Loader: LOAD_START, ORDER_LOADED, LOAD_ISSUE, LOAD_READY, PLAN_ACK.
  final String type;
  final String? tripId;
  final String? stopId;
  final String? orderId;
  final String? dependsOn;
  final Map<String, dynamic> payload;
  final String? fileBase64;
  final String createdAt;

  Map<String, dynamic> toJson() => {'operationId': operationId, 'type': type, 'tripId': tripId, 'stopId': stopId, 'orderId': orderId, 'dependsOn': dependsOn, 'payload': payload, 'fileBase64': fileBase64, 'createdAt': createdAt};
  factory QueuedOperation.fromJson(Map<String, dynamic> j) => QueuedOperation(
        operationId: j['operationId'] as String,
        type: j['type'] as String,
        tripId: j['tripId'] as String?,
        stopId: j['stopId'] as String?,
        orderId: j['orderId'] as String?,
        dependsOn: j['dependsOn'] as String?,
        payload: (j['payload'] as Map<String, dynamic>?) ?? const {},
        fileBase64: j['fileBase64'] as String?,
        createdAt: j['createdAt'] as String?,
      );
}

enum SyncPhase { idle, offline, syncing, synced, error, paused }

class SyncState {
  const SyncState(this.phase, {this.pending = 0, this.pendingProofs = 0, this.detail = '', this.lastSyncedAt});
  final SyncPhase phase;
  final int pending;
  final int pendingProofs;
  final String detail;
  final DateTime? lastSyncedAt;
}

/// FIFO queue persisted on the device. Draining stops at the first failure so
/// dependent work (proof → outcome → route completion) stays in order.
class SyncEngine extends ChangeNotifier {
  SyncEngine({required this.store, required this.api, required this.ownerId});

  final LocalStore store;
  final ApiClient api;
  final String ownerId;
  SyncState state = const SyncState(SyncPhase.idle);
  bool _draining = false;

  String get _key => 'queue.$ownerId';

  Future<List<QueuedOperation>> pending() async {
    final raw = await store.read(_key);
    if (raw == null) return [];
    return (jsonDecode(raw) as List).whereType<Map<String, dynamic>>().map(QueuedOperation.fromJson).toList();
  }

  Future<void> _save(List<QueuedOperation> items) => store.write(_key, jsonEncode(items.map((e) => e.toJson()).toList()));

  Future<void> enqueue(QueuedOperation op) async {
    final items = await pending();
    if (items.any((i) => i.operationId == op.operationId)) return;
    items.add(op);
    await _save(items);
    await _refresh(SyncPhase.idle);
  }

  Future<void> _refresh(SyncPhase phase, {String detail = ''}) async {
    final items = await pending();
    state = SyncState(phase, pending: items.length, pendingProofs: items.where((i) => i.type == 'PROOF_UPLOAD').length, detail: detail, lastSyncedAt: phase == SyncPhase.synced ? DateTime.now() : state.lastSyncedAt);
    notifyListeners();
  }

  Future<SyncState> drain() async {
    if (_draining) return state;
    _draining = true;
    try {
      var items = await pending();
      if (items.isEmpty) {
        await _refresh(SyncPhase.synced);
        return state;
      }
      await _refresh(SyncPhase.syncing);
      while (items.isNotEmpty) {
        final item = items.first;
        try {
          final applied = await _apply(item);
          if (!applied) {
            await _refresh(SyncPhase.error, detail: '${item.type} was not accepted by the server. It stays saved on this device.');
            return state;
          }
          items = (await pending()).where((i) => i.operationId != item.operationId).toList();
          await _save(items);
          await _refresh(SyncPhase.syncing);
        } on OfflineException {
          await _refresh(SyncPhase.offline);
          return state;
        } on ApiException catch (e) {
          if (e.status == 401) {
            await _refresh(SyncPhase.paused, detail: 'Sign in again to send saved work. Nothing was deleted.');
          } else if (e.status == 409 || e.status == 412) {
            await _refresh(SyncPhase.error, detail: 'Plan conflict: the dispatcher published a newer plan. Your saved record is kept.');
          } else {
            await _refresh(SyncPhase.error, detail: '${item.type == 'PROOF_UPLOAD' ? 'Photo upload failed' : 'Upload failed'} (${e.status}). It stays saved on this device.');
          }
          return state;
        }
      }
      await _refresh(SyncPhase.synced);
      return state;
    } finally {
      _draining = false;
    }
  }

  Future<bool> _apply(QueuedOperation item) async {
    final key = {'Idempotency-Key': item.operationId};
    switch (item.type) {
      case 'START':
        await api.post('/delivery/trips/${item.tripId}/start', headers: key, body: {'operationId': item.operationId});
        return true;
      case 'PROOF_UPLOAD':
        await api.multipart('/delivery/trips/${item.tripId}/stops/${item.stopId}/proofs',
            headers: key,
            fields: {'type': '${item.payload['proofType'] ?? 'PHOTO'}', if (item.payload['mimeType'] != null) 'mimeType': '${item.payload['mimeType']}', if (item.payload['receiverName'] != null) 'receiverName': '${item.payload['receiverName']}'},
            bytes: base64Decode(item.fileBase64 ?? ''),
            filename: item.payload['proofType'] == 'SIGNATURE' ? 'signature.png' : 'photo.jpg');
        return true;
      case 'ARRIVED':
      case 'STOP_OUTCOME':
      case 'INCIDENT_REPORT':
      case 'ROUTE_COMPLETED':
        final res = await api.post('/delivery/sync', body: {
          'operations': [
            {'operationId': item.operationId, 'type': item.type, 'tripId': item.tripId, 'stopId': item.stopId, 'occurredAt': item.payload['occurredAt'] ?? item.createdAt, 'dependsOnOperationId': item.dependsOn, 'payload': item.payload}
          ]
        });
        final results = (res is Map<String, dynamic> ? res['results'] as List? : null) ?? const [];
        final result = results.whereType<Map<String, dynamic>>().firstWhere((r) => r['operationId'] == item.operationId, orElse: () => const {});
        return result['status'] == 'APPLIED' || (result['status'] == 'DUPLICATE' && result['originalStatus'] == 'APPLIED');
      case 'LOAD_START':
        await api.post('/loading/trips/${item.tripId}/start', headers: {...key, if (item.payload['planVersion'] != null) 'If-Match': '${item.payload['planVersion']}'});
        return true;
      case 'ORDER_LOADED':
        await api.put('/loading/trips/${item.tripId}/orders/${item.orderId}/loaded', headers: key);
        return true;
      case 'LOAD_ISSUE':
        await api.post('/loading/trips/${item.tripId}/orders/${item.orderId}/issues', headers: key, body: item.payload);
        return true;
      case 'LOAD_READY':
        await api.post('/loading/trips/${item.tripId}/ready', headers: key);
        return true;
      case 'PLAN_ACK':
        await api.post('/planning/plans/${item.payload['planId']}/acknowledgements', body: {'version': item.payload['version']});
        return true;
    }
    return false;
  }
}
