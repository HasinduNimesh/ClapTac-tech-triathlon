import '../data/driver_models.dart';

/// Where a driver is sent to reach a stop.
///
/// A recorded, exact position is navigated to. A district-centre position is never used as the
/// destination: it is kilometres from the shop. Then, and when the server has no position at all, the
/// maps app opens a search for the shop by name so the driver can pick the right place, and
/// [exact] is false so the app says so. The web driver page follows the same rule.
class StopDirections {
  const StopDirections._(this.uri, this.exact, this.searchText);

  /// Opens in the phone's maps app (or its browser).
  final Uri uri;

  /// True when [uri] navigates to the shop's recorded position.
  final bool exact;

  /// What is searched for when [exact] is false; empty otherwise.
  final String searchText;

  static StopDirections forStop(StopInfo stop) {
    final lat = stop.latitude;
    final lng = stop.longitude;
    if (lat != null && lng != null && !stop.locationApproximate && _isPlace(lat, lng)) {
      return StopDirections._(
        Uri.https('www.google.com', '/maps/dir/', {
          'api': '1',
          'destination': '${_trim(lat)},${_trim(lng)}',
          'travelmode': 'driving',
        }),
        true,
        '',
      );
    }
    final text = [
      stop.name.isNotEmpty ? stop.name : stop.outletCode,
      stop.district,
      'Sri Lanka',
    ].where((part) => part.isNotEmpty).join(', ');
    return StopDirections._(
      Uri.https('www.google.com', '/maps/search/', {'api': '1', 'query': text}),
      false,
      text,
    );
  }

  static bool _isPlace(double lat, double lng) => lat.isFinite && lng.isFinite && lat >= -90 && lat <= 90 && lng >= -180 && lng <= 180;

  /// 6.93441, not 6.934410000000001, and never exponent notation.
  static String _trim(double value) => value.toStringAsFixed(6).replaceFirst(RegExp(r'0+$'), '').replaceFirst(RegExp(r'\.$'), '');
}
