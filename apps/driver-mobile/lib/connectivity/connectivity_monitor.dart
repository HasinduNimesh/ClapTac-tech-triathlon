import 'dart:async';

import 'package:connectivity_plus/connectivity_plus.dart';

/// Whether the phone has a network. It is a hint, not proof: a Wi-Fi network with no internet behind it
/// counts as connected here, so failed requests (the sync worker's `offline` state) are treated as
/// being offline too.
abstract class ConnectivityMonitor {
  /// The state right now.
  Future<bool> isOnline();

  /// Emits whenever the state changes.
  Stream<bool> get changes;
}

/// The phone's network state through the platform.
class PlatformConnectivityMonitor implements ConnectivityMonitor {
  PlatformConnectivityMonitor({Connectivity? connectivity}) : _connectivity = connectivity ?? Connectivity();

  final Connectivity _connectivity;

  /// Online when at least one transport is up (Wi-Fi, mobile data, ethernet, VPN, ...).
  static bool onlineFrom(List<ConnectivityResult> results) =>
      results.any((result) => result != ConnectivityResult.none);

  @override
  Future<bool> isOnline() async => onlineFrom(await _connectivity.checkConnectivity());

  @override
  Stream<bool> get changes => _connectivity.onConnectivityChanged.map(onlineFrom).distinct();
}
