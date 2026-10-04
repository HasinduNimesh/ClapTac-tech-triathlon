import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:waypoint_driver/data/driver_models.dart';
import 'package:waypoint_driver/trips/trips_api.dart';

StopInfo _stop({int sequence = 1, String name = 'Dehiwala'}) =>
    StopInfo(sequence: sequence, outletCode: 'OUT1', name: name, windowStart: '', windowEnd: '', accessNote: '', contactNote: '', goods: 'G');

void main() {
  group('StopInfo.sequenceTitle', () {
    test('names the outlet after the stop number', () {
      expect(_stop(name: 'Dehiwala').sequenceTitle, 'Stop 1 · Dehiwala');
      expect(_stop(sequence: 3, name: 'Kirulapone').sequenceTitle, 'Stop 3 · Kirulapone');
    });

    test('does not repeat itself when the server sent no outlet name', () {
      expect(_stop(sequence: 1, name: 'Stop 1').sequenceTitle, 'Stop 1');
      expect(_stop(sequence: 2, name: 'Stop 2').sequenceTitle, 'Stop 2');
    });

    test('a stop mapped from the server without outlet details reads "Stop N" once', () {
      final trip = tripFromJson({
        'tripId': 't',
        'run': {'id': 'r', 'tripId': 't'},
        'stops': [
          {'id': 's', 'stopSequence': 4, 'status': 'pending'},
        ],
      });
      expect(trip.stops.single.sequenceTitle, 'Stop 4');
    });

    test('a different outlet name that merely starts with Stop is kept', () {
      expect(_stop(sequence: 1, name: 'Stop and Shop').sequenceTitle, 'Stop 1 · Stop and Shop');
    });
  });

  group('the installed app', () {
    test('is called Waypoint Driver, not its package name', () {
      final manifest = File('android/app/src/main/AndroidManifest.xml').readAsStringSync();
      expect(manifest, contains('android:label="Waypoint Driver"'));
      expect(manifest, isNot(contains('android:label="waypoint_driver"')));
    });
  });
}
