import '../auth/auth_failure.dart';
import '../auth/auth_gateway.dart';
import '../data/driver_models.dart';
import 'trips_api.dart';

/// What loading today's route came back with.
class TripLoad {
  const TripLoad.loaded(TripInfo this.trip)
      : failure = null,
        signInExpired = false;
  const TripLoad.none()
      : trip = null,
        failure = null,
        signInExpired = false;
  const TripLoad.failed(String this.failure, {this.signInExpired = false}) : trip = null;

  final TripInfo? trip;

  /// A message for the driver when the route could not be loaded.
  final String? failure;

  /// The stored sign-in is no longer accepted, so the driver has to sign in again.
  final bool signInExpired;
}

abstract class TripSource {
  Future<TripLoad> loadToday();
}

/// Loads the signed-in driver's route for today from the Waypoint API. The server picks the
/// vehicle from the driver's profile.
class ApiTripSource implements TripSource {
  ApiTripSource({required this.api, required this.auth, DateTime Function()? clock}) : _clock = clock ?? DateTime.now;

  final TripsApi api;
  final AuthGateway auth;
  final DateTime Function() _clock;

  /// Waypoint's business day is the Asia/Colombo calendar date (fixed UTC+05:30, no daylight
  /// saving), the same zone the services use for delivery dates. It is computed from UTC so a
  /// phone set to another timezone still asks for the right day.
  static const businessOffset = Duration(hours: 5, minutes: 30);

  static String dateKey(DateTime now) {
    final business = now.toUtc().add(businessOffset);
    return '${business.year.toString().padLeft(4, '0')}-${business.month.toString().padLeft(2, '0')}-${business.day.toString().padLeft(2, '0')}';
  }

  @override
  Future<TripLoad> loadToday() async {
    final String? token;
    try {
      token = await auth.accessToken();
    } on AuthFailure catch (failure) {
      // A refresh was needed and Waypoint could not be reached: the sign-in is still good.
      return TripLoad.failed(failure.message ?? 'Could not reach Waypoint. Check your connection and try again.');
    }
    if (token == null) return const TripLoad.failed('Your sign-in expired. Sign in again.', signInExpired: true);
    try {
      final trips = await api.tripsFor(dateKey(_clock()), token);
      // One route at a time: the first trip of the day that is not finished.
      final open = trips.where((trip) => !trip.isCompleted).toList()..sort((a, b) => a.tripNumber.compareTo(b.tripNumber));
      if (open.isEmpty) return const TripLoad.none();
      final trip = await api.trip(open.first.tripId, token);
      if (trip.stops.isEmpty) return const TripLoad.none();
      return TripLoad.loaded(trip);
    } on TripsFailure catch (failure) {
      return TripLoad.failed(failure.message, signInExpired: failure.kind == TripsFailureKind.unauthorized);
    }
  }
}
