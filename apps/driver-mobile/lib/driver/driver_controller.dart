import 'dart:convert';

import 'package:flutter/foundation.dart';

import '../api/api_client.dart';
import '../offline/store.dart';
import '../shared/models.dart';
import '../sync/sync.dart';
import '../widgets/common.dart';

/// Driver route state. Every action is saved on the phone first and queued;
/// the queue sends it when there is signal ("Saved on this device" → "Synced").
class DriverController extends ChangeNotifier {
  DriverController({required this.api, required this.store, required this.sync, required this.ownerId});

  final ApiClient api;
  final LocalStore store;
  final SyncEngine sync;
  final String ownerId;

  DeliveryTrip? trip;
  List<TripMessage> messages = [];
  bool loading = true;
  bool offline = false;
  bool fromCache = false;
  String error = '';
  bool checkedOut = false;
  final Map<String, String> proofIds = {};
  final Map<String, bool> photoSaved = {};

  String get _tripKey => 'driver.trip.$ownerId.${todayIso()}';

  Future<void> load() async {
    loading = true;
    error = '';
    notifyListeners();
    try {
      final body = await api.get('/delivery/trips?date=${todayIso()}');
      final items = ((body as Map<String, dynamic>)['items'] as List? ?? const []).whereType<Map<String, dynamic>>().toList();
      if (items.isEmpty) {
        trip = null;
      } else {
        final active = items.firstWhere((t) => !RegExp('complete', caseSensitive: false).hasMatch('${t['status']}'), orElse: () => items.first);
        final detail = await api.get('/delivery/trips/${active['tripId']}');
        trip = await _overlayQueued(DeliveryTrip(detail as Map<String, dynamic>));
        await store.writeJson(_tripKey, trip!.raw);
        try {
          final m = await api.get('/delivery/trips/${trip!.tripId}/messages');
          messages = ((m as Map<String, dynamic>)['items'] as List? ?? const []).whereType<Map<String, dynamic>>().map(TripMessage.new).toList();
        } catch (_) {/* messages are supplementary */}
      }
      offline = false;
      fromCache = false;
      checkedOut = trip?.started ?? false;
      await sync.drain();
    } on OfflineException {
      offline = true;
      final cached = await store.readJson(_tripKey);
      trip = cached == null ? null : DeliveryTrip(cached);
      fromCache = cached != null;
      checkedOut = trip?.started ?? checkedOut;
    } on ApiException catch (e) {
      error = e.status == 401 ? 'Your sign-in has expired. Sign in again; saved work stays on this phone.' : 'The route could not be loaded (${e.status}).';
      final cached = await store.readJson(_tripKey);
      if (cached != null) {
        trip = DeliveryTrip(cached);
        fromCache = true;
      }
    } finally {
      loading = false;
      notifyListeners();
    }
  }

  /// Re-applies outcomes still waiting in the queue, so a refreshed route never
  /// shows a delivered stop as pending before the queue has synced.
  Future<DeliveryTrip> _overlayQueued(DeliveryTrip t) async {
    var result = t;
    for (final op in await sync.pending()) {
      if (op.tripId != t.tripId || op.stopId == null) continue;
      final stop = result.stops.where((s) => s.id == op.stopId).firstOrNull;
      if (stop == null) continue;
      if (op.type == 'STOP_OUTCOME') result = result.withStop(stop.copyWith({'status': 'completed', 'outcomeCode': op.payload['code'], 'outcomeReason': op.payload['reason']}));
      if (op.type == 'ARRIVED') result = result.withStop(stop.copyWith({'status': 'arrived', 'arrivedAt': op.payload['occurredAt']}));
    }
    return result;
  }

  Future<void> _persist() async {
    if (trip != null) await store.writeJson(_tripKey, trip!.raw);
    notifyListeners();
  }

  Future<void> _queue(QueuedOperation op) async {
    await sync.enqueue(op);
    await _persist();
    await sync.drain();
    offline = sync.state.phase == SyncPhase.offline;
    notifyListeners();
  }

  /// Truck check-out: the driver confirms the load before leaving the depot.
  Future<void> confirmLoad() async {
    final t = trip;
    if (t == null) return;
    checkedOut = true;
    if (!t.started) {
      trip = t.withStatus('in_progress');
      await _queue(QueuedOperation(operationId: newOperationId(), type: 'START', tripId: t.tripId));
    } else {
      notifyListeners();
    }
  }

  Future<void> arrive(DeliveryStop stop) async {
    final t = trip;
    if (t == null || stop.arrivedAt != null) return;
    final now = DateTime.now().toUtc().toIso8601String();
    trip = t.withStop(stop.copyWith({'status': 'arrived', 'arrivedAt': now}));
    await _queue(QueuedOperation(operationId: newOperationId(), type: 'ARRIVED', tripId: t.tripId, stopId: stop.id, payload: {'occurredAt': now}));
  }

  Future<void> addProof(DeliveryStop stop, {required List<int> bytes, required String proofType, String mimeType = 'image/jpeg', String? receiverName}) async {
    final t = trip;
    if (t == null) return;
    final id = newOperationId();
    proofIds[stop.id] = id;
    if (proofType == 'PHOTO') photoSaved[stop.id] = true;
    await _queue(QueuedOperation(operationId: id, type: 'PROOF_UPLOAD', tripId: t.tripId, stopId: stop.id, fileBase64: base64Encode(bytes), payload: {'proofType': proofType, 'mimeType': mimeType, if (receiverName != null && receiverName.isNotEmpty) 'receiverName': receiverName}));
  }

  /// Codes match the delivery service: DELIVERED, PARTIAL, REFUSED, NOT_DELIVERED, FAILED.
  Future<void> recordOutcome(DeliveryStop stop, {required String code, String reason = '', String note = '', int? receivedUnits}) async {
    final t = trip;
    if (t == null) return;
    final now = DateTime.now().toUtc().toIso8601String();
    trip = t.withStop(stop.copyWith({'status': 'completed', 'outcomeCode': code, 'outcomeReason': reason}));
    await _queue(QueuedOperation(
      operationId: newOperationId(),
      type: 'STOP_OUTCOME',
      tripId: t.tripId,
      stopId: stop.id,
      dependsOn: proofIds[stop.id],
      payload: {'code': code, 'reason': reason, 'note': [note, if (receivedUnits != null) 'Received $receivedUnits units'].where((s) => s.isNotEmpty).join(' · '), 'occurredAt': now},
    ));
  }

  Future<void> reportProblem({required String category, required String description, DeliveryStop? stop}) async {
    final t = trip;
    if (t == null) return;
    await _queue(QueuedOperation(operationId: newOperationId(), type: 'INCIDENT_REPORT', tripId: t.tripId, stopId: stop?.id, payload: {'category': category, 'description': description, 'occurredAt': DateTime.now().toUtc().toIso8601String()}));
  }

  Future<void> acknowledgePlan() async {
    final t = trip;
    if (t == null || t.planId.isEmpty) return;
    await _queue(QueuedOperation(operationId: newOperationId(), type: 'PLAN_ACK', tripId: t.tripId, payload: {'planId': t.planId, 'version': t.planVersion}));
    trip = DeliveryTrip({...t.raw, 'run': {...t.run, 'acknowledgedVersion': t.planVersion}});
    await _persist();
  }

  Future<void> acknowledgeMessage(TripMessage message) async {
    final t = trip;
    if (t == null) return;
    try {
      await api.post('/delivery/trips/${t.tripId}/messages/${message.id}/ack');
      messages = messages.map((m) => m.id == message.id ? TripMessage({...m.raw, 'acknowledgedAt': DateTime.now().toUtc().toIso8601String()}) : m).toList();
      notifyListeners();
    } on OfflineException {
      error = 'No signal. Acknowledge the message when you are back online.';
      notifyListeners();
    }
  }

  Future<bool> finishTrip() async {
    final t = trip;
    if (t == null) return false;
    if (t.stops.any((s) => !s.done)) return false;
    trip = t.withStatus('completed');
    await _queue(QueuedOperation(operationId: newOperationId(), type: 'ROUTE_COMPLETED', tripId: t.tripId));
    return true;
  }

  Future<void> retrySync() async {
    await sync.drain();
    offline = sync.state.phase == SyncPhase.offline;
    notifyListeners();
  }
}
