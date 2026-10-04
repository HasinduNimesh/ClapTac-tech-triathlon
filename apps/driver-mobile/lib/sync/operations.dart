import 'dart:math';

import '../data/driver_models.dart';
import 'sync.dart';

/// A random version-4 UUID. It is created once when the driver does something and kept
/// through retries and restarts, so the server can recognise a replay as a duplicate.
String newOperationId([Random? random]) {
  final source = random ?? Random.secure();
  final bytes = List<int>.generate(16, (_) => source.nextInt(256));
  bytes[6] = (bytes[6] & 0x0f) | 0x40;
  bytes[8] = (bytes[8] & 0x3f) | 0x80;
  final hex = bytes.map((byte) => byte.toRadixString(16).padLeft(2, '0')).join();
  return '${hex.substring(0, 8)}-${hex.substring(8, 12)}-${hex.substring(12, 16)}-${hex.substring(16, 20)}-${hex.substring(20)}';
}

/// Operation types accepted by `POST /api/v1/delivery/sync`.
class OperationType {
  static const arrived = 'ARRIVED';
  static const stopOutcome = 'STOP_OUTCOME';
  static const routeCompleted = 'ROUTE_COMPLETED';
  static const incidentReport = 'INCIDENT_REPORT';

  /// Not part of the sync contract: a proof is a separate multipart upload
  /// (`POST /api/v1/delivery/trips/{id}/stops/{id}/proofs`). It sits in the local queue between the
  /// arrival and the outcome so the order is kept, and the sync worker sends it on its own.
  static const proofUpload = 'PROOF_UPLOAD';
}

/// One entry of the delivery sync contract (contracts/openapi/delivery.yaml).
///
/// Proof bytes never travel in this JSON: they are a separate multipart upload, and a
/// `STOP_OUTCOME` refers to it through [dependsOnOperationId].
class OfflineOperation {
  const OfflineOperation({
    required this.operationId,
    required this.type,
    required this.tripId,
    required this.occurredAt,
    this.runId,
    this.stopId,
    this.dependsOnOperationId,
    this.payload = const {},
  });

  final String operationId;
  final String type;
  final String tripId;
  final String? runId;

  /// Optional for incidents: not every incident happens at a stop.
  final String? stopId;
  final DateTime occurredAt;
  final String? dependsOnOperationId;
  final Map<String, Object?> payload;

  /// The object sent inside `operations` of the sync request.
  Map<String, Object?> toSyncJson() => {
        'operationId': operationId,
        'type': type,
        'tripId': tripId,
        if (runId != null && runId!.isNotEmpty) 'runId': runId,
        if (stopId != null && stopId!.isNotEmpty) 'stopId': stopId,
        'occurredAt': occurredAt.toUtc().toIso8601String(),
        if (dependsOnOperationId != null) 'dependsOnOperationId': dependsOnOperationId,
        'payload': payload,
      };

  /// How the operation sits in the local queue. The full sync object is kept as the payload so a
  /// sync worker can send it unchanged.
  SyncEvent toSyncEvent() => SyncEvent(
        eventId: operationId,
        idempotencyKey: operationId,
        action: type,
        resourceType: (stopId ?? '').isEmpty ? 'trip' : 'stop',
        resourceId: (stopId ?? '').isEmpty ? tripId : stopId!,
        payload: toSyncJson(),
        occurredAt: occurredAt,
      );
}

/// The code the delivery API uses for an outcome.
String outcomeCode(DeliveryOutcome outcome) {
  switch (outcome) {
    case DeliveryOutcome.delivered:
      return 'DELIVERED';
    case DeliveryOutcome.partial:
      return 'PARTIAL';
    case DeliveryOutcome.failed:
      return 'FAILED';
    case DeliveryOutcome.refused:
      return 'REFUSED';
  }
}

/// The API requires a reason code for a failed or refused delivery. When the driver did not pick
/// one, a rejection means the goods were refused and anything else is recorded as OTHER, with the
/// driver's own words in the note.
String? reasonFor(DeliveryDraft draft) {
  switch (draft.outcome) {
    case DeliveryOutcome.delivered:
    case DeliveryOutcome.partial:
      return null;
    case DeliveryOutcome.refused:
      return draft.reason ?? 'GOODS_REJECTED';
    case DeliveryOutcome.failed:
      return draft.reason ?? 'OTHER';
  }
}

class Operations {
  /// A photo or signature waiting to be uploaded. The bytes stay in the file at [CapturedProof.path].
  static OfflineOperation proofUpload({
    required String operationId,
    required TripInfo trip,
    required StopInfo stop,
    required CapturedProof proof,
    required DateTime occurredAt,
  }) =>
      OfflineOperation(
        operationId: operationId,
        type: OperationType.proofUpload,
        tripId: trip.tripId,
        runId: trip.runId,
        stopId: stop.stopId,
        occurredAt: occurredAt,
        payload: {
          'proofType': proof.kind == ProofKind.photo ? 'PHOTO' : 'SIGNATURE',
          'filePath': proof.path,
          'mimeType': proof.mimeType,
          'capturedAt': proof.capturedAt.toUtc().toIso8601String(),
          if (proof.receiverName.isNotEmpty) 'receiverName': proof.receiverName,
        },
      );

  static OfflineOperation arrived({
    required String operationId,
    required TripInfo trip,
    required StopInfo stop,
    required DateTime occurredAt,
  }) =>
      OfflineOperation(
        operationId: operationId,
        type: OperationType.arrived,
        tripId: trip.tripId,
        runId: trip.runId,
        stopId: stop.stopId,
        occurredAt: occurredAt,
      );

  /// `deliveredUnits` is sent only when the server knows the stop's expected units, because it
  /// checks the quantity against them: DELIVERED must equal them, PARTIAL must be more than none
  /// and fewer than all, and a failed or refused stop delivers none. Without expected units the
  /// server rejects the field, so it is left out and the quantity stays unknown to the server.
  static OfflineOperation stopOutcome({
    required String operationId,
    required TripInfo trip,
    required StopInfo stop,
    required DeliveryDraft draft,
    required DateTime occurredAt,
    String? proofOperationId,
  }) {
    final expected = stop.units;
    final deliveredUnits = expected == null
        ? null
        : switch (draft.outcome) {
            DeliveryOutcome.delivered => expected,
            DeliveryOutcome.partial => draft.quantity,
            DeliveryOutcome.failed || DeliveryOutcome.refused => 0,
          };
    final reason = reasonFor(draft);
    return OfflineOperation(
      operationId: operationId,
      type: OperationType.stopOutcome,
      tripId: trip.tripId,
      runId: trip.runId,
      stopId: stop.stopId,
      occurredAt: occurredAt,
      dependsOnOperationId: proofOperationId,
      payload: {
        'code': outcomeCode(draft.outcome),
        if (reason != null) 'reason': reason,
        if (deliveredUnits != null) 'deliveredUnits': deliveredUnits,
        if (draft.notes.isNotEmpty) 'note': draft.notes,
      },
    );
  }

  static OfflineOperation routeCompleted({
    required String operationId,
    required TripInfo trip,
    required DateTime occurredAt,
  }) =>
      OfflineOperation(
        operationId: operationId,
        type: OperationType.routeCompleted,
        tripId: trip.tripId,
        runId: trip.runId,
        occurredAt: occurredAt,
      );

  static OfflineOperation incident({
    required String operationId,
    required TripInfo trip,
    required String category,
    required String description,
    required DateTime occurredAt,
    StopInfo? stop,
  }) =>
      OfflineOperation(
        operationId: operationId,
        type: OperationType.incidentReport,
        tripId: trip.tripId,
        runId: trip.runId,
        stopId: stop?.stopId,
        occurredAt: occurredAt,
        payload: {'category': category, 'description': description},
      );
}
