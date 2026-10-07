import 'dart:io';

import 'package:flutter_test/flutter_test.dart';

// The load-check sheet must say what really happens: confirming sends the check to Waypoint (see
// ApiTripStarter), and with no signal it waits on the phone. It once said it was kept on the phone only.
void main() {
  final sheet = File('lib/screens/route/truck_checkout_sheet.dart').readAsStringSync();

  test('the load check sheet says it is sent, and what happens without signal', () {
    expect(sheet, contains('the load check goes to Waypoint so the loader and dispatcher can see it'));
    expect(sheet, contains('With no signal it waits on this phone and is sent when you are back online.'));
  });

  test('it no longer says the check is kept on the phone only', () {
    expect(sheet, isNot(contains('Kept on this phone only')));
    expect(sheet, isNot(contains('not sent to the loader or dispatcher yet')));
  });
}
