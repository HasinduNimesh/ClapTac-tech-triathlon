import 'dart:io';
import 'dart:typed_data';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:waypoint_driver/data/driver_models.dart';
import 'package:waypoint_driver/proof/proof_store.dart';
import 'package:waypoint_driver/proof/signature_pad.dart';

import 'helpers/render.dart';

final _jpeg = Uint8List.fromList([0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46, 0x00]);
final _png = Uint8List.fromList([0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0, 0, 0, 0]);

void main() {
  setUpAll(loadAppFonts);

  group('sniffMime', () {
    test('recognises JPEG and PNG by their signature, not by name', () {
      expect(sniffMime(_jpeg), 'image/jpeg');
      expect(sniffMime(_png), 'image/png');
      expect(sniffMime([0x47, 0x49, 0x46, 0x38, 0x39, 0x61]), isNull, reason: 'GIF');
      expect(sniffMime('RIFFxxxxWEBP'.codeUnits), isNull, reason: 'WebP');
      expect(sniffMime(const []), isNull);
      expect(sniffMime([0xFF, 0xD8]), isNull, reason: 'truncated');
    });
  });

  group('FileProofStore', () {
    late Directory root;
    late FileProofStore store;
    var counter = 0;

    setUp(() async {
      root = await Directory.systemTemp.createTemp('waypoint-store-test-');
      counter = 0;
      store = FileProofStore(directory: () async => Directory('${root.path}/proofs'), clock: () => DateTime.utc(2026, 10, 4, 9), newId: () => 'id-${++counter}');
    });

    tearDown(() => root.delete(recursive: true));

    Future<String> source(List<int> bytes, [String name = 'camera.jpg']) async {
      final file = File('${root.path}/$name');
      await file.writeAsBytes(bytes);
      return file.path;
    }

    test('copies a photo into app storage and describes it', () async {
      final proof = await store.savePhoto(await source(_jpeg));
      expect(proof.kind, ProofKind.photo);
      expect(proof.mimeType, 'image/jpeg');
      expect(proof.path, '${root.path}/proofs/photo-id-1.jpg');
      expect(proof.capturedAt, DateTime.utc(2026, 10, 4, 9));
      expect(await File(proof.path).readAsBytes(), _jpeg);
    });

    test('labels the photo by what it is, whatever the camera called the file', () async {
      final proof = await store.savePhoto(await source(_png, 'IMG_0001.jpg'));
      expect(proof.mimeType, 'image/png');
      expect(proof.path, endsWith('.png'));
    });

    test('refuses a file that is not a JPEG or PNG, an empty file, and a photo over 4 MB', () async {
      await expectLater(store.savePhoto(await source('GIF89a....'.codeUnits)), throwsA(isA<ProofRejected>()));
      await expectLater(store.savePhoto(await source(const [])), throwsA(isA<ProofRejected>()));
      final big = Uint8List(maxPhotoBytes + 1)..setRange(0, 3, [0xFF, 0xD8, 0xFF]);
      await expectLater(store.savePhoto(await source(big)), throwsA(isA<ProofRejected>().having((e) => e.message, 'message', contains('4 MB'))));
      expect(Directory('${root.path}/proofs').existsSync() ? Directory('${root.path}/proofs').listSync() : const [], isEmpty, reason: 'nothing is stored for a refused file');
    });

    test('stores a signature with the receiver name trimmed', () async {
      final proof = await store.saveSignature(_png, receiverName: '  S. Perera ');
      expect(proof.kind, ProofKind.signature);
      expect(proof.mimeType, 'image/png');
      expect(proof.receiverName, 'S. Perera');
      expect(proof.path, endsWith('signature-id-1.png'));
    });

    test('refuses a signature that is not a PNG or is over 512 KB', () async {
      await expectLater(store.saveSignature(_jpeg), throwsA(isA<ProofRejected>()));
      final big = Uint8List(maxSignatureBytes + 1)..setRange(0, 8, _png.sublist(0, 8));
      await expectLater(store.saveSignature(big), throwsA(isA<ProofRejected>()));
    });

    test('discarding removes the file and ignores one that is already gone', () async {
      final proof = await store.savePhoto(await source(_jpeg));
      await store.discard(proof);
      expect(File(proof.path).existsSync(), isFalse);
      await store.discard(proof);
    });
  });

  group('SignatureScreen', () {
    Future<void> open(WidgetTester tester, void Function(SignatureResult?) onResult) async {
      await pumpScreen(
        tester,
        Builder(builder: (context) => Scaffold(body: Center(child: TextButton(
          onPressed: () async => onResult(await Navigator.of(context).push<SignatureResult>(MaterialPageRoute(builder: (_) => const SignatureScreen()))),
          child: const Text('open'),
        )))),
      );
      await tester.tap(find.text('open'));
      await tester.pumpAndSettle();
    }

    testWidgets('drawing enables Use signature, and Clear empties the pad', (tester) async {
      await open(tester, (_) {});
      Finder button(String label) => find.ancestor(of: find.text(label), matching: find.byType(InkWell));
      expect(tester.widget<InkWell>(button('Use signature')).onTap, isNull);
      expect(tester.widget<InkWell>(button('Clear')).onTap, isNull);
      await tester.drag(find.byKey(const ValueKey('signature-pad')), const Offset(120, 40));
      await tester.pump();
      expect(find.text('Sign here'), findsNothing);
      expect(tester.widget<InkWell>(button('Use signature')).onTap, isNotNull);
      await tester.tap(find.text('Clear'));
      await tester.pump();
      expect(find.text('Sign here'), findsOneWidget);
      expect(tester.widget<InkWell>(button('Use signature')).onTap, isNull);
    });

    testWidgets('hands back a small PNG and the receiver name', (tester) async {
      SignatureResult? result;
      await open(tester, (value) => result = value);
      await tester.enterText(find.byType(TextField), '  S. Perera ');
      await tester.drag(find.byKey(const ValueKey('signature-pad')), const Offset(150, 50));
      await tester.pump();
      await tester.tap(find.text('Use signature'));
      await tester.runAsync(() => Future<void>.delayed(const Duration(milliseconds: 500)));
      await tester.pumpAndSettle();
      expect(result, isNotNull);
      expect(sniffMime(result!.png), 'image/png');
      expect(result!.png.length, lessThan(maxSignatureBytes));
      expect(result!.receiverName, 'S. Perera');
    });

    testWidgets('going back returns nothing', (tester) async {
      SignatureResult? result = SignatureResult(png: _png, receiverName: 'x');
      await open(tester, (value) => result = value);
      await tester.pageBack();
      await tester.pumpAndSettle();
      expect(result, isNull);
    });
  });
}
