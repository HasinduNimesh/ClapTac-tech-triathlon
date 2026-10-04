import 'package:url_launcher/url_launcher.dart';

/// Opens an address in the phone's maps app. Behind an interface so screens and tests do not depend
/// on the plugin.
abstract class MapLauncher {
  /// True when a maps app (or the browser) was opened.
  Future<bool> open(Uri uri);
}

class DeviceMapLauncher implements MapLauncher {
  const DeviceMapLauncher();

  @override
  Future<bool> open(Uri uri) async {
    try {
      return await launchUrl(uri, mode: LaunchMode.externalApplication);
    } catch (_) {
      return false;
    }
  }
}
