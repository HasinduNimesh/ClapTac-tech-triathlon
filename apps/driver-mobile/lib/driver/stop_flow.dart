import 'dart:ui' as ui;

import 'package:flutter/material.dart';
import 'package:image_picker/image_picker.dart';
import 'package:url_launcher/url_launcher.dart';

import '../shared/models.dart';
import '../sync/sync.dart';
import '../theme/tokens.dart';
import '../widgets/common.dart';
import 'driver_controller.dart';
import 'sheets.dart';
import 'sync_status_screen.dart';

void openStop(BuildContext context, DriverController c, DeliveryStop stop) {
  Navigator.of(context).push(MaterialPageRoute(
    builder: (_) => stop.arrivedAt == null && !stop.done ? SafeStopScreen(controller: c, stopId: stop.id) : RecordDeliveryScreen(controller: c, stopId: stop.id),
  ));
}

class _StopScaffold extends StatelessWidget {
  const _StopScaffold({required this.children, this.bottom});

  final List<Widget> children;
  final Widget? bottom;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: Column(children: [
        WpPhoneHeader(trailing: todayLabel()),
        Expanded(
          child: Transform.translate(
            offset: const Offset(0, -14),
            child: Container(
              decoration: const BoxDecoration(color: Wp.surface, borderRadius: BorderRadius.vertical(top: Radius.circular(20))),
              child: ListView(padding: const EdgeInsets.all(Wp.space16), children: children),
            ),
          ),
        ),
        if (bottom != null) SafeArea(top: false, child: Padding(padding: const EdgeInsets.fromLTRB(16, 0, 16, 12), child: bottom)),
      ]),
    );
  }
}

/// Safe stop & outlet access: shows when the receiving window opens and how
/// long to wait. Nothing can be recorded until the driver says they are parked.
class SafeStopScreen extends StatelessWidget {
  const SafeStopScreen({super.key, required this.controller, required this.stopId});

  final DriverController controller;
  final String stopId;

  @override
  Widget build(BuildContext context) {
    return ListenableBuilder(
      listenable: controller,
      builder: (context, _) {
        final trip = controller.trip;
        final stop = trip?.stops.where((s) => s.id == stopId).firstOrNull;
        if (trip == null || stop == null) return const Scaffold(body: Center(child: Text('Stop not found on this route.')));
        final open = DateTime.tryParse(stop.windowOpen ?? '');
        final close = DateTime.tryParse(stop.windowClose ?? '');
        final now = DateTime.now().toUtc();
        final earlyMinutes = open == null ? 0 : open.toUtc().difference(now).inMinutes;
        final lateMinutes = close == null ? 0 : now.difference(close.toUtc()).inMinutes;
        return _StopScaffold(children: [
          Text('Stop ${stop.sequence} · ${stop.outletName}', style: const TextStyle(fontSize: 22, fontWeight: FontWeight.w700)),
          Text('Trip ${trip.planRef.isEmpty ? trip.tripId : trip.planRef} · Plan v${trip.planVersion}', style: const TextStyle(color: Wp.muted)),
          const SizedBox(height: 16),
          WpCard(
            child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
              Text('${stop.outletId} · ${stop.outletName}', style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 16)),
              Text('Requested window ${hhmm(stop.windowOpen)}–${hhmm(stop.windowClose)}', style: const TextStyle(color: Wp.muted)),
              const SizedBox(height: 8),
              if (earlyMinutes > 0) StatusTag('$earlyMinutes min early', tone: Tone.amber) else if (lateMinutes > 0) StatusTag('$lateMinutes min after the window', tone: Tone.red) else const StatusTag('Inside the window', tone: Tone.green),
            ]),
          ),
          const SizedBox(height: 12),
          WpCard(
            child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
              if (earlyMinutes > 0) ...[
                const Text('Wait for the receiving window', style: TextStyle(fontWeight: FontWeight.w700, fontSize: 18)),
                Text('Earliest access ${hhmm(stop.windowOpen)} · estimated wait $earlyMinutes min', style: const TextStyle(color: Wp.amber, fontWeight: FontWeight.w500)),
                const Divider(height: 24),
              ],
              Text([stop.dockType, stop.parking].where((s) => s.isNotEmpty).join(' · ').ifEmpty('Outlet access'), style: const TextStyle(fontWeight: FontWeight.w600)),
              Text(stop.access.ifEmpty('Call the store on arrival.'), style: const TextStyle(color: Wp.muted)),
            ]),
          ),
          const SizedBox(height: 12),
          const StatusNote(tone: Tone.amber, title: 'Before continuing', text: 'Confirm you are parked safely. Do not interact while driving.'),
          const SizedBox(height: 16),
          FilledButton(
            onPressed: () async {
              await controller.arrive(stop);
              if (!context.mounted) return;
              Navigator.of(context).pushReplacement(MaterialPageRoute(builder: (_) => RecordDeliveryScreen(controller: controller, stopId: stop.id)));
            },
            child: const Text("I've stopped safely"),
          ),
          const SizedBox(height: 10),
          OutlinedButton(
            onPressed: () => launchUrl(Uri.parse('https://www.google.com/maps/search/?api=1&query=${Uri.encodeComponent('${stop.outletName} ${stop.district}')}'), mode: LaunchMode.externalApplication),
            child: const Text('Open in maps'),
          ),
          const SizedBox(height: 12),
          const Text('Offline ready · route and stop details saved on this phone', style: TextStyle(fontSize: 12, color: Wp.muted)),
        ]);
      },
    );
  }
}

enum Outcome { delivered, partial, rejected, unavailable }

extension on Outcome {
  String get code => switch (this) { Outcome.delivered => 'DELIVERED', Outcome.partial => 'PARTIAL', Outcome.rejected => 'REFUSED', Outcome.unavailable => 'NOT_DELIVERED' };
  String get label => switch (this) { Outcome.delivered => 'Delivered', Outcome.partial => 'Partial', Outcome.rejected => 'Rejected', Outcome.unavailable => 'Unavailable' };
  String get hint => switch (this) { Outcome.delivered => 'Full quantity received', Outcome.partial => 'Record exact quantity', Outcome.rejected => 'Store declined goods', Outcome.unavailable => 'Store closed / no access' };
}

extension on String {
  String ifEmpty(String fallback) => isEmpty ? fallback : this;
}

class RecordDeliveryScreen extends StatefulWidget {
  const RecordDeliveryScreen({super.key, required this.controller, required this.stopId});

  final DriverController controller;
  final String stopId;

  @override
  State<RecordDeliveryScreen> createState() => _RecordDeliveryScreenState();
}

class _RecordDeliveryScreenState extends State<RecordDeliveryScreen> {
  Outcome? outcome;
  final qty = TextEditingController();
  final notes = TextEditingController();
  String error = '';
  bool hideParkTip = false;
  bool saving = false;

  @override
  void dispose() {
    qty.dispose();
    notes.dispose();
    super.dispose();
  }

  Future<void> _photo(DeliveryStop stop) async {
    try {
      final file = await ImagePicker().pickImage(source: ImageSource.camera, maxWidth: 1600, imageQuality: 80);
      if (file == null) return;
      await widget.controller.addProof(stop, bytes: await file.readAsBytes(), proofType: 'PHOTO');
    } catch (_) {
      setState(() => error = 'The camera is unavailable. Use name and signature instead.');
    }
  }

  Future<void> _signature(DeliveryStop stop) async {
    final result = await showWpSheet<(String, List<int>)>(context, title: 'Name and signature', subtitle: 'The store staff member signs on the screen.', builder: (ctx) => const _SignatureForm());
    if (result == null) return;
    await widget.controller.addProof(stop, bytes: result.$2, proofType: 'SIGNATURE', mimeType: 'image/png', receiverName: result.$1);
  }

  Future<void> _save(DeliveryStop stop) async {
    final c = widget.controller;
    final o = outcome;
    if (o == null) return setState(() => error = 'Choose a delivery outcome.');
    if ((o == Outcome.delivered || o == Outcome.partial) && c.proofIds[stop.id] == null) return setState(() => error = 'Add a photo or a name and signature before saving.');
    if (o == Outcome.partial && int.tryParse(qty.text.replaceAll(RegExp(r'[^0-9]'), '')) == null) return setState(() => error = 'Enter the quantity received.');
    if (o == Outcome.rejected) {
      final saved = await showTakeBackSheet(context, c, stop);
      if (saved != true || !mounted) return;
    } else {
      setState(() => saving = true);
      await c.recordOutcome(stop,
          code: o.code,
          reason: o == Outcome.unavailable ? 'OUTLET_CLOSED' : (o == Outcome.partial ? 'SHORT_RECEIVED' : ''),
          note: notes.text.trim(),
          receivedUnits: o == Outcome.partial ? int.tryParse(qty.text.replaceAll(RegExp(r'[^0-9]'), '')) : null);
    }
    if (!mounted) return;
    Navigator.of(context).pushReplacement(MaterialPageRoute(builder: (_) => SyncStatusScreen(controller: c, stopId: stop.id)));
  }

  @override
  Widget build(BuildContext context) {
    final c = widget.controller;
    return ListenableBuilder(
      listenable: Listenable.merge([c, c.sync]),
      builder: (context, _) {
        final trip = c.trip;
        final stop = trip?.stops.where((s) => s.id == widget.stopId).firstOrNull;
        if (trip == null || stop == null) return const Scaffold(body: Center(child: Text('Stop not found on this route.')));
        final offline = c.offline || c.sync.state.phase == SyncPhase.offline;
        final done = stop.done;
        return _StopScaffold(
          bottom: done ? null : FilledButton(onPressed: saving ? null : () => _save(stop), child: Text(offline ? 'Save on this device' : 'Save delivery')),
          children: [
            if (offline) const Padding(padding: EdgeInsets.only(bottom: 12), child: StatusNote(tone: Tone.amber, icon: Icons.wifi_off, title: 'No connection', text: 'You can still record this delivery.')),
            WpCard(
              color: const Color(0xFFDDE7FB),
              borderColor: const Color(0xFF9DB3EE),
              child: Row(children: [
                const Icon(Icons.location_on, size: 44, color: Wp.primary),
                const SizedBox(width: 12),
                Expanded(
                  child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                    Text('${stop.outletId} - ${stop.outletName}', style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 16)),
                    Text('${hhmm(stop.windowOpen)} - ${hhmm(stop.windowClose)}'),
                    Text('${stop.orderRef}${stop.expectedUnits != null ? ' · ${stop.expectedUnits} units' : ''}'),
                  ]),
                ),
              ]),
            ),
            const SizedBox(height: 12),
            WpCard(
              child: Row(children: [
                const Icon(Icons.store_mall_directory_outlined, size: 36, color: Wp.muted),
                const SizedBox(width: 12),
                Expanded(child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                  Text([stop.dockType, stop.parking].where((s) => s.isNotEmpty).join(' · ').ifEmpty('Outlet access'), style: const TextStyle(fontWeight: FontWeight.w600)),
                  Text(stop.access.ifEmpty('Call store on arrival'), style: const TextStyle(fontSize: 12, color: Wp.muted)),
                ])),
              ]),
            ),
            const SizedBox(height: 12),
            WpCard(
              child: Row(children: [
                Icon(stop.chilled ? Icons.ac_unit : Icons.inventory_2_outlined, size: 32, color: stop.chilled ? Wp.cool : const Color(0xFFB07A3B)),
                const SizedBox(width: 12),
                Expanded(child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                  Text(stop.chilled ? 'Chilled goods · keep at 2–8 °C' : 'Ambient goods', style: const TextStyle(fontWeight: FontWeight.w600)),
                  Text(stop.expectedUnits != null ? 'Expected: ${stop.expectedUnits}' : stop.brand, style: const TextStyle(fontSize: 12, color: Wp.muted)),
                  for (final sf in stop.shortfalls) Text('${sf['type']} · ${sf['affectedUnits']} units short at loading', style: const TextStyle(fontSize: 12, color: Wp.amber)),
                ])),
              ]),
            ),
            if (done) ...[
              const SizedBox(height: 16),
              StatusNote(tone: stop.delivered ? Tone.green : Tone.amber, title: 'Outcome recorded', text: '${stop.outcomeCode ?? 'Completed'}${(stop.outcomeReason ?? '').isNotEmpty ? ' · ${stop.outcomeReason}' : ''}'),
            ] else ...[
              const SectionTitle('Delivery outcome'),
              GridView.count(
                crossAxisCount: 2,
                shrinkWrap: true,
                physics: const NeverScrollableScrollPhysics(),
                mainAxisSpacing: 10,
                crossAxisSpacing: 10,
                childAspectRatio: 2.4,
                children: [
                  for (final o in Outcome.values)
                    Semantics(
                      button: true,
                      selected: outcome == o,
                      child: WpCard(
                        onTap: () => setState(() { outcome = o; error = ''; }),
                        padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
                        color: outcome == o ? Wp.tint : Wp.surface,
                        borderColor: outcome == o ? Wp.primary : Wp.border,
                        borderWidth: outcome == o ? 2 : 1,
                        child: Column(crossAxisAlignment: CrossAxisAlignment.start, mainAxisAlignment: MainAxisAlignment.center, children: [
                          Text(o.label, style: const TextStyle(fontWeight: FontWeight.w700)),
                          Text(o.hint, style: const TextStyle(fontSize: 11, color: Wp.muted)),
                        ]),
                      ),
                    ),
                ],
              ),
              if (outcome == Outcome.partial) ...[
                const SectionTitle('Quantity received'),
                TextField(controller: qty, keyboardType: TextInputType.number, decoration: InputDecoration(hintText: stop.expectedUnits != null ? 'of ${stop.expectedUnits} expected' : 'Units received')),
              ],
              const SectionTitle('Proof of delivery'),
              Row(children: [
                Expanded(child: OutlinedButton.icon(onPressed: () => _photo(stop), icon: Icon(c.photoSaved[stop.id] == true ? Icons.check_circle : Icons.photo_camera_outlined, color: c.photoSaved[stop.id] == true ? Wp.green : null), label: Text(c.photoSaved[stop.id] == true ? 'Photo saved' : 'Take photo'))),
                const SizedBox(width: 10),
                Expanded(child: OutlinedButton.icon(onPressed: () => _signature(stop), icon: const Icon(Icons.draw_outlined), label: const Text('Name / signature'))),
              ]),
              if (c.proofIds[stop.id] != null) const Padding(padding: EdgeInsets.only(top: 10), child: StatusNote(tone: Tone.cool, text: 'Saved locally first · uploads when connected')),
              const SectionTitle('Report an issue (optional)'),
              WpCard(
                color: Wp.redBg,
                borderColor: const Color(0xFFF1C4BD),
                onTap: () => showReportProblemSheet(context, c, stop),
                child: const Row(children: [Icon(Icons.warning_amber, color: Wp.red), SizedBox(width: 10), Expanded(child: Text('Report an issue')), Icon(Icons.chevron_right, color: Wp.red)]),
              ),
              const SectionTitle('Delivery notes'),
              TextField(controller: notes, maxLines: 3, maxLength: 500, decoration: const InputDecoration(hintText: 'Add notes (optional) ...')),
              if (error.isNotEmpty) Padding(padding: const EdgeInsets.only(top: 8), child: StatusNote(tone: Tone.red, text: error)),
              if (!hideParkTip) Padding(padding: const EdgeInsets.only(top: 12), child: StatusNote(icon: Icons.info_outline, text: 'Complete this form only while parked.', onDismiss: () => setState(() => hideParkTip = true))),
            ],
          ],
        );
      },
    );
  }
}

class _SignatureForm extends StatefulWidget {
  const _SignatureForm();

  @override
  State<_SignatureForm> createState() => _SignatureFormState();
}

class _SignatureFormState extends State<_SignatureForm> {
  final name = TextEditingController();
  final strokes = <List<Offset>>[];
  final key = GlobalKey();

  @override
  void dispose() {
    name.dispose();
    super.dispose();
  }

  Future<void> _done() async {
    if (name.text.trim().isEmpty || strokes.isEmpty) return;
    final box = key.currentContext!.findRenderObject() as RenderBox;
    final recorder = ui.PictureRecorder();
    final canvas = Canvas(recorder);
    canvas.drawColor(Colors.white, BlendMode.src);
    _SignaturePainter(strokes).paint(canvas, box.size);
    final image = await recorder.endRecording().toImage(box.size.width.round(), box.size.height.round());
    final bytes = await image.toByteData(format: ui.ImageByteFormat.png);
    if (!mounted || bytes == null) return;
    Navigator.of(context).pop((name.text.trim(), bytes.buffer.asUint8List().toList()));
  }

  @override
  Widget build(BuildContext context) {
    return Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
      TextField(controller: name, decoration: const InputDecoration(labelText: 'Receiver name'), onChanged: (_) => setState(() {})),
      const SizedBox(height: 12),
      Semantics(
        label: 'Signature drawing area',
        child: Container(
          key: key,
          height: 180,
          decoration: BoxDecoration(border: Border.all(color: Wp.border), borderRadius: BorderRadius.circular(Wp.radiusSm), color: Colors.white),
          child: GestureDetector(
            onPanStart: (d) => setState(() => strokes.add([d.localPosition])),
            onPanUpdate: (d) => setState(() => strokes.last.add(d.localPosition)),
            child: CustomPaint(painter: _SignaturePainter(strokes), size: Size.infinite),
          ),
        ),
      ),
      TextButton(onPressed: () => setState(strokes.clear), child: const Text('Clear signature')),
      FilledButton(onPressed: name.text.trim().isEmpty || strokes.isEmpty ? null : _done, child: const Text('Save signature')),
    ]);
  }
}

class _SignaturePainter extends CustomPainter {
  _SignaturePainter(this.strokes);
  final List<List<Offset>> strokes;

  @override
  void paint(Canvas canvas, Size size) {
    final paint = Paint()
      ..color = Wp.ink
      ..strokeWidth = 2.5
      ..strokeCap = StrokeCap.round
      ..style = PaintingStyle.stroke;
    for (final s in strokes) {
      final path = Path();
      for (var i = 0; i < s.length; i++) {
        i == 0 ? path.moveTo(s[i].dx, s[i].dy) : path.lineTo(s[i].dx, s[i].dy);
      }
      canvas.drawPath(path, paint);
    }
  }

  @override
  bool shouldRepaint(covariant _SignaturePainter oldDelegate) => true;
}
