import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:path_provider/path_provider.dart';

import '../data/driver_models.dart';

/// The route as last loaded from Waypoint, kept on the phone so the app can still show it when it is
/// opened without a connection.
class SavedRoute {
  const SavedRoute({required this.businessDate, required this.trip});

  /// The Waypoint business day (`yyyy-MM-dd`, Asia/Colombo) the route belongs to. A route saved on an
  /// earlier day is never shown.
  final String businessDate;
  final TripInfo trip;

  Map<String, Object?> toJson() => {'businessDate': businessDate, 'trip': trip.toJson()};

  factory SavedRoute.fromJson(Map<String, Object?> json) {
    final trip = json['trip'];
    final date = json['businessDate'];
    if (trip is! Map || date is! String || date.isEmpty) throw const FormatException('saved route');
    return SavedRoute(businessDate: date, trip: TripInfo.fromJson(trip.cast<String, Object?>()));
  }
}

/// Keeps one saved route per signed-in driver, so a phone shared between drivers never shows one
/// driver's route to another.
abstract class RouteStore {
  Future<SavedRoute?> read(String userId);
  Future<void> write(String userId, SavedRoute route);
  Future<void> clear(String userId);
}

/// A JSON file per driver in the app's private storage. A missing, unreadable or damaged file is
/// simply "no saved route".
class FileRouteStore implements RouteStore {
  FileRouteStore({Future<Directory> Function()? directory}) : _directory = directory ?? _defaultDirectory;

  final Future<Directory> Function() _directory;

  /// What each driver's file is busy with. Reads, writes and clears for one driver run one after the
  /// other, in the order they were asked for: two saves cannot share the temporary file, an older
  /// snapshot cannot overwrite a newer one, and a save still in progress cannot bring the route back
  /// after a clear (sign-out). Different drivers do not wait on each other.
  final Map<String, Future<void>> _tails = {};

  Future<T> _serial<T>(String userId, Future<T> Function() operation) {
    final previous = _tails[userId] ?? Future<void>.value();
    final result = Completer<T>();
    final tail = previous.then((_) async {
      try {
        result.complete(await operation());
      } on Object catch (error, stack) {
        // The caller gets the error; the queue carries on for whoever is next.
        result.completeError(error, stack);
      }
    });
    _tails[userId] = tail;
    unawaited(tail.whenComplete(() {
      if (identical(_tails[userId], tail)) _tails.remove(userId);
    }));
    return result.future;
  }

  static Future<Directory> _defaultDirectory() async {
    final root = await getApplicationDocumentsDirectory();
    return Directory('${root.path}/routes');
  }

  /// The user id comes from Waypoint, but it is still kept out of the file name's path.
  static String fileName(String userId) => 'route-${userId.replaceAll(RegExp(r'[^A-Za-z0-9_-]'), '_')}.json';

  Future<File> _file(String userId) async => File('${(await _directory()).path}/${fileName(userId)}');

  @override
  Future<SavedRoute?> read(String userId) => _serial(userId, () async {
        try {
          final file = await _file(userId);
          if (!file.existsSync()) return null;
          final decoded = jsonDecode(await file.readAsString());
          if (decoded is! Map) return null;
          return SavedRoute.fromJson(decoded.cast<String, Object?>());
        } on FormatException {
          return null;
        } on FileSystemException {
          return null;
        } on TypeError {
          // Valid JSON of the wrong shape. The models report this as a FormatException, but whatever
          // is wrong with a saved file, the answer is "no saved route", never a failed start-up.
          return null;
        }
      });

  @override
  Future<void> write(String userId, SavedRoute route) => _serial(userId, () async {
        final directory = await _directory();
        await directory.create(recursive: true);
        final file = await _file(userId);
        // Written to a temporary file first so a crash mid-write cannot leave half a route behind.
        final temporary = File('${file.path}.tmp');
        await temporary.writeAsString(jsonEncode(route.toJson()), flush: true);
        await temporary.rename(file.path);
      });

  @override
  Future<void> clear(String userId) => _serial(userId, () async {
        try {
          final file = await _file(userId);
          if (file.existsSync()) await file.delete();
        } on FileSystemException {
          // Nothing more to do: a leftover file for another day is ignored anyway.
        }
      });
}

class MemoryRouteStore implements RouteStore {
  final saved = <String, SavedRoute>{};

  @override
  Future<SavedRoute?> read(String userId) async => saved[userId];

  @override
  Future<void> write(String userId, SavedRoute route) async => saved[userId] = route;

  @override
  Future<void> clear(String userId) async => saved.remove(userId);
}
