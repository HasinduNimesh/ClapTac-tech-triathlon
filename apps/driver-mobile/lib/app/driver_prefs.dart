import 'dart:convert';
import 'dart:io';

import 'package:flutter/foundation.dart';
import 'package:path_provider/path_provider.dart';

/// How large a proof photo is taken. Smaller photos upload faster on a weak signal.
class PhotoSize {
  const PhotoSize(this.maxSide, this.quality);

  final double maxSide;
  final int quality;

  /// DR-6: shrunk from the camera's full size; still sharp enough to read a label or a delivery note.
  static const normal = PhotoSize(1280, 75);

  /// Low-data mode: about a third of the bytes of [normal].
  static const lowData = PhotoSize(960, 55);
}

/// Settings kept on this phone across sign-ins: low-data mode (DR-6) and whether the first-run sheet
/// about keeping Waypoint on the home screen and today's route offline was shown (DR-8).
class DriverPrefs extends ChangeNotifier {
  DriverPrefs({DriverPrefsStore? store}) : _store = store ?? MemoryDriverPrefsStore();

  final DriverPrefsStore _store;
  bool _lowData = false;
  bool _introSeen = false;
  bool _loaded = false;

  /// Low-data mode: route text first, smaller photos sent after the status updates, slower message checks.
  bool get lowData => _lowData;
  bool get introSeen => _introSeen;
  bool get loaded => _loaded;

  PhotoSize get photoSize => _lowData ? PhotoSize.lowData : PhotoSize.normal;

  /// How often to check for dispatch messages: four times less often in low-data mode.
  Duration messagePoll(Duration normal) => _lowData ? normal * 4 : normal;

  Future<void> load() async {
    final values = await _store.read();
    _lowData = values['lowData'] == true;
    _introSeen = values['introSeen'] == true;
    _loaded = true;
    notifyListeners();
  }

  Future<void> setLowData(bool value) async {
    if (_lowData == value) return;
    _lowData = value;
    notifyListeners();
    await _save();
  }

  Future<void> markIntroSeen() async {
    if (_introSeen) return;
    _introSeen = true;
    notifyListeners();
    await _save();
  }

  Future<void> _save() async {
    try {
      await _store.write({'lowData': _lowData, 'introSeen': _introSeen});
    } on Object {
      // A setting that cannot be saved still applies until the app is closed.
    }
  }
}

abstract class DriverPrefsStore {
  Future<Map<String, Object?>> read();
  Future<void> write(Map<String, Object?> values);
}

/// A small JSON file in the app's private storage. Missing or damaged means "defaults".
class FileDriverPrefsStore implements DriverPrefsStore {
  FileDriverPrefsStore({Future<Directory> Function()? directory}) : _directory = directory ?? getApplicationDocumentsDirectory;

  final Future<Directory> Function() _directory;

  Future<File> _file() async => File('${(await _directory()).path}/driver_prefs.json');

  @override
  Future<Map<String, Object?>> read() async {
    try {
      final file = await _file();
      if (!file.existsSync()) return const {};
      final decoded = jsonDecode(await file.readAsString());
      return decoded is Map ? decoded.cast<String, Object?>() : const {};
    } on Object {
      return const {};
    }
  }

  @override
  Future<void> write(Map<String, Object?> values) async {
    final file = await _file();
    await file.parent.create(recursive: true);
    await file.writeAsString(jsonEncode(values), flush: true);
  }
}

class MemoryDriverPrefsStore implements DriverPrefsStore {
  MemoryDriverPrefsStore([Map<String, Object?>? initial]) : values = {...?initial};

  Map<String, Object?> values;

  @override
  Future<Map<String, Object?>> read() async => {...values};

  @override
  Future<void> write(Map<String, Object?> next) async => values = {...next};
}
