import 'package:flutter_test/flutter_test.dart';
import 'package:waypoint_loader/shared/models.dart';

void main() {
  test('a scanner burst counts as scanned, slow typing as typed', () {
    expect(LoadEntry.looksScanned('FR-4702', const Duration(milliseconds: 90)), isTrue);
    expect(LoadEntry.looksScanned('FR-4702', const Duration(seconds: 3)), isFalse);
    expect(LoadEntry.looksScanned('FR', Duration.zero), isFalse, reason: 'too short to be a label');
  });

  test('a typed order number needs a known reason before it can be sent', () {
    expect(const LoadEntry.manual('').complete, isFalse);
    expect(const LoadEntry.manual('SMUDGED').complete, isFalse);
    expect(const LoadEntry.manual('DAMAGED_LABEL').complete, isTrue);
    expect(const LoadEntry.scan().complete, isTrue);
  });

  test('the request body carries the method, and the reason and note only when typed', () {
    expect(const LoadEntry.scan().toJson(), {'entryMethod': 'SCAN'});
    expect(const LoadEntry.manual('UNREADABLE', note: '  torn corner ').toJson(), {'entryMethod': 'MANUAL', 'reasonCode': 'UNREADABLE', 'note': 'torn corner'});
    expect(const LoadEntry.manual('OTHER', note: ' ').toJson(), {'entryMethod': 'MANUAL', 'reasonCode': 'OTHER'});
  });
}
