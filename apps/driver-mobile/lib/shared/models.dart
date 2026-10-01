class Trip {
  const Trip({required this.id, required this.status});

  final String id;
  final String status;
}

class Delivery {
  const Delivery({required this.id, required this.tripId, required this.status});

  final String id;
  final String tripId;
  final String status;
}

class ProofRecord {
  const ProofRecord({required this.deliveryId, required this.objectKey});

  final String deliveryId;
  final String objectKey;
}
