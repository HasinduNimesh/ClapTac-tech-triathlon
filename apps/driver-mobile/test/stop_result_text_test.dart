import 'package:flutter_test/flutter_test.dart';
import 'package:waypoint_driver/app/stop_result_text.dart';

void main() {
  group('stopFollowUpText', () {
    test('while updates are still queued it says they are only on the phone', () {
      expect(stopFollowUpText(short: 4, stillOnPhone: true), '4 units short - saved on this phone, not sent yet');
      expect(stopFollowUpText(note: 'Store closed', stillOnPhone: true), 'Store closed - saved on this phone, not sent yet');
      expect(stopFollowUpText(stillOnPhone: true), 'Saved on this phone, not sent yet');
    });

    test('once the queue is empty it says the server has it, instead of claiming it is unsent', () {
      expect(stopFollowUpText(short: 4, stillOnPhone: false), '4 units short - sent to Waypoint');
      expect(stopFollowUpText(note: 'Store closed', stillOnPhone: false), 'Store closed - sent to Waypoint');
      expect(stopFollowUpText(stillOnPhone: false), 'Sent to Waypoint');
    });

    test('one unit is not "1 units", and nothing short falls back to the note or the plain text', () {
      expect(stopFollowUpText(short: 1, stillOnPhone: false), '1 unit short - sent to Waypoint');
      expect(stopFollowUpText(short: 0, note: 'Wrong day', stillOnPhone: false), 'Wrong day - sent to Waypoint');
      expect(stopFollowUpText(short: -2, stillOnPhone: true), 'Saved on this phone, not sent yet');
      expect(stopFollowUpText(note: '   ', stillOnPhone: false), 'Sent to Waypoint');
    });
  });
}
