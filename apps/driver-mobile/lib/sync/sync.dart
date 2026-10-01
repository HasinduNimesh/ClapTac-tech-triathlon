enum ConflictResolution { clientWins, serverWins, merge }

class SyncEvent {
  SyncEvent({
    required this.eventId,
    required this.idempotencyKey,
    required this.action,
    required this.resourceType,
    required this.resourceId,
    required this.payload,
  });

  final String eventId;
  final String idempotencyKey;
  final String action;
  final String resourceType;
  final String resourceId;
  final Map<String, Object?> payload;
}

abstract class SyncQueue {
  Future<void> enqueue(SyncEvent event);
  Future<List<SyncEvent>> pending();
  Future<void> markSynced(String idempotencyKey);
}

class InMemorySyncQueue implements SyncQueue {
  final Map<String, SyncEvent> _events = {};

  @override
  Future<void> enqueue(SyncEvent event) async {
    _events.putIfAbsent(event.idempotencyKey, () => event);
  }

  @override
  Future<List<SyncEvent>> pending() async => _events.values.toList();

  @override
  Future<void> markSynced(String idempotencyKey) async {
    _events.remove(idempotencyKey);
  }
}
