import 'dart:typed_data';
import 'dart:ui' as ui;

import 'package:flutter/material.dart';

import '../theme/tokens.dart';
import '../widgets/app_buttons.dart';

/// Strokes drawn on the pad, kept as plain points so they can be redrawn and rendered to a PNG.
class SignatureController extends ChangeNotifier {
  final List<List<Offset>> _strokes = [];

  List<List<Offset>> get strokes => List.unmodifiable(_strokes);
  bool get isEmpty => _strokes.every((stroke) => stroke.isEmpty);

  void start(Offset point) {
    _strokes.add([point]);
    notifyListeners();
  }

  void extend(Offset point) {
    if (_strokes.isEmpty) return;
    _strokes.last.add(point);
    notifyListeners();
  }

  void clear() {
    _strokes.clear();
    notifyListeners();
  }

  /// A white PNG of the signature, [width] x [height] pixels. The size is fixed and the drawing is
  /// scaled to it, so the file stays far below the server's 512 KB limit.
  Future<Uint8List> toPng(Size padSize, {int width = 600, int height = 240}) async {
    final recorder = ui.PictureRecorder();
    final canvas = Canvas(recorder, Rect.fromLTWH(0, 0, width.toDouble(), height.toDouble()));
    canvas.drawRect(Rect.fromLTWH(0, 0, width.toDouble(), height.toDouble()), Paint()..color = Colors.white);
    final scaleX = width / padSize.width;
    final scaleY = height / padSize.height;
    canvas.scale(scaleX, scaleY);
    _paintStrokes(canvas, _strokes, strokeWidth: 3);
    final image = await recorder.endRecording().toImage(width, height);
    final data = await image.toByteData(format: ui.ImageByteFormat.png);
    image.dispose();
    return data!.buffer.asUint8List();
  }
}

void _paintStrokes(Canvas canvas, List<List<Offset>> strokes, {required double strokeWidth}) {
  final paint = Paint()
    ..color = Colors.black
    ..strokeWidth = strokeWidth
    ..strokeCap = StrokeCap.round
    ..strokeJoin = StrokeJoin.round
    ..style = PaintingStyle.stroke;
  for (final stroke in strokes) {
    if (stroke.length == 1) {
      canvas.drawCircle(stroke.first, strokeWidth / 2, paint..style = PaintingStyle.fill);
      paint.style = PaintingStyle.stroke;
    } else if (stroke.length > 1) {
      final path = Path()..moveTo(stroke.first.dx, stroke.first.dy);
      for (final point in stroke.skip(1)) {
        path.lineTo(point.dx, point.dy);
      }
      canvas.drawPath(path, paint);
    }
  }
}

class _SignaturePainter extends CustomPainter {
  _SignaturePainter(this.controller) : super(repaint: controller);

  final SignatureController controller;

  @override
  void paint(Canvas canvas, Size size) {
    canvas.clipRect(Offset.zero & size);
    _paintStrokes(canvas, controller.strokes, strokeWidth: 3);
  }

  @override
  bool shouldRepaint(_SignaturePainter oldDelegate) => true;
}

/// What the signature screen hands back: the PNG and the name typed for who is signing.
class SignatureResult {
  const SignatureResult({required this.png, required this.receiverName});

  final Uint8List png;
  final String receiverName;
}

/// Full-screen pad for the person receiving the goods to sign. Pops a [SignatureResult], or null
/// when the driver goes back.
class SignatureScreen extends StatefulWidget {
  const SignatureScreen({super.key});

  @override
  State<SignatureScreen> createState() => _SignatureScreenState();
}

class _SignatureScreenState extends State<SignatureScreen> {
  final _controller = SignatureController();
  final _name = TextEditingController();
  final _padKey = GlobalKey();
  bool _saving = false;

  @override
  void dispose() {
    _controller.dispose();
    _name.dispose();
    super.dispose();
  }

  Future<void> _save() async {
    final size = (_padKey.currentContext?.size) ?? const Size(300, 150);
    setState(() => _saving = true);
    final png = await _controller.toPng(size);
    if (!mounted) return;
    Navigator.of(context).pop(SignatureResult(png: png, receiverName: _name.text.trim()));
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: Colors.white,
      appBar: AppBar(
        backgroundColor: Colors.white,
        foregroundColor: AppColors.ink,
        elevation: 0,
        title: Text('Receiver’s signature', style: AppText.of(18, FontWeight.w700)),
      ),
      body: SafeArea(
        child: Padding(
          padding: const EdgeInsets.all(16),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text('Ask the person receiving the goods to sign below.', style: AppText.of(14, FontWeight.w400, color: AppColors.muted)),
              const SizedBox(height: 12),
              TextField(
                controller: _name,
                maxLength: 120,
                textCapitalization: TextCapitalization.words,
                decoration: const InputDecoration(labelText: 'Name of the person receiving (optional)', border: OutlineInputBorder()),
              ),
              const SizedBox(height: 4),
              Expanded(
                child: Semantics(
                  label: 'Signature pad',
                  child: Container(
                    key: _padKey,
                    decoration: BoxDecoration(border: Border.all(color: AppColors.fieldLabel), borderRadius: BorderRadius.circular(8)),
                    child: GestureDetector(
                      key: const ValueKey('signature-pad'),
                      behavior: HitTestBehavior.opaque,
                      onPanStart: (details) => _controller.start(details.localPosition),
                      onPanUpdate: (details) => _controller.extend(details.localPosition),
                      child: ListenableBuilder(
                        listenable: _controller,
                        builder: (context, _) => Stack(
                          children: [
                            Positioned.fill(child: CustomPaint(painter: _SignaturePainter(_controller))),
                            if (_controller.isEmpty)
                              Center(child: Text('Sign here', style: AppText.of(16, FontWeight.w400, color: AppColors.fieldLabel))),
                          ],
                        ),
                      ),
                    ),
                  ),
                ),
              ),
              const SizedBox(height: 12),
              ListenableBuilder(
                listenable: _controller,
                builder: (context, _) => Row(
                  children: [
                    Expanded(child: AppButton(label: 'Clear', outline: true, onPressed: _controller.isEmpty ? null : _controller.clear)),
                    const SizedBox(width: 12),
                    Expanded(child: AppButton(label: 'Use signature', onPressed: _controller.isEmpty || _saving ? null : _save)),
                  ],
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}
