import '../shared/models.dart';
import '../sync/sync.dart';

class ProofRepository {
  ProofRepository(this.queue);

  final SyncQueue queue;

  Future<void> queueProof(ProofRecord proof) {
    return queue.enqueue(
      SyncEvent(
        eventId: proof.deliveryId,
        idempotencyKey: 'proof-${proof.deliveryId}',
        action: 'proof.create',
        resourceType: 'proof',
        resourceId: proof.deliveryId,
        payload: {'object_key': proof.objectKey},
      ),
    );
  }
}
