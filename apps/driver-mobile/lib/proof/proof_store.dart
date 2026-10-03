import 'dart:io';
import 'dart:typed_data';

import 'package:path_provider/path_provider.dart';

import '../data/driver_models.dart';
import '../sync/operations.dart';

/// Limits the server enforces (`MaxPhotoBytes`, `MaxSignatureBytes`). A file over them is refused
/// here, while the driver can still retake it, instead of failing silently when it uploads later.
const maxPhotoBytes = 4 * 1024 * 1024;
const maxSignatureBytes = 512 * 1024;

class ProofRejected implements Exception {
  const ProofRejected(this.message);

  final String message;

  @override
  String toString() => message;
}

/// Keeps captured proof as files in the app's own storage, so a photo taken without signal is still
/// there after a restart and can be uploaded when the phone is back online.
class FileProofStore {
  FileProofStore({Future<Directory> Function()? directory, DateTime Function()? clock, String Function()? newId})
      : _directory = directory ?? _defaultDirectory,
        _clock = clock ?? DateTime.now,
        _newId = newId ?? newOperationId;

  final Future<Directory> Function() _directory;
  final DateTime Function() _clock;
  final String Function() _newId;

  static Future<Directory> _defaultDirectory() async {
    final root = await getApplicationDocumentsDirectory();
    return Directory('${root.path}/proofs');
  }

  /// Copies a photo the camera produced into storage. Throws [ProofRejected] when it is not a JPEG
  /// or PNG, is empty, or is bigger than the server accepts.
  Future<CapturedProof> savePhoto(String sourcePath) async {
    final bytes = await File(sourcePath).readAsBytes();
    final mime = sniffMime(bytes);
    if (mime == null) throw const ProofRejected('That photo is not a JPEG or PNG image. Take it again.');
    if (bytes.length > maxPhotoBytes) throw const ProofRejected('That photo is larger than 4 MB. Take it again.');
    return _write(bytes, ProofKind.photo, mime, '');
  }

  /// Stores a drawn signature (PNG bytes).
  Future<CapturedProof> saveSignature(Uint8List png, {String receiverName = ''}) async {
    if (sniffMime(png) != 'image/png') throw const ProofRejected('The signature could not be saved. Try again.');
    if (png.length > maxSignatureBytes) throw const ProofRejected('The signature is too large. Clear it and sign again.');
    return _write(png, ProofKind.signature, 'image/png', receiverName.trim());
  }

  /// Removes a stored file the driver chose not to use. Missing files are ignored.
  Future<void> discard(CapturedProof proof) async {
    try {
      await File(proof.path).delete();
    } on FileSystemException {
      // Already gone.
    }
  }

  Future<CapturedProof> _write(List<int> bytes, ProofKind kind, String mime, String receiver) async {
    final directory = await _directory();
    await directory.create(recursive: true);
    final extension = mime == 'image/png' ? 'png' : 'jpg';
    final file = File('${directory.path}/${kind.name}-${_newId()}.$extension');
    await file.writeAsBytes(bytes, flush: true);
    return CapturedProof(kind: kind, path: file.path, mimeType: mime, capturedAt: _clock().toUtc(), receiverName: receiver);
  }
}

/// `image/jpeg` or `image/png` from the file's own signature bytes, null for anything else. The
/// server checks the same thing, so a mislabelled file would be refused after the driver left.
String? sniffMime(List<int> bytes) {
  if (bytes.length >= 3 && bytes[0] == 0xFF && bytes[1] == 0xD8 && bytes[2] == 0xFF) return 'image/jpeg';
  const png = [0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A];
  if (bytes.length >= png.length) {
    for (var i = 0; i < png.length; i++) {
      if (bytes[i] != png[i]) return null;
    }
    return 'image/png';
  }
  return null;
}
