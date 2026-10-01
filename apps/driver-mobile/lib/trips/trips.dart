import '../offline/local_database.dart';
import '../shared/models.dart';

class TripRepository {
  TripRepository(this.db);

  final LocalDatabase db;

  Future<Trip?> assigned(String id) => db.getTrip(id);
}
