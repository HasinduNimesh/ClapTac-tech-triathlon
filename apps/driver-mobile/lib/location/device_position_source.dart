import 'dart:io';

import 'package:geolocator/geolocator.dart';

import 'location_reporter.dart';

/// The phone's location service. While a trip runs it holds a visible "sharing location" notification, which is
/// what lets Android keep reporting with the screen off, and which tells the driver it is on.
class DevicePositionSource implements PositionSource {
  const DevicePositionSource();

  @override
  Future<LocationAccess> access() async {
    if (!await Geolocator.isLocationServiceEnabled()) return LocationAccess.serviceOff;
    return _map(await Geolocator.checkPermission());
  }

  @override
  Future<LocationAccess> requestAccess() async {
    if (!await Geolocator.isLocationServiceEnabled()) return LocationAccess.serviceOff;
    var permission = await Geolocator.checkPermission();
    if (permission == LocationPermission.denied) permission = await Geolocator.requestPermission();
    return _map(permission);
  }

  LocationAccess _map(LocationPermission permission) {
    switch (permission) {
      case LocationPermission.always:
      case LocationPermission.whileInUse:
        return LocationAccess.granted;
      case LocationPermission.deniedForever:
        return LocationAccess.deniedForever;
      case LocationPermission.denied:
      case LocationPermission.unableToDetermine:
        return LocationAccess.denied;
    }
  }

  @override
  Stream<Fix> positions() {
    final settings = Platform.isAndroid
        ? AndroidSettings(
            accuracy: LocationAccuracy.high,
            distanceFilter: 10,
            intervalDuration: const Duration(seconds: 15),
            foregroundNotificationConfig: const ForegroundNotificationConfig(
              notificationTitle: 'Waypoint Driver',
              notificationText: 'Sharing your truck\'s position with dispatch during this trip.',
              setOngoing: true,
            ),
          )
        : const LocationSettings(accuracy: LocationAccuracy.high, distanceFilter: 10);
    return Geolocator.getPositionStream(locationSettings: settings).map(
      (p) => Fix(latitude: p.latitude, longitude: p.longitude, at: p.timestamp.toUtc()),
    );
  }
}
