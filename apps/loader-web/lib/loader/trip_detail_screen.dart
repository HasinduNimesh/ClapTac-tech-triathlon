// _toast() checks context.mounted before using the context after an await.
// ignore_for_file: use_build_context_synchronously

import 'dart:typed_data';

import 'package:flutter/material.dart';
import 'package:image_picker/image_picker.dart';

import '../shared/models.dart';
import '../theme/tokens.dart';
import '../widgets/common.dart';
import 'loader_controller.dart';
import 'loader_home.dart';

const _zones = ['Front', 'Front-middle', 'Middle', 'Middle', 'Rear', 'Doors'];
String _zone(int index, int total) {
  if (total <= 1) return 'Doors';
  final pos = (index / (total - 1) * (_zones.length - 1)).round();
  return _zones[pos.clamp(0, _zones.length - 1)];
}

bool _isTech(LoadingOrder o) => o.brand.toLowerCase() == 'tech';

String _pct(double v, double cap) => cap <= 0 ? '' : '  ·  ${(v / cap * 100).round()}%';

bool _chilledOk(double? t) => t != null && t >= 2 && t <= 4;

String _issueLine(LoadingOrder o, LoadingIssue i) => '${o.orderRef} · Stop ${o.stopSequence} ${o.outletId} · ${i.units} of ${o.expectedUnits} ${i.typeLabel}';

class TripDetailScreen extends StatelessWidget {
  const TripDetailScreen({super.key, required this.controller, required this.tripId, required this.profile, required this.onSignOut});

  final LoaderController controller;
  final String tripId;
  final Profile profile;
  final VoidCallback onSignOut;

  void _toast(BuildContext context, String? error, String ok) {
    if (!context.mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(error ?? ok), backgroundColor: error == null ? null : Wp.red));
  }

  @override
  Widget build(BuildContext context) {
    return ListenableBuilder(
      listenable: controller,
      builder: (context, _) => LayoutBuilder(builder: (context, box) {
        final tablet = box.maxWidth >= Wp.tabletBreakpoint;
        final trip = controller.details[tripId] ?? controller.trips.where((t) => t.tripId == tripId).firstOrNull;
        if (trip == null) return const Scaffold(body: Center(child: Text("Trip not found on this date's load list.")));
        final groups = controller.loadOrder(trip);
        final unresolved = trip.orders.where((o) => o.unresolved).toList();
        final waiting = unresolved.where((o) => o.issues.any((i) => i.decision == null)).toList();
        final onHold = unresolved.where((o) => o.issues.any((i) => i.decision == 'HOLD')).toList();
        final decided = trip.orders.where((o) => o.short && !o.unresolved).toList();
        final allHandled = groups.isNotEmpty && trip.orders.every((o) => o.loaded || (o.short && !o.unresolved));
        final check = controller.checkFor(trip.tripId);
        final checksDone = (!trip.refrigerated || _chilledOk(check.temperatureC)) && check.seal.isNotEmpty;
        final canReady = !trip.ready && trip.started && !trip.planChanged && !trip.needsAck && allHandled && unresolved.isEmpty && checksDone && !controller.busy && !controller.connectionLost;
        final sameVehicle = controller.vehicleTrips(trip);
        final tripOf = sameVehicle.length < trip.tripNumber ? trip.tripNumber : sameVehicle.length;

        final guidance = _Guidance(
          controller: controller,
          trip: trip,
          groups: groups,
          onLoadStop: (g) => _markStop(context, trip, g),
          onReport: (o) => _report(context, trip, o, tablet),
          onClear: (o, i) async => _toast(context, await controller.clearIssue(trip, o, i), 'Report withdrawn. Load the order or report it again.'),
        );
        final side = Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
          _Capacity(trip: trip),
          const SizedBox(height: 16),
          _ReadyCard(
            controller: controller,
            trip: trip,
            groups: groups,
            unresolved: unresolved.length,
            decided: decided.length,
            canReady: canReady,
            onReady: () => _confirmReady(context, trip, tablet),
            onAck: () async => _toast(context, await controller.acknowledgePlan(trip), 'Plan v${trip.planVersion} acknowledged.'),
            onTemperature: () => _recordTemperature(context, trip),
            onSeal: () => _recordSeal(context, trip),
          ),
        ]);
        final header = Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
          Wrap(crossAxisAlignment: WrapCrossAlignment.center, spacing: 8, runSpacing: 8, children: [
            TextButton.icon(onPressed: () => Navigator.pop(context), icon: const Icon(Icons.arrow_back, size: 18), label: const Text('Load list')),
            Text('/  ${trip.vehicleId} · Trip ${trip.tripNumber}', style: const TextStyle(color: Wp.muted)),
            OutlinedButton.icon(onPressed: () => _checkOrder(context, trip), icon: const Icon(Icons.qr_code_2, size: 18), label: const Text('Check an order number')),
            IconButton(onPressed: () => controller.refreshTrip(trip.tripId), icon: const Icon(Icons.refresh), tooltip: 'Refresh from server'),
          ]),
          const SizedBox(height: 8),
          Wrap(crossAxisAlignment: WrapCrossAlignment.end, spacing: 12, runSpacing: 8, children: [
            Text('${trip.vehicleId} · Trip ${trip.tripNumber}', style: TextStyle(fontSize: tablet ? 28 : 22, fontWeight: FontWeight.w700)),
            StatusTag(trip.refrigerated ? 'Chilled' : 'Ambient', tone: trip.refrigerated ? Tone.cool : Tone.muted),
            StatusTag('Trip ${trip.tripNumber} of $tripOf'),
            StatusTag('Manifest v${trip.planVersion}', tone: Tone.primary),
            if (trip.departAt != null) _DepartTag(trip.departAt!),
          ]),
          Text('${vehicleLine(trip)} · ${depotName(trip.depot)} · ${trip.planRef}', style: const TextStyle(color: Wp.muted)),
          const SizedBox(height: 16),
          ConnectionNote(controller: controller),
          if (trip.planChanged || trip.needsAck && trip.planVersion > 1)
            Padding(
              padding: const EdgeInsets.only(bottom: 12),
              child: ChangeBanner(controller: controller, trip: trip, compact: !tablet, onView: () => showChanges(context, trip)),
            ),
          if (trip.ready)
            StatusNote(
              tone: Tone.green,
              icon: Icons.check_circle,
              title: 'Ready to depart · logged as ${trip.readyBy.isEmpty ? (profile.displayName ?? profile.userId) : trip.readyBy} at ${clock(trip.readyAt)}',
              text: [
                if (trip.readyTemperatureC != null) 'Chilled zone ${trip.readyTemperatureC!.toStringAsFixed(1)} °C',
                if (trip.readySeal.isNotEmpty) 'seal ${trip.readySeal}',
                'The driver of ${trip.vehicleId} now sees the trip as ready.',
              ].join(' · '),
            )
          else if (unresolved.isNotEmpty)
            _ShortfallBanner(
              orders: unresolved,
              hold: onHold.isNotEmpty && waiting.isEmpty,
              onView: () => _showReports(context, trip, unresolved),
            )
          else if (decided.isNotEmpty)
            StatusNote(
              tone: Tone.green,
              icon: Icons.check_circle,
              title: 'Shortfalls resolved — the dispatcher decided',
              text: decided.expand((o) => o.issues.map((i) => '${o.orderRef} (Stop ${o.stopSequence} ${o.outletId}): ${i.decisionLabel.toLowerCase()}${i.decision == 'PARTIAL_LOAD' ? ', load ${o.expectedUnits - o.affectedUnits} of ${o.expectedUnits}' : ''}${i.decisionNote.isNotEmpty ? ' · ${i.decisionNote}' : ''}')).join('\n'),
            ),
          if (!trip.started && !trip.ready)
            Padding(
              padding: const EdgeInsets.only(top: 12),
              child: StatusNote(
                text: trip.needsAck ? 'Acknowledge plan v${trip.planVersion} first, then start loading.' : 'Loading has not started for this trip.',
                trailing: FilledButton(onPressed: controller.busy || trip.needsAck ? null : () async => _toast(context, await controller.startLoading(trip), 'Loading started.'), child: const Text('Start loading')),
              ),
            ),
          const SizedBox(height: 16),
        ]);

        if (tablet) {
          return Scaffold(
            body: Column(children: [
              LoaderHeader(controller: controller, profile: profile, onSignOut: onSignOut),
              if (controller.busy) const LinearProgressIndicator(minHeight: 2),
              Expanded(child: ListView(padding: const EdgeInsets.fromLTRB(28, 16, 28, 28), children: [
                header,
                Row(crossAxisAlignment: CrossAxisAlignment.start, children: [Expanded(child: guidance), const SizedBox(width: 20), SizedBox(width: 400, child: side)]),
              ])),
            ]),
          );
        }
        return Scaffold(
          body: ListView(padding: EdgeInsets.zero, children: [
            LoaderBanner(phone: true, title: '${trip.vehicleId} · Trip ${trip.tripNumber}', subtitle: '${trip.capability.isEmpty ? trip.vehicleType : trip.capability} · ${depotName(trip.depot)}${trip.departAt != null ? ' · departs ${clock(trip.departAt)}' : ''}'),
            if (controller.busy) const LinearProgressIndicator(minHeight: 2),
            Padding(padding: const EdgeInsets.all(16), child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [header, side, const SizedBox(height: 16), guidance])),
          ]),
          bottomNavigationBar: SafeArea(
            child: Container(
              decoration: const BoxDecoration(color: Wp.surface, border: Border(top: BorderSide(color: Wp.border))),
              padding: const EdgeInsets.fromLTRB(16, 12, 16, 12),
              child: Row(children: [
                OutlinedButton(
                  style: OutlinedButton.styleFrom(foregroundColor: Wp.red, side: const BorderSide(color: Wp.red)),
                  onPressed: trip.ready || !trip.orders.any((o) => !o.loaded && !o.short) ? null : () => _report(context, trip, trip.orders.firstWhere((o) => !o.loaded && !o.short), false),
                  child: const Text('Report issue'),
                ),
                const SizedBox(width: 10),
                Expanded(child: FilledButton(style: FilledButton.styleFrom(backgroundColor: trip.ready ? Wp.green : Wp.primary), onPressed: canReady ? () => _confirmReady(context, trip, false) : null, child: Text(trip.ready ? 'Ready' : 'Ready to depart'))),
              ]),
            ),
          ),
        );
      }),
    );
  }

  /// "Mark Stop N loaded". High-value Tech orders record custody first.
  Future<void> _markStop(BuildContext context, LoadingTrip trip, StopGroup stop) async {
    final custody = <String, TechCustody>{};
    for (final o in stop.orders.where((o) => !o.loaded && !o.short && _isTech(o))) {
      final c = await showDialog<TechCustody>(context: context, builder: (_) => _CustodyDialog(order: o));
      if (c == null) return;
      custody[o.orderId] = c;
    }
    final err = await controller.markStopLoaded(trip, stop, custody);
    _toast(context, err, 'Stop ${stop.stopSequence} loaded.');
  }

  Future<void> _recordTemperature(BuildContext context, LoadingTrip trip) async {
    final check = controller.checkFor(trip.tripId);
    final input = TextEditingController(text: check.temperatureC?.toStringAsFixed(1) ?? '');
    final value = await showDialog<double>(context: context, builder: (ctx) => StatefulBuilder(builder: (ctx, setState) {
      final v = double.tryParse(input.text.trim().replaceAll(',', '.'));
      final valid = v != null && v >= -30 && v <= 30;
      return AlertDialog(
        title: const Text('Chilled zone reading'),
        content: SizedBox(width: 420, child: Column(mainAxisSize: MainAxisSize.min, crossAxisAlignment: CrossAxisAlignment.stretch, children: [
          const Text('Read the reefer display with the doors closed. The chilled zone must be 2–4 °C before departure.', style: TextStyle(color: Wp.muted)),
          const SizedBox(height: 12),
          TextField(controller: input, autofocus: true, keyboardType: const TextInputType.numberWithOptions(decimal: true, signed: true), decoration: const InputDecoration(labelText: 'Temperature', suffixText: '°C'), onChanged: (_) => setState(() {})),
          if (valid && !_chilledOk(v)) const Padding(padding: EdgeInsets.only(top: 10), child: StatusNote(tone: Tone.red, text: 'Outside 2–4 °C. Do not dispatch — check the reefer unit and read it again.')),
        ])),
        actions: [
          OutlinedButton(onPressed: () => Navigator.pop(ctx), child: const Text('Cancel')),
          FilledButton(onPressed: valid ? () => Navigator.pop(ctx, v) : null, child: const Text('Save reading')),
        ],
      );
    }));
    if (value != null) {
      check.temperatureC = value;
      check.temperatureAt = DateTime.now();
      controller.select(trip.tripId);
    }
  }

  Future<void> _recordSeal(BuildContext context, LoadingTrip trip) async {
    final check = controller.checkFor(trip.tripId);
    final input = TextEditingController(text: check.seal);
    final value = await showDialog<String>(context: context, builder: (ctx) => StatefulBuilder(builder: (ctx, setState) {
      final v = input.text.trim();
      return AlertDialog(
        title: const Text('Doors closed & sealed'),
        content: SizedBox(width: 420, child: Column(mainAxisSize: MainAxisSize.min, crossAxisAlignment: CrossAxisAlignment.stretch, children: [
          const Text('Close the doors, fit the seal and type the number printed on it.', style: TextStyle(color: Wp.muted)),
          const SizedBox(height: 12),
          TextField(controller: input, autofocus: true, maxLength: 40, decoration: const InputDecoration(labelText: 'Seal number', hintText: 'e.g. WP-22817'), onChanged: (_) => setState(() {})),
        ])),
        actions: [
          OutlinedButton(onPressed: () => Navigator.pop(ctx), child: const Text('Cancel')),
          FilledButton(onPressed: v.isEmpty ? null : () => Navigator.pop(ctx, v), child: const Text('Save seal')),
        ],
      );
    }));
    if (value != null) {
      check.seal = value;
      controller.select(trip.tripId);
    }
  }

  Future<void> _confirmReady(BuildContext context, LoadingTrip trip, bool tablet) async {
    final groups = controller.stops(trip);
    final check = controller.checkFor(trip.tripId);
    final decided = trip.orders.where((o) => o.short && !o.unresolved).length;
    final content = Column(mainAxisSize: MainAxisSize.min, crossAxisAlignment: CrossAxisAlignment.stretch, children: [
      const CircleAvatar(radius: 24, backgroundColor: Wp.green, child: Icon(Icons.check, color: Colors.white, size: 30)),
      const SizedBox(height: 12),
      Text('Confirm ${trip.vehicleId} is ready to depart?', style: const TextStyle(fontSize: 20, fontWeight: FontWeight.w700)),
      Text('This tells the dispatcher and the driver the trip is loaded against plan v${trip.planVersion}.', style: const TextStyle(color: Wp.muted)),
      const SizedBox(height: 12),
      Container(
        padding: const EdgeInsets.all(12),
        decoration: BoxDecoration(color: Wp.canvas, borderRadius: BorderRadius.circular(8)),
        child: Column(children: [
          KvRow('Manifest', 'Plan v${trip.planVersion} · ${groups.length} stops · ${trip.orders.length} lines'),
          if (trip.weightKg > 0) KvRow('Load', '${kg(trip.weightKg)}${trip.weightCap > 0 ? ' (${(trip.weightKg / trip.weightCap * 100).round()}%)' : ''} · ${m3(trip.volumeM3)}${trip.volumeCap > 0 ? ' (${(trip.volumeM3 / trip.volumeCap * 100).round()}%)' : ''}'),
          KvRow('Shortfalls', decided > 0 ? '$decided resolved by dispatcher · 0 open' : 'None'),
          if (trip.refrigerated && check.temperatureC != null) KvRow('Chilled zone', '${check.temperatureC!.toStringAsFixed(1)} °C · checked ${clock(check.temperatureAt)}'),
          KvRow('Seal', '${check.seal}${trip.departAt != null ? ' · departs ${clock(trip.departAt)}' : ''}'),
        ]),
      ),
      const SizedBox(height: 8),
      const Text('After confirming, any change to this load needs dispatcher approval.', style: TextStyle(color: Wp.amber, fontSize: 13)),
    ]);
    bool? ok;
    if (tablet) {
      ok = await showDialog<bool>(context: context, builder: (ctx) => AlertDialog(content: SizedBox(width: 460, child: content), actions: [
        OutlinedButton(onPressed: () => Navigator.pop(ctx, false), child: const Text('Go back')),
        FilledButton(style: FilledButton.styleFrom(backgroundColor: Wp.green), onPressed: () => Navigator.pop(ctx, true), child: const Text('Confirm ready')),
      ]));
    } else {
      ok = await showWpSheet<bool>(context, title: 'Ready to depart', builder: (ctx) => Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
        content,
        const SizedBox(height: 16),
        Row(children: [Expanded(child: OutlinedButton(onPressed: () => Navigator.pop(ctx, false), child: const Text('Go back'))), const SizedBox(width: 10), Expanded(child: FilledButton(style: FilledButton.styleFrom(backgroundColor: Wp.green), onPressed: () => Navigator.pop(ctx, true), child: const Text('Confirm ready')))]),
      ]));
    }
    if (ok == true && context.mounted) _toast(context, await controller.confirmReady(trip), '${trip.vehicleId} is ready to depart.');
  }

  Future<void> _checkOrder(BuildContext context, LoadingTrip trip) async {
    final input = TextEditingController();
    final note = TextEditingController();
    // LD-6: time the entry so a scanner burst is told apart from typing; the loader can still correct it.
    final typing = Stopwatch();
    var burst = Duration.zero; // first keystroke to the latest one
    bool? typedChoice;
    String reason = '';
    await showDialog<void>(context: context, builder: (ctx) => StatefulBuilder(builder: (ctx, setState) {
      final code = input.text.trim();
      final typed = typedChoice ?? (code.isNotEmpty && !LoadEntry.looksScanned(code, burst));
      final entry = typed ? LoadEntry.manual(reason, note: note.text) : const LoadEntry.scan();
      final match = controller.findOrder(code);
      final wrong = match != null && match.$1.tripId != trip.tripId;
      final canLoad = match != null && !wrong && !match.$2.loaded && !match.$2.short && trip.started;
      Future<void> tell() async {
        final err = await controller.tellDispatcher(trip, match?.$2.orderRef ?? code, wrong ? match.$1.vehicleId : '');
        if (ctx.mounted) Navigator.pop(ctx);
        _toast(context, err, 'Dispatcher told: ${match?.$2.orderRef ?? code} is at ${trip.vehicleId}.');
      }

      return AlertDialog(
        title: const Text('Check an order number'),
        content: SizedBox(width: 480, child: Column(mainAxisSize: MainAxisSize.min, crossAxisAlignment: CrossAxisAlignment.stretch, children: [
          TextField(controller: input, autofocus: true, decoration: const InputDecoration(hintText: 'Scan the label, or type the order number, e.g. FR-4702'), onChanged: (v) => setState(() {
                if (v.trim().isEmpty) {
                  typing..stop()..reset();
                  burst = Duration.zero;
                  typedChoice = null;
                } else if (!typing.isRunning) {
                  typing.start();
                } else {
                  burst = typing.elapsed;
                }
              })),
          const SizedBox(height: 12),
          if (code.isNotEmpty && match == null) const StatusNote(tone: Tone.amber, text: 'This order is not on any load list at your depot for this date. Keep it off the truck and tell the dispatcher.'),
          if (wrong) ...[
            StatusNote(tone: Tone.red, icon: Icons.block, title: 'Stop. ${match.$2.orderRef} belongs on ${match.$1.vehicleId} (Trip ${match.$1.tripNumber}), not this vehicle ${trip.vehicleId}.', text: '${match.$2.orderRef} · ${match.$2.outletId} · ${match.$1.orders.where((o) => o.orderId == match.$2.orderId).length} line(s)${match.$2.chilled ? ' · chilled' : ''}. Put it back in the ${match.$1.vehicleId} staging lane.'),
          ],
          if (match != null && !wrong) StatusNote(tone: Tone.green, text: '${match.$2.orderRef} belongs on this vehicle · Stop ${match.$2.stopSequence}${match.$2.chilled ? ' · chilled zone' : ''}${match.$2.loaded ? ' · already loaded' : ''}.'),
          if (match != null && !wrong && match.$2.short) const Padding(padding: EdgeInsets.only(top: 8), child: StatusNote(tone: Tone.amber, text: 'This order has a reported shortfall.')),
          if (match != null && !wrong && !trip.started) const Padding(padding: EdgeInsets.only(top: 8), child: StatusNote(text: 'Start loading this trip first.')),
          if (canLoad) ...[
            const SizedBox(height: 14),
            const Text('How did you enter this number?', style: TextStyle(fontWeight: FontWeight.w600)),
            const SizedBox(height: 6),
            Wrap(spacing: 8, children: [
              ChoiceChip(label: const Text('Scanned the label'), selected: !typed, onSelected: (_) => setState(() => typedChoice = false)),
              ChoiceChip(label: const Text('Typed it in'), selected: typed, onSelected: (_) => setState(() => typedChoice = true)),
            ]),
            if (typed) ...[
              const SizedBox(height: 10),
              const Text('Why could the label not be scanned?'),
              const SizedBox(height: 6),
              Wrap(spacing: 8, runSpacing: 6, children: [
                for (final r in LoadEntry.manualReasons.entries)
                  ChoiceChip(label: Text(r.value), selected: reason == r.key, onSelected: (_) => setState(() => reason = r.key)),
              ]),
              const SizedBox(height: 8),
              TextField(controller: note, maxLength: 200, decoration: const InputDecoration(hintText: 'Note for the dispatcher (optional)')),
              if (!entry.complete) const StatusNote(tone: Tone.amber, text: 'Choose a reason before marking a typed order number loaded.'),
            ],
          ],
        ])),
        actions: [
          if (wrong || code.isNotEmpty && match == null) OutlinedButton(onPressed: controller.busy ? null : tell, child: const Text('Tell dispatcher')),
          if (wrong) FilledButton(onPressed: () => Navigator.pop(ctx), child: const Text('Put back, not on this truck')),
          if (canLoad) FilledButton(onPressed: entry.complete ? () { Navigator.pop(ctx); _markOne(context, trip, match.$2, entry: entry); } : null, child: const Text('Mark loaded')),
          if (!wrong) OutlinedButton(onPressed: () => Navigator.pop(ctx), child: const Text('Close')),
        ],
      );
    }));
  }

  Future<void> _markOne(BuildContext context, LoadingTrip trip, LoadingOrder order, {LoadEntry? entry}) async {
    TechCustody? custody;
    if (_isTech(order)) {
      custody = await showDialog<TechCustody>(context: context, builder: (_) => _CustodyDialog(order: order));
      if (custody == null) return;
    }
    _toast(context, await controller.markLoaded(trip, order, custody: custody, entry: entry), '${order.orderRef} loaded${entry?.manual == true ? ' · typed, reason recorded' : ''}.');
  }

  Future<void> _report(BuildContext context, LoadingTrip trip, LoadingOrder order, bool tablet) async {
    final Future<bool?> pending = tablet
        ? showGeneralDialog<bool>(
            context: context,
            barrierDismissible: true,
            barrierLabel: 'Close',
            barrierColor: Wp.ink.withValues(alpha: 0.45),
            pageBuilder: (ctx, _, __) => Align(
              alignment: Alignment.centerRight,
              child: Material(color: Wp.surface, child: SizedBox(width: 480, height: double.infinity, child: SafeArea(child: Padding(padding: const EdgeInsets.all(24), child: _ReportForm(controller: controller, trip: trip, initial: order))))),
            ),
          )
        : showWpSheet<bool>(context, title: 'Report an issue', subtitle: '${trip.vehicleId} · Trip ${trip.tripNumber} · Stop ${order.stopSequence} ${order.outletId}', builder: (ctx) => _ReportForm(controller: controller, trip: trip, initial: order, compact: true));
    final result = await pending;
    if (result == true && context.mounted) {
      final fresh = controller.details[trip.tripId] ?? trip;
      await _showReports(context, fresh, fresh.orders.where((o) => o.unresolved).toList());
    }
  }

  /// "Sent to dispatcher · waiting for a decision" for the trip's open reports.
  Future<void> _showReports(BuildContext context, LoadingTrip trip, List<LoadingOrder> orders) {
    return showDialog<void>(context: context, builder: (ctx) => ListenableBuilder(
      listenable: controller,
      builder: (ctx, _) {
        final t = controller.details[trip.tripId] ?? trip;
        final ids = orders.map((o) => o.orderId).toSet();
        final rows = [for (final o in t.orders.where((o) => ids.contains(o.orderId))) for (final i in o.issues) (o, i)];
        final allDecided = rows.isNotEmpty && rows.every((r) => r.$2.decision != null);
        final seen = rows.map((r) => r.$2.seenAt).whereType<DateTime>().fold<DateTime?>(null, (a, b) => a == null || b.isBefore(a) ? b : a);
        final seenBy = rows.map((r) => r.$2.seenBy).whereType<String>().firstOrNull;
        final left = t.departAt == null ? null : minutesUntil(t.departAt!);
        return AlertDialog(
          title: Text(allDecided ? 'Dispatcher decision received' : 'Sent to dispatcher · waiting for a decision'),
          content: SizedBox(width: 500, child: Column(mainAxisSize: MainAxisSize.min, crossAxisAlignment: CrossAxisAlignment.stretch, children: [
            for (final r in rows)
              Padding(padding: const EdgeInsets.only(bottom: 8), child: Row(crossAxisAlignment: CrossAxisAlignment.start, children: [
                Icon(r.$2.hasPhoto ? Icons.photo_camera : Icons.report, size: 20, color: r.$2.allowsDeparture ? Wp.green : Wp.red),
                const SizedBox(width: 8),
                Expanded(child: Text('${_issueLine(r.$1, r.$2)} · reported ${clock(r.$2.reportedAt)} by ${r.$2.reportedBy}${r.$2.hasPhoto ? ' · photo attached' : ''}')),
              ])),
            if (!allDecided) const StatusNote(tone: Tone.amber, text: 'Ready to depart stays locked until the dispatcher decides. Keep loading the other stops.'),
            const SizedBox(height: 8),
            KvRow('Dispatcher', seen == null ? 'Not seen yet' : '${seenBy ?? 'Dispatcher'} · seen ${clock(seen)}'),
            if (t.departAt != null) KvRow('Departure', '${clock(t.departAt)}${left != null && left > 0 ? ' · $left min left' : ''}'),
            for (final r in rows.where((r) => r.$2.decision != null))
              Padding(padding: const EdgeInsets.only(top: 8), child: StatusNote(
                tone: r.$2.allowsDeparture ? Tone.green : Tone.red,
                text: 'Decision at ${clock(r.$2.decidedAt)}: ${r.$2.decisionLabel.toLowerCase()}${r.$2.decision == 'PARTIAL_LOAD' ? '. Load ${r.$1.expectedUnits - r.$1.affectedUnits} of ${r.$1.expectedUnits} and continue' : r.$2.decision == 'HOLD' ? '. Wait for the stock, then withdraw the report and load it' : '. Leave it off this truck'}${r.$2.decisionNote.isNotEmpty ? ' · ${r.$2.decisionNote}' : ''}.',
              )),
          ])),
          actions: [
            IconButton(onPressed: () => controller.refreshTrip(trip.tripId), icon: const Icon(Icons.refresh), tooltip: 'Check for a decision'),
            FilledButton(onPressed: () => Navigator.pop(ctx), child: const Text('Back to trip')),
          ],
        );
      },
    ));
  }
}

class _DepartTag extends StatelessWidget {
  const _DepartTag(this.at);
  final DateTime at;

  @override
  Widget build(BuildContext context) {
    final left = minutesUntil(at);
    return StatusTag('Departs ${clock(at)}${left > 0 && left < 600 ? ' · in $left min' : ''}', tone: left > 0 && left <= 30 ? Tone.amber : Tone.muted, icon: Icons.schedule);
  }
}

class _ShortfallBanner extends StatelessWidget {
  const _ShortfallBanner({required this.orders, required this.hold, required this.onView});
  final List<LoadingOrder> orders;
  final bool hold;
  final VoidCallback onView;

  @override
  Widget build(BuildContext context) {
    final lines = [for (final o in orders) for (final i in o.issues) _issueLine(o, i)];
    return StatusNote(
      tone: hold ? Tone.red : Tone.amber,
      icon: hold ? Icons.pause_circle : Icons.hourglass_top,
      title: hold ? 'Dispatcher: hold the trip until stock arrives' : '${orders.length} unresolved shortfall${orders.length == 1 ? '' : 's'} on this trip',
      text: '${lines.join('\n')}\n${hold ? 'When the stock arrives, withdraw the report and load the order.' : 'Waiting for the dispatcher. Ready to depart stays locked until they decide.'}',
      trailing: OutlinedButton(onPressed: onView, child: const Text('View reports')),
    );
  }
}

class _Guidance extends StatelessWidget {
  const _Guidance({required this.controller, required this.trip, required this.groups, required this.onLoadStop, required this.onReport, required this.onClear});
  final LoaderController controller;
  final LoadingTrip trip;
  final List<StopGroup> groups;
  final ValueChanged<StopGroup> onLoadStop;
  final ValueChanged<LoadingOrder> onReport;
  final void Function(LoadingOrder, LoadingIssue) onClear;

  @override
  Widget build(BuildContext context) {
    final current = groups.indexWhere((g) => !g.loaded);
    final canAct = trip.started && !trip.ready && !trip.planChanged && !trip.needsAck && !controller.busy && !controller.connectionLost;
    return WpCard(
      padding: EdgeInsets.zero,
      child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
        Padding(
          padding: const EdgeInsets.all(16),
          child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
            Row(children: [const Expanded(child: Text('Stop-by-stop load guidance', style: TextStyle(fontSize: 18, fontWeight: FontWeight.w600))), StatusTag('${groups.where((g) => g.loaded).length} of ${groups.length} stops loaded', tone: groups.every((g) => g.loaded) ? Tone.green : Tone.primary)]),
            const SizedBox(height: 4),
            const Text('Load in reverse drop-off order. First in goes to the front of the cargo; Stop 1 sits by the doors.', style: TextStyle(fontSize: 13, color: Wp.muted)),
          ]),
        ),
        for (var i = 0; i < groups.length; i++) _stop(context, i, groups[i], i == current, canAct),
      ]),
    );
  }

  Widget _stop(BuildContext context, int i, StopGroup g, bool current, bool canAct) {
    final pending = g.orders.where((o) => !o.loaded && !o.short).toList();
    final expanded = current || g.hasShortfall || g.changeNote.isNotEmpty && !g.loaded;
    return Container(
      decoration: BoxDecoration(color: current ? Wp.tint : null, border: const Border(top: BorderSide(color: Wp.border))),
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
      child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
        Row(crossAxisAlignment: CrossAxisAlignment.start, children: [
          CircleAvatar(
            radius: 13,
            backgroundColor: g.loaded ? Wp.green : current ? Wp.primary : Wp.surface,
            child: g.loaded ? const Icon(Icons.check, size: 16, color: Colors.white) : Text('${i + 1}', style: TextStyle(fontSize: 12, fontWeight: FontWeight.w700, color: current ? Colors.white : Wp.muted)),
          ),
          const SizedBox(width: 12),
          Expanded(child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
            Text('Load ${i + 1} → Stop ${g.stopSequence} · ${g.outletLabel}', style: const TextStyle(fontWeight: FontWeight.w600, fontSize: 15)),
            const SizedBox(height: 4),
            Wrap(spacing: 6, runSpacing: 4, children: [
              StatusTag(_zone(i, groups.length) + (g.chilled ? ' · chilled zone' : ''), tone: Tone.primary),
              if (g.head.dock.isNotEmpty) StatusTag(g.head.dock),
              if (g.changeNote.isNotEmpty) StatusTag('${g.changeNote}${g.changedInVersion > 0 && !g.changeNote.contains('v${g.changedInVersion}') ? ' in v${g.changedInVersion}' : ''}', tone: Tone.amber),
              if (g.hasShortfall) const StatusTag('Shortfall reported', tone: Tone.amber),
            ]),
          ])),
          Column(crossAxisAlignment: CrossAxisAlignment.end, children: [
            Text('${g.orders.length} line${g.orders.length == 1 ? '' : 's'}${g.weightKg > 0 ? ' · ${kg(g.weightKg)} · ${m3(g.volumeM3)}' : ' · ${g.expectedUnits} units'}', style: const TextStyle(fontSize: 12, color: Wp.muted)),
            Text([g.outletId, g.head.window].where((s) => s.isNotEmpty).join(' · '), style: const TextStyle(fontSize: 12, color: Wp.muted)),
          ]),
        ]),
        if (expanded) ...[
          const SizedBox(height: 10),
          for (final o in g.orders) _line(o, canAct),
          if (pending.isNotEmpty && canAct)
            Padding(
              padding: const EdgeInsets.only(top: 4),
              child: Wrap(spacing: 10, runSpacing: 8, alignment: WrapAlignment.end, children: [
                OutlinedButton(style: OutlinedButton.styleFrom(foregroundColor: Wp.red, side: const BorderSide(color: Wp.red)), onPressed: () => onReport(pending.first), child: const Text('Report missing / damaged')),
                FilledButton(onPressed: () => onLoadStop(g), child: Text('Mark Stop ${g.stopSequence} loaded')),
              ]),
            ),
        ],
      ]),
    );
  }

  Widget _line(LoadingOrder o, bool canAct) {
    return Container(
      margin: const EdgeInsets.only(bottom: 6),
      padding: const EdgeInsets.all(10),
      decoration: BoxDecoration(color: o.unresolved ? Wp.redBg : Wp.surface, borderRadius: BorderRadius.circular(8), border: Border.all(color: o.unresolved ? const Color(0xFFF1C4BD) : Wp.border)),
      child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
        Row(children: [
          Icon(o.loaded ? Icons.check_box : Icons.check_box_outline_blank, color: o.loaded ? Wp.primary : o.unresolved ? Wp.red : Wp.border),
          const SizedBox(width: 10),
          Expanded(child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
            Wrap(spacing: 6, crossAxisAlignment: WrapCrossAlignment.center, children: [
              Text(o.orderRef, style: const TextStyle(fontWeight: FontWeight.w600)),
              if (o.chilled) const StatusTag('Chilled', tone: Tone.cool),
              if (_isTech(o)) const StatusTag('Tech · custody check', tone: Tone.primary),
              if (o.changeNote.isNotEmpty) StatusTag(o.changeNote, tone: Tone.amber),
            ]),
            Text([o.brand, o.temperature, 'ordered ${o.expectedUnits}'].where((s) => s.isNotEmpty).join(' · '), style: const TextStyle(fontSize: 12, color: Wp.muted)),
          ])),
          Text('${o.loadedUnits} / ${o.expectedUnits}', style: TextStyle(fontWeight: FontWeight.w700, color: o.unresolved ? Wp.red : Wp.ink)),
          if (o.loaded) const Padding(padding: EdgeInsets.only(left: 8), child: StatusTag('Loaded', tone: Tone.green)),
          if (!o.loaded && !o.short && canAct) TextButton(onPressed: () => onReport(o), child: const Text('Report issue')),
        ]),
        for (final iss in o.issues)
          Padding(
            padding: const EdgeInsets.only(left: 34, top: 4),
            child: Row(children: [
              Expanded(child: Text('${iss.units} ${iss.typeLabel} · ${iss.decision == null ? 'awaiting dispatcher' : iss.decisionLabel.toLowerCase()}${iss.hasPhoto ? ' · photo' : ''}${iss.decisionNote.isNotEmpty ? ' · ${iss.decisionNote}' : ''}${iss.note.isNotEmpty ? ' · ${iss.note}' : ''}', style: TextStyle(fontSize: 12, color: iss.allowsDeparture ? Wp.green : Wp.red, fontWeight: FontWeight.w600))),
              if (canAct && (iss.decision == null || iss.decision == 'HOLD')) TextButton(onPressed: () => onClear(o, iss), child: const Text('Withdraw report')),
            ]),
          ),
      ]),
    );
  }
}

class _Capacity extends StatelessWidget {
  const _Capacity({required this.trip});
  final LoadingTrip trip;

  @override
  Widget build(BuildContext context) {
    final wf = trip.weightCap > 0 ? trip.weightKg / trip.weightCap : 0.0;
    final vf = trip.volumeCap > 0 ? trip.volumeM3 / trip.volumeCap : 0.0;
    final cf = trip.volumeCap > 0 ? trip.chilledM3 / trip.volumeCap : 0.0;
    Color tone(double f) => f > 1 ? Wp.red : f >= 0.9 ? Wp.amber : Wp.primary;
    final minutes = trip.tripMinutes;
    final budget = trip.freshBudget;
    return WpCard(
      child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
        Row(children: [const Expanded(child: Text('Capacity summary', style: TextStyle(fontSize: 18, fontWeight: FontWeight.w600))), StatusTag('Plan v${trip.planVersion}', tone: Tone.primary)]),
        const SizedBox(height: 12),
        Meter(label: 'Weight', valueText: trip.weightCap > 0 ? '${kg(trip.weightKg)} / ${kg(trip.weightCap)}${_pct(trip.weightKg, trip.weightCap)}' : kg(trip.weightKg), fraction: wf.clamp(0, 1), color: tone(wf), tag: wf > vf && wf > 0 ? const StatusTag('Closest to full', tone: Tone.amber) : null),
        const SizedBox(height: 12),
        Meter(label: 'Volume', valueText: trip.volumeCap > 0 ? '${m3(trip.volumeM3)} / ${m3(trip.volumeCap)}${_pct(trip.volumeM3, trip.volumeCap)}' : m3(trip.volumeM3), fraction: vf.clamp(0, 1), color: tone(vf), tag: vf >= wf && vf > 0 ? const StatusTag('Closest to full', tone: Tone.amber) : null),
        if (trip.refrigerated) ...[
          const SizedBox(height: 12),
          Meter(label: 'Chilled zone', valueText: trip.volumeCap > 0 ? '${m3(trip.chilledM3)} / ${m3(trip.volumeCap)}${_pct(trip.chilledM3, trip.volumeCap)}' : m3(trip.chilledM3), fraction: cf.clamp(0, 1), color: Wp.cool),
        ],
        if (minutes != null && budget != null) ...[
          const SizedBox(height: 12),
          Meter(label: 'Fresh trip time', valueText: '$minutes / $budget min${_pct(minutes.toDouble(), budget.toDouble())}', fraction: (minutes / budget).clamp(0, 1), color: minutes > budget ? Wp.red : Wp.green),
        ],
        const SizedBox(height: 10),
        Text('Totals match manifest plan v${trip.planVersion} agreed by the dispatcher.', style: const TextStyle(fontSize: 12, color: Wp.muted)),
      ]),
    );
  }
}

class _ReadyCard extends StatelessWidget {
  const _ReadyCard({required this.controller, required this.trip, required this.groups, required this.unresolved, required this.decided, required this.canReady, required this.onReady, required this.onAck, required this.onTemperature, required this.onSeal});
  final LoaderController controller;
  final LoadingTrip trip;
  final List<StopGroup> groups;
  final int unresolved;
  final int decided;
  final bool canReady;
  final VoidCallback onReady;
  final VoidCallback onAck;
  final VoidCallback onTemperature;
  final VoidCallback onSeal;

  Widget _row(String state, String label, {Widget? action}) => Row(children: [Expanded(child: CheckRow(state: state, label: label)), if (action != null) action]);

  @override
  Widget build(BuildContext context) {
    final ready = trip.ready;
    final check = controller.checkFor(trip.tripId);
    final loadedStops = groups.where((g) => g.loaded).length;
    final editable = !ready && trip.started && !controller.busy;
    final temp = ready ? trip.readyTemperatureC : check.temperatureC;
    final seal = ready ? trip.readySeal : check.seal;
    final acked = !trip.needsAck && !trip.planChanged;
    return WpCard(
      borderColor: ready ? Wp.green : Wp.border,
      borderWidth: ready ? 2 : 1,
      child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
        const Text('Ready to depart', style: TextStyle(fontSize: 18, fontWeight: FontWeight.w600)),
        const SizedBox(height: 8),
        _row(acked ? 'ok' : 'bad', acked ? 'Plan v${trip.planVersion} acknowledged${trip.acknowledgedAt != null ? ' · ${clock(trip.acknowledgedAt)}' : ''}' : 'Plan v${trip.planVersion} not acknowledged yet',
            action: acked || ready ? null : TextButton(onPressed: controller.busy ? null : onAck, child: const Text('Acknowledge'))),
        _row(!trip.started ? 'todo' : loadedStops == groups.length ? 'ok' : 'now', loadedStops == groups.length && groups.isNotEmpty ? 'All ${groups.length} stops loaded and counted' : 'Stops loaded · $loadedStops of ${groups.length}'),
        _row(unresolved > 0 ? 'bad' : 'ok', unresolved > 0 ? '$unresolved shortfall${unresolved == 1 ? '' : 's'} unresolved' : decided > 0 ? '$decided shortfall${decided == 1 ? '' : 's'} resolved by dispatcher' : 'No shortfalls'),
        if (trip.refrigerated)
          _row(temp == null ? 'todo' : _chilledOk(temp) ? 'ok' : 'bad', temp == null ? 'Chilled zone at 2–4 °C' : 'Chilled zone checked · ${temp.toStringAsFixed(1)} °C', action: editable ? TextButton(onPressed: onTemperature, child: Text(temp == null ? 'Record' : 'Change')) : null),
        _row(seal.isEmpty ? 'todo' : 'ok', seal.isEmpty ? 'Doors closed & sealed' : 'Doors closed · seal $seal', action: editable ? TextButton(onPressed: onSeal, child: Text(seal.isEmpty ? 'Record seal' : 'Change')) : null),
        const SizedBox(height: 12),
        FilledButton(style: FilledButton.styleFrom(backgroundColor: ready ? Wp.green : Wp.primary), onPressed: canReady ? onReady : null, child: Text(ready ? 'Ready · confirmed ${clock(trip.readyAt)}' : 'Confirm ready to depart')),
        const SizedBox(height: 8),
        Text(
          ready
              ? 'Logged as ${trip.readyBy}. The driver of ${trip.vehicleId} sees this trip as ready.'
              : unresolved > 0
                  ? 'Finish loading and resolve $unresolved shortfall${unresolved == 1 ? '' : 's'} before departure.'
                  : 'The driver of ${trip.vehicleId} will see this trip as ready once you confirm.',
          style: TextStyle(fontSize: 12, color: ready ? Wp.muted : unresolved > 0 ? Wp.red : Wp.muted),
        ),
      ]),
    );
  }
}

class _CustodyDialog extends StatefulWidget {
  const _CustodyDialog({required this.order});
  final LoadingOrder order;

  @override
  State<_CustodyDialog> createState() => _CustodyDialogState();
}

class _CustodyDialogState extends State<_CustodyDialog> {
  final seal = TextEditingController();
  final serials = TextEditingController();
  final condition = TextEditingController(text: 'Sealed, no visible damage');
  final evidence = TextEditingController();

  @override
  void dispose() {
    for (final c in [seal, serials, condition, evidence]) {
      c.dispose();
    }
    super.dispose();
  }

  List<String> get _serials => serials.text.split(RegExp(r'[\n,;]')).map((s) => s.trim()).where((s) => s.isNotEmpty).toList();

  @override
  Widget build(BuildContext context) {
    final ok = seal.text.trim().isNotEmpty && _serials.isNotEmpty && condition.text.trim().isNotEmpty;
    return AlertDialog(
      title: Text('Tech custody · ${widget.order.orderRef}'),
      content: SizedBox(
        width: 460,
        child: Column(mainAxisSize: MainAxisSize.min, crossAxisAlignment: CrossAxisAlignment.stretch, children: [
          const Text('High-value Tech goods need a seal and serial numbers before they are loaded.', style: TextStyle(color: Wp.muted)),
          const SizedBox(height: 12),
          TextField(controller: seal, decoration: const InputDecoration(labelText: 'Seal ID'), onChanged: (_) => setState(() {})),
          const SizedBox(height: 10),
          TextField(controller: serials, maxLines: 3, decoration: const InputDecoration(labelText: 'Serial number(s), comma separated'), onChanged: (_) => setState(() {})),
          const SizedBox(height: 10),
          TextField(controller: condition, decoration: const InputDecoration(labelText: 'Loading condition'), onChanged: (_) => setState(() {})),
          const SizedBox(height: 10),
          TextField(controller: evidence, decoration: const InputDecoration(labelText: 'Evidence reference (optional)')),
        ]),
      ),
      actions: [
        OutlinedButton(onPressed: () => Navigator.pop(context), child: const Text('Cancel')),
        FilledButton(onPressed: ok ? () => Navigator.pop(context, TechCustody(sealId: seal.text.trim(), serials: _serials, condition: condition.text.trim(), evidenceRef: evidence.text.trim())) : null, child: const Text('Record custody & load')),
      ],
    );
  }
}

class _ReportForm extends StatefulWidget {
  const _ReportForm({required this.controller, required this.trip, required this.initial, this.compact = false});
  final LoaderController controller;
  final LoadingTrip trip;
  final LoadingOrder initial;
  final bool compact;

  @override
  State<_ReportForm> createState() => _ReportFormState();
}

class _ReportFormState extends State<_ReportForm> {
  late LoadingOrder order = widget.initial;
  final attempt = ReportAttempt();
  String type = 'MISSING';
  int units = 1;
  String error = '';
  bool sending = false;
  Uint8List? photo;
  String photoMime = 'image/jpeg';
  final note = TextEditingController();

  @override
  void dispose() {
    note.dispose();
    super.dispose();
  }

  bool get _photoRequired => type == 'DAMAGED';

  Future<void> _pickPhoto() async {
    try {
      final file = await ImagePicker().pickImage(source: ImageSource.camera, maxWidth: 1600, imageQuality: 80);
      if (file == null) return;
      final bytes = await file.readAsBytes();
      final png = bytes.length > 4 && bytes[0] == 0x89 && bytes[1] == 0x50;
      setState(() {
        photo = bytes;
        photoMime = png ? 'image/png' : 'image/jpeg';
        error = '';
      });
    } catch (_) {
      setState(() => error = 'Could not open the camera. Allow camera access for this site, or choose a photo file.');
    }
  }

  @override
  Widget build(BuildContext context) {
    // Once the report has been filed the form can only finish it (the photo), not change it:
    // the server already holds these details, and a changed report would look like a new one.
    final locked = attempt.filed;
    final candidates = widget.trip.orders.where((o) => !o.loaded && (!o.short || o.orderId == order.orderId && locked)).toList();
    final selectedId = candidates.any((o) => o.orderId == order.orderId) ? order.orderId : (candidates.isEmpty ? null : candidates.first.orderId);
    final form = Column(mainAxisSize: MainAxisSize.min, crossAxisAlignment: CrossAxisAlignment.stretch, children: [
      if (!widget.compact) ...[
        Row(children: [
          const Expanded(child: Text('Report missing or damaged item', style: TextStyle(fontSize: 22, fontWeight: FontWeight.w700))),
          IconButton(onPressed: () => Navigator.pop(context, false), icon: const Icon(Icons.close), tooltip: 'Close'),
        ]),
        Text('${widget.trip.vehicleId} · Trip ${widget.trip.tripNumber} · Stop ${order.stopSequence} ${order.outletId}', style: const TextStyle(color: Wp.muted, fontSize: 13)),
        const SizedBox(height: 18),
      ],
      DropdownButtonFormField<String>(
        initialValue: selectedId,
        decoration: const InputDecoration(labelText: 'Item'),
        items: [for (final o in candidates) DropdownMenuItem(value: o.orderId, child: Text('${o.orderRef} · Stop ${o.stopSequence}${o.chilled ? ' · Chilled' : ''}'))],
        onChanged: locked ? null : (v) => setState(() { order = widget.trip.orders.firstWhere((o) => o.orderId == v); units = 1; }),
      ),
      Padding(padding: const EdgeInsets.only(top: 4, bottom: 12), child: Text('Ordered ${order.expectedUnits} for this stop in plan v${widget.trip.planVersion}${order.temperature.isNotEmpty ? ' · ${order.temperature}' : ''}', style: const TextStyle(fontSize: 12, color: Wp.muted))),
      const Text('What happened?', style: TextStyle(fontWeight: FontWeight.w600)),
      const SizedBox(height: 6),
      SegmentedButton<String>(
        segments: const [ButtonSegment(value: 'MISSING', label: Text('Missing')), ButtonSegment(value: 'DAMAGED', label: Text('Damaged')), ButtonSegment(value: 'WRONG_ITEM', label: Text('Wrong item'))],
        selected: {type},
        onSelectionChanged: locked ? null : (s) => setState(() => type = s.first),
      ),
      const SizedBox(height: 14),
      const Text('Quantity affected', style: TextStyle(fontWeight: FontWeight.w600)),
      Row(children: [
        IconButton.outlined(onPressed: !locked && units > 1 ? () => setState(() => units--) : null, icon: const Icon(Icons.remove), tooltip: 'Fewer'),
        SizedBox(width: 56, child: Text('$units', textAlign: TextAlign.center, style: const TextStyle(fontSize: 20, fontWeight: FontWeight.w600))),
        IconButton.outlined(onPressed: !locked && (order.expectedUnits == 0 || units < order.expectedUnits) ? () => setState(() => units++) : null, icon: const Icon(Icons.add), tooltip: 'More'),
        const SizedBox(width: 12),
        Flexible(child: Text('of ${order.expectedUnits} ordered · ${order.expectedUnits - units} on hand', style: const TextStyle(color: Wp.muted))),
      ]),
      const SizedBox(height: 12),
      TextField(controller: note, enabled: !locked, maxLines: 3, maxLength: 500, decoration: const InputDecoration(labelText: 'Note', hintText: 'e.g. only 4 crates in chiller B')),
      const Text('Evidence', style: TextStyle(fontWeight: FontWeight.w600)),
      const SizedBox(height: 6),
      InkWell(
        onTap: sending ? null : _pickPhoto,
        borderRadius: BorderRadius.circular(10),
        child: Container(
          padding: const EdgeInsets.all(14),
          decoration: BoxDecoration(borderRadius: BorderRadius.circular(10), border: Border.all(color: photo == null && _photoRequired ? Wp.red : Wp.border)),
          child: Row(children: [
            if (photo == null) const Icon(Icons.add_a_photo_outlined, color: Wp.primary, size: 32) else ClipRRect(borderRadius: BorderRadius.circular(6), child: Image.memory(photo!, width: 64, height: 64, fit: BoxFit.cover)),
            const SizedBox(width: 12),
            Expanded(child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
              Text(photo == null ? 'Add photo' : 'Photo added · tap to retake', style: const TextStyle(fontWeight: FontWeight.w600, color: Wp.primary)),
              Text(_photoRequired ? 'Required for damaged items' : 'Optional for missing items · required for damaged', style: const TextStyle(fontSize: 12, color: Wp.muted)),
            ])),
            if (photo != null) IconButton(onPressed: () => setState(() => photo = null), icon: const Icon(Icons.close), tooltip: 'Remove photo'),
          ]),
        ),
      ),
      const SizedBox(height: 10),
      const StatusNote(icon: Icons.info, text: 'The order is not reduced automatically. The dispatcher approves a partial load, holds the trip or moves the line to the next run — you will see the decision here, and the trip stays "not ready" until then.'),
      if (locked) const Padding(padding: EdgeInsets.only(top: 8), child: StatusNote(tone: Tone.amber, icon: Icons.cloud_done_outlined, title: 'Report sent', text: 'The dispatcher has the report. The photo did not upload yet — try again; the report will not be sent twice.')),
      if (error.isNotEmpty) Padding(padding: const EdgeInsets.only(top: 8), child: StatusNote(tone: Tone.red, text: error)),
      const SizedBox(height: 16),
      Row(children: [
        Expanded(child: OutlinedButton(onPressed: () => Navigator.pop(context, locked), child: Text(locked ? 'Close without photo' : 'Cancel'))),
        const SizedBox(width: 12),
        Expanded(child: FilledButton(
          style: FilledButton.styleFrom(backgroundColor: Wp.red),
          onPressed: sending || candidates.isEmpty || (_photoRequired && photo == null)
              ? null
              : () async {
                  setState(() { sending = true; error = ''; });
                  final target = widget.trip.orders.firstWhere((o) => o.orderId == selectedId);
                  final err = await widget.controller.reportIssue(widget.trip, target, attempt: attempt, type: type, units: units, note: note.text.trim(), photo: photo, photoMime: photoMime);
                  if (!context.mounted) return;
                  if (err == null) {
                    Navigator.pop(context, true);
                  } else {
                    setState(() { sending = false; error = err; });
                  }
                },
          child: Text(sending ? 'Sending…' : locked ? 'Retry photo upload' : 'Send to dispatcher'),
        )),
      ]),
    ]);
    return widget.compact ? form : SingleChildScrollView(child: form);
  }
}
