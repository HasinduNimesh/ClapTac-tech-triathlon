import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:waypoint_driver/data/driver_models.dart';
import 'package:waypoint_driver/main.dart';
import 'package:waypoint_driver/offline/local_database.dart';
import 'package:waypoint_driver/proof/proof_capturer.dart';
import 'package:waypoint_driver/sync/sync.dart';

import 'helpers/render.dart';

/// Hands back proof without a camera, and remembers what was asked for and discarded.
class _Capturer implements ProofCapturer {
  _Capturer({this.cancel = false});

  final bool cancel;
  final asked = <ProofKind>[];
  final discarded = <String>[];
  var _n = 0;

  @override
  Future<CapturedProof?> capture(BuildContext context, ProofKind kind) async {
    asked.add(kind);
    if (cancel) return null;
    final isPhoto = kind == ProofKind.photo;
    return CapturedProof(
      kind: kind,
      path: '/proofs/${kind.name}-${++_n}.${isPhoto ? 'jpg' : 'png'}',
      mimeType: isPhoto ? 'image/jpeg' : 'image/png',
      capturedAt: DateTime.utc(2026, 10, 4, 9),
      receiverName: isPhoto ? '' : 'S. Perera',
    );
  }

  @override
  Future<void> discard(CapturedProof proof) async => discarded.add(proof.path);
}

Future<InMemorySyncQueue> _boot(WidgetTester tester, {ProofCapturer? capturer}) async {
  final queue = InMemorySyncQueue();
  await pumpScreen(tester, WaypointDriverApp(database: InMemoryLocalDatabase(), queue: queue, demoAuth: true, capturer: capturer));
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
  await tester.tap(find.text('I’ve stopped safely'));
  await tester.pumpAndSettle();
  return queue;
}

Future<void> _tapText(WidgetTester tester, String text) async {
  await tester.ensureVisible(find.text(text));
  await tester.tap(find.text(text));
  await tester.pumpAndSettle();
}

Future<List<Map<String, Object?>>> _ops(InMemorySyncQueue queue) async => [for (final event in await queue.pending()) event.payload];

void main() {
  setUpAll(loadAppFonts);

  testWidgets('a delivered stop cannot be saved without proof when capture is available', (tester) async {
    final queue = await _boot(tester, capturer: _Capturer());
    await _tapText(tester, 'Save delivery');
    expect(find.textContaining('A delivery is not accepted without proof'), findsOneWidget);
    expect(find.text('Saved on this device'), findsNothing);
    expect((await _ops(queue)).map((op) => op['type']), ['ARRIVED']);
  });

  testWidgets('a photo is queued before the outcome, and the outcome depends on it', (tester) async {
    final capturer = _Capturer();
    final queue = await _boot(tester, capturer: capturer);
    await _tapText(tester, 'Take photo');
    expect(find.text('Photo saved'), findsOneWidget);
    await _tapText(tester, 'Save delivery');
    expect(find.text('Saved on this device'), findsOneWidget);

    final ops = await _ops(queue);
    expect(ops.map((op) => op['type']), ['ARRIVED', 'PROOF_UPLOAD', 'STOP_OUTCOME']);
    final proof = ops[1];
    expect(proof['stopId'], 'sample-stop-1');
    expect((proof['payload'] as Map)['proofType'], 'PHOTO');
    expect((proof['payload'] as Map)['filePath'], '/proofs/photo-1.jpg');
    expect(ops[2]['dependsOnOperationId'], proof['operationId']);
    expect(((ops[2]['payload']) as Map)['code'], 'DELIVERED');
    expect(((ops[2]['payload']) as Map)['deliveredUnits'], 12);
  });

  testWidgets('a photo and a signature are both queued, with the receiver name on the signature', (tester) async {
    final queue = await _boot(tester, capturer: _Capturer());
    await _tapText(tester, 'Take photo');
    await _tapText(tester, 'Add signature');
    await _tapText(tester, 'Save delivery');
    final ops = await _ops(queue);
    expect(ops.map((op) => op['type']), ['ARRIVED', 'PROOF_UPLOAD', 'PROOF_UPLOAD', 'STOP_OUTCOME']);
    expect((ops[2]['payload'] as Map)['proofType'], 'SIGNATURE');
    expect((ops[2]['payload'] as Map)['receiverName'], 'S. Perera');
    expect(ops[3]['dependsOnOperationId'], ops[2]['operationId'], reason: 'the outcome depends on the last proof, which is queued after the first');
    expect(ops[1]['operationId'], isNot(ops[2]['operationId']));
  });

  testWidgets('retaking a photo discards the first file and queues only the new one', (tester) async {
    final capturer = _Capturer();
    final queue = await _boot(tester, capturer: capturer);
    await _tapText(tester, 'Take photo');
    await tester.tap(find.text('Photo saved'));
    await tester.pumpAndSettle();
    expect(capturer.asked, [ProofKind.photo, ProofKind.photo]);
    expect(capturer.discarded, ['/proofs/photo-1.jpg']);
    await _tapText(tester, 'Save delivery');
    final proofs = (await _ops(queue)).where((op) => op['type'] == 'PROOF_UPLOAD').toList();
    expect(proofs, hasLength(1));
    expect((proofs.single['payload'] as Map)['filePath'], '/proofs/photo-2.jpg');
  });

  testWidgets('backing out of the camera leaves nothing captured', (tester) async {
    final capturer = _Capturer(cancel: true);
    await _boot(tester, capturer: capturer);
    await _tapText(tester, 'Take photo');
    expect(capturer.asked, [ProofKind.photo]);
    expect(find.text('Photo saved'), findsNothing);
    await _tapText(tester, 'Save delivery');
    expect(find.textContaining('A delivery is not accepted without proof'), findsOneWidget);
  });

  testWidgets('a partial delivery keeps the proof captured on the previous screen', (tester) async {
    final queue = await _boot(tester, capturer: _Capturer());
    await _tapText(tester, 'Take photo');
    await _tapText(tester, 'Partial');
    await _tapText(tester, 'Save delivery');
    expect(find.text('Record delivery'), findsOneWidget);
    await tester.enterText(find.byType(TextField).last, '5');
    await tester.pump();
    await _tapText(tester, 'Save delivery offline');
    expect(find.text('Saved on this device'), findsOneWidget);
    final ops = await _ops(queue);
    expect(ops.map((op) => op['type']), ['ARRIVED', 'PROOF_UPLOAD', 'STOP_OUTCOME']);
    expect(ops[2]['dependsOnOperationId'], ops[1]['operationId']);
    expect((ops[2]['payload'] as Map)['code'], 'PARTIAL');
    expect((ops[2]['payload'] as Map)['deliveredUnits'], 5);
  });

  testWidgets('a failed delivery needs no proof', (tester) async {
    final queue = await _boot(tester, capturer: _Capturer());
    await _tapText(tester, 'Failed');
    await _tapText(tester, 'Save delivery');
    await tester.pumpAndSettle();
    expect(find.textContaining('A delivery is not accepted without proof'), findsNothing);
    await _tapText(tester, 'Save take-back offline');
    final ops = await _ops(queue);
    expect(ops.map((op) => op['type']), ['ARRIVED', 'STOP_OUTCOME']);
    expect((ops.last['payload'] as Map)['deliveredUnits'], 0);
  });

  testWidgets('without a capturer the buttons say capture is unavailable and nothing is stored', (tester) async {
    final queue = await _boot(tester);
    await _tapText(tester, 'Take photo');
    expect(find.textContaining('capture is not available in this build'), findsOneWidget);
    expect(find.text('Photo saved'), findsNothing);
    await tester.pump(const Duration(seconds: 4));
    await _tapText(tester, 'Save delivery');
    expect((await _ops(queue)).map((op) => op['type']), ['ARRIVED', 'STOP_OUTCOME'], reason: 'demo builds still save without proof');
  });
}
