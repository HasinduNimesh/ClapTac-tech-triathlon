import 'dart:io';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:image_picker/image_picker.dart';

import '../app/driver_prefs.dart';
import '../data/driver_models.dart';
import 'proof_store.dart';
import 'signature_pad.dart';

/// Asks the driver for a photo or a signature. Behind an interface so screens and tests do not
/// depend on the camera plugin.
abstract class ProofCapturer {
  /// A captured proof stored on the phone, or null when the driver backed out or it failed (the
  /// driver has already been told why).
  Future<CapturedProof?> capture(BuildContext context, ProofKind kind);

  /// Deletes a captured proof the driver replaced or removed.
  Future<void> discard(CapturedProof proof);
}

/// The phone's camera and a signature pad, stored through [FileProofStore].
class DeviceProofCapturer implements ProofCapturer {
  DeviceProofCapturer({required this.store, ImagePicker? picker, PhotoSize Function()? photoSize})
      : _picker = picker ?? ImagePicker(),
        _photoSize = photoSize ?? (() => PhotoSize.normal);

  final FileProofStore store;
  final PhotoSize Function() _photoSize;
  final ImagePicker _picker;

  @override
  Future<CapturedProof?> capture(BuildContext context, ProofKind kind) {
    switch (kind) {
      case ProofKind.photo:
        return _photo(context);
      case ProofKind.signature:
        return _signature(context);
    }
  }

  /// DR-6: photos are scaled and compressed by the camera plugin (1280 px, quality 75; 960 px, quality
  /// 55 in low-data mode), which keeps them well under the server's 4 MB limit and quick to upload on a
  /// weak connection. The sync queue sends status updates before them.
  Future<CapturedProof?> _photo(BuildContext context) async {
    final XFile? shot;
    final size = _photoSize();
    try {
      shot = await _picker.pickImage(
        source: ImageSource.camera,
        maxWidth: size.maxSide,
        maxHeight: size.maxSide,
        imageQuality: size.quality,
        preferredCameraDevice: CameraDevice.rear,
      );
    } on PlatformException catch (error) {
      if (context.mounted) _tell(context, _cameraProblem(error));
      return null;
    }
    if (shot == null) return null;
    try {
      return await store.savePhoto(shot.path);
    } on ProofRejected catch (error) {
      if (context.mounted) _tell(context, error.message);
      return null;
    } finally {
      // The plugin's own temporary copy is no longer needed.
      try {
        await File(shot.path).delete();
      } on FileSystemException {
        // Nothing depends on it.
      }
    }
  }

  Future<CapturedProof?> _signature(BuildContext context) async {
    final result = await Navigator.of(context).push<SignatureResult>(MaterialPageRoute(builder: (_) => const SignatureScreen()));
    if (result == null) return null;
    try {
      return await store.saveSignature(result.png, receiverName: result.receiverName);
    } on ProofRejected catch (error) {
      if (context.mounted) _tell(context, error.message);
      return null;
    }
  }

  @override
  Future<void> discard(CapturedProof proof) => store.discard(proof);

  static String _cameraProblem(PlatformException error) {
    if (error.code == 'camera_access_denied') {
      return 'The camera is blocked. Allow camera access for Waypoint Driver in Settings, then try again.';
    }
    return 'The camera could not be opened. Try again, or add a signature instead.';
  }

  static void _tell(BuildContext context, String message) {
    ScaffoldMessenger.of(context)
      ..hideCurrentSnackBar()
      ..showSnackBar(SnackBar(content: Text(message), behavior: SnackBarBehavior.floating, showCloseIcon: true));
  }
}
