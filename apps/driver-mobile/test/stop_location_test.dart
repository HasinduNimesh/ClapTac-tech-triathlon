import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:waypoint_driver/data/driver_models.dart';
import 'package:waypoint_driver/main.dart';
import 'package:waypoint_driver/maps/map_launcher.dart';
import 'package:waypoint_driver/maps/stop_directions.dart';
import 'package:waypoint_driver/offline/local_database.dart';
import 'package:waypoint_driver/sync/sync.dart';
import 'package:waypoint_driver/trips/trips_api.dart';

import 'helpers/render.dart';

StopInfo _stop({double? latitude, double? longitude, bool approximate = false, String name = 'Fresh Pettah', String district = 'Colombo', String code = 'OUT001'}) => StopInfo(
      sequence: 1,
      outletCode: code,
      name: name,
      district: district,
      windowStart: '09:00',
      windowEnd: '10:00',
      accessNote: '',
      contactNote: '',
      goods: 'Goods',
      latitude: latitude,
      longitude: longitude,
      locationApproximate: approximate,
    );

Map<String, Object?> _serverStop(Map<String, Object?> extra) => {
      'id': 'stop-1',
      'outletId': 'OUT108',
      'outletName': 'Dehiwala',
      'district': 'Colombo',
      'stopSequence': 1,
      'status': 'pending',
      ...extra,
    };

Map<String, Object?> _detail(List<Map<String, Object?>> stops) => {
      'tripId': 'TRP02801',
      'status': 'prepared',
      'run': {'id': 'run-1', 'tripId': 'TRP02801', 'planRef': 'PLAN000002', 'vehicleId': 'VEH001', 'status': 'prepared'},
      'stops': stops,
    };

class _RecordingLauncher implements MapLauncher {
  _RecordingLauncher({this.opens = true});
  final bool opens;
  final opened = <Uri>[];

  @override
  Future<bool> open(Uri uri) async {
    opened.add(uri);
    return opens;
  }
}

void main() {
  setUpAll(loadAppFonts);

  group('directions to a stop', () {
    test('an exact recorded position is navigated to', () {
      final directions = StopDirections.forStop(_stop(latitude: 6.93441, longitude: 79.84281));
      expect(directions.exact, isTrue);
      expect(directions.uri.toString(), 'https://www.google.com/maps/dir/?api=1&destination=6.93441%2C79.84281&travelmode=driving');
    });

    test('an approximate district position is never the destination; the shop is searched for by name', () {
      final directions = StopDirections.forStop(_stop(latitude: 6.9, longitude: 79.9, approximate: true));
      expect(directions.exact, isFalse);
      expect(directions.searchText, 'Fresh Pettah, Colombo, Sri Lanka');
      expect(directions.uri.toString(), contains('/maps/search/'));
      expect(directions.uri.toString(), isNot(contains('79.9')));
    });

    test('a stop with no position, or an impossible one, is searched for by name, falling back to the outlet code', () {
      expect(StopDirections.forStop(_stop()).exact, isFalse);
      expect(StopDirections.forStop(_stop(latitude: 123, longitude: 79.8)).exact, isFalse);
      expect(StopDirections.forStop(_stop(latitude: 6.9, longitude: double.nan)).exact, isFalse);
      final unnamed = StopDirections.forStop(_stop(name: '', district: '', code: 'OUT007'));
      expect(unnamed.searchText, 'OUT007, Sri Lanka');
    });

    test('coordinates are written plainly, without float noise', () {
      final directions = StopDirections.forStop(_stop(latitude: 6.9, longitude: 79.123456789));
      expect(directions.uri.queryParameters['destination'], '6.9,79.123457');
    });
  });

  group('a stop keeps its position', () {
    test('the position and district survive saving the route on the phone', () {
      final restored = StopInfo.fromJson(_stop(latitude: 6.93441, longitude: 79.84281).toJson());
      expect(restored.latitude, 6.93441);
      expect(restored.longitude, 79.84281);
      expect(restored.locationApproximate, isFalse);
      expect(restored.district, 'Colombo');
      final approximate = StopInfo.fromJson(_stop(latitude: 6.9, longitude: 79.9, approximate: true).toJson());
      expect(approximate.locationApproximate, isTrue);
    });

    test('a route saved before positions existed still opens, with no position', () {
      final json = _stop().toJson()
        ..remove('latitude')
        ..remove('longitude')
        ..remove('locationApproximate')
        ..remove('district');
      final restored = StopInfo.fromJson(json);
      expect(restored.latitude, isNull);
      expect(restored.locationApproximate, isFalse);
      expect(restored.district, '');
    });

    test('a damaged saved position is recognised as damaged', () {
      expect(() => StopInfo.fromJson(_stop().toJson()..['latitude'] = '6.9'), throwsFormatException);
      expect(() => StopInfo.fromJson(_stop().toJson()..['locationApproximate'] = 'yes'), throwsFormatException);
    });

    test('the position comes from the server, with its approximate flag', () {
      final trip = tripFromJson(_detail([
        _serverStop({'latitude': 6.93441, 'longitude': 79.84281}),
        _serverStop({'id': 'stop-2', 'stopSequence': 2, 'latitude': 6.9, 'longitude': 79.9, 'locationApproximate': true}),
        _serverStop({'id': 'stop-3', 'stopSequence': 3}),
        _serverStop({'id': 'stop-4', 'stopSequence': 4, 'latitude': 'six', 'longitude': null}),
      ]));
      final stops = trip.stops;
      expect(stops[0].latitude, 6.93441);
      expect(stops[0].longitude, 79.84281);
      expect(stops[0].locationApproximate, isFalse);
      expect(stops[0].district, 'Colombo');
      expect(stops[1].locationApproximate, isTrue);
      expect(stops[2].latitude, isNull);
      expect(stops[3].latitude, isNull, reason: 'a coordinate that is not a number is ignored, so the stop is searched for by name');
    });
  });

  group('Open in maps', () {
    Future<void> reachTheStopScreen(WidgetTester tester, MapLauncher? launcher) async {
      await pumpScreen(tester, WaypointDriverApp(database: InMemoryLocalDatabase(), queue: InMemorySyncQueue(), demoAuth: true, mapLauncher: launcher));
      await tester.enterText(find.byType(TextField).first, 'DRV-0318');
      await tester.enterText(find.byType(TextField).last, 'secret');
      await tester.pump();
      await tester.tap(find.text('Sign in').last);
      await tester.pumpAndSettle();
      await tester.tap(find.text('Confirm load on board'));
      await tester.pumpAndSettle();
      await tester.pump(const Duration(seconds: 4));
      await tester.pumpAndSettle();
      await tester.tap(find.text('View Stop'));
      await tester.pumpAndSettle();
    }

    testWidgets('opens the phone\'s maps app; with no recorded position it searches for the shop and says so', (tester) async {
      final launcher = _RecordingLauncher();
      await reachTheStopScreen(tester, launcher);
      await tester.tap(find.text('Open in maps'));
      await tester.pumpAndSettle();

      expect(launcher.opened, hasLength(1));
      expect(launcher.opened.single.path, '/maps/search/');
      expect(launcher.opened.single.queryParameters['query'], endsWith('Sri Lanka'));
      expect(find.textContaining('Exact location not recorded'), findsOneWidget);
    });

    testWidgets('says so when no maps app can be opened', (tester) async {
      final launcher = _RecordingLauncher(opens: false);
      await reachTheStopScreen(tester, launcher);
      await tester.tap(find.text('Open in maps'));
      await tester.pumpAndSettle();

      expect(find.textContaining('Could not open a maps app'), findsOneWidget);
    });

    testWidgets('says so when this build has no maps launcher, instead of doing nothing', (tester) async {
      await reachTheStopScreen(tester, null);
      await tester.tap(find.text('Open in maps'));
      await tester.pumpAndSettle();

      expect(find.text('Maps are not available in this build.'), findsOneWidget);
    });
  });
}
