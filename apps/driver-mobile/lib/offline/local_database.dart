import '../shared/models.dart';

/// Local persistence for offline-first route execution.
abstract class LocalDatabase {
  Future<void> upsertTrip(Trip trip);
  Future<Trip?> getTrip(String id);
  Future<void> upsertDelivery(Delivery delivery);
}

class InMemoryLocalDatabase implements LocalDatabase {
  final Map<String, Trip> _trips = {};
  final Map<String, Delivery> _deliveries = {};

  @override
  Future<void> upsertTrip(Trip trip) async => _trips[trip.id] = trip;

  @override
  Future<Trip?> getTrip(String id) async => _trips[id];

  @override
  Future<void> upsertDelivery(Delivery delivery) async =>
      _deliveries[delivery.id] = delivery;
}
