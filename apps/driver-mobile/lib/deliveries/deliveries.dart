import '../offline/local_database.dart';
import '../shared/models.dart';
import '../sync/sync.dart';

class DeliveryRepository {
  DeliveryRepository(this.db, this.queue);

  final LocalDatabase db;
  final SyncQueue queue;

  Future<void> recordOutcome(Delivery delivery) async {
    await db.upsertDelivery(delivery);
    await queue.enqueue(
      SyncEvent(
        eventId: delivery.id,
        idempotencyKey: 'delivery-outcome-${delivery.id}',
        action: 'delivery.outcome_changed',
        resourceType: 'delivery',
        resourceId: delivery.id,
        payload: {'status': delivery.status},
      ),
    );
  }
}
