import 'package:flutter/material.dart';

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

class TripDetailScreen extends StatelessWidget {
  const TripDetailScreen({super.key, required this.controller, required this.tripId, required this.profile});

  final LoaderController controller;
  final String tripId;
  final Profile profile;

  @override
  Widget build(BuildContext context) {
    return ListenableBuilder(
      listenable: Listenable.merge([controller, controller.sync]),
      builder: (context, _) => LayoutBuilder(builder: (context, box) {
        final tablet = box.maxWidth >= Wp.tabletBreakpoint;
        final trip = controller.details[tripId] ?? controller.trips.where((t) => t.tripId == tripId).firstOrNull;
        if (trip == null) return const Scaffold(body: Center(child: Text('Trip not found on today\'s load list.')));
        final groups = controller.loadOrder(trip);
        final shortOrders = trip.orders.where((o) => o.unresolved).toList();
        final readyNow = controller.readyConfirmed.contains(trip.tripId) || trip.ready;
        final checks = <(String, String)>[
          (trip.planChanged ? 'bad' : 'ok', trip.planChanged ? 'Plan v${trip.planVersion} not acknowledged yet' : 'Plan v${trip.planVersion} acknowledged'),
          (groups.every((g) => g.loaded) ? 'ok' : 'now', 'Stops loaded · ${groups.where((g) => g.loaded).length} of ${groups.length}'),
          (shortOrders.isEmpty ? 'ok' : 'bad', shortOrders.isEmpty ? 'No unresolved shortfalls' : '${shortOrders.length} shortfall(s) unresolved'),
          if (trip.refrigerated) (readyNow ? 'ok' : 'todo', 'Chilled zone at 2–4 °C'),
          (readyNow ? 'ok' : 'todo', 'Doors closed & sealed'),
        ];
        final canReady = !readyNow && !trip.planChanged && groups.isNotEmpty && trip.orders.every((o) => o.loaded || (o.short && !o.unresolved)) && shortOrders.isEmpty;

        final guidance = _Guidance(controller: controller, trip: trip, groups: groups, onReport: (o) => _report(context, trip, o, tablet));
        final side = Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
          _Capacity(trip: trip, groups: groups),
          const SizedBox(height: 16),
          _ReadyCard(checks: checks, ready: readyNow, canReady: canReady, onReady: () => _confirmReady(context, trip, tablet), shortCount: shortOrders.length, profile: profile),
        ]);
        final header = Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
          Row(children: [
            TextButton.icon(onPressed: () => Navigator.pop(context), icon: const Icon(Icons.arrow_back, size: 18), label: const Text('Load list')),
            Text('/  ${trip.vehicleId} · Trip ${trip.tripNumber}', style: const TextStyle(color: Wp.muted)),
            const Spacer(),
            OutlinedButton.icon(onPressed: () => _checkOrder(context, trip), icon: const Icon(Icons.qr_code_scanner, size: 18), label: const Text('Check an order number')),
          ]),
          const SizedBox(height: 8),
          Wrap(crossAxisAlignment: WrapCrossAlignment.end, spacing: 12, runSpacing: 8, children: [
            Text('${trip.vehicleId} · Trip ${trip.tripNumber}', style: TextStyle(fontSize: tablet ? 28 : 22, fontWeight: FontWeight.w700)),
            StatusTag(trip.refrigerated ? 'Chilled' : 'Ambient', tone: trip.refrigerated ? Tone.cool : Tone.muted),
            StatusTag('Trip ${trip.tripNumber} of 2'),
            StatusTag('Manifest v${trip.planVersion}', tone: Tone.primary),
          ]),
          Text('${[trip.vehicleType, trip.capability].where((s) => s.isNotEmpty).join(' · ')} · ${depotName(trip.depot)} · ${trip.planRef}', style: const TextStyle(color: Wp.muted)),
          const SizedBox(height: 16),
          if (readyNow)
            const StatusNote(tone: Tone.green, icon: Icons.check_circle, title: 'Ready to depart', text: 'Logged with your name and time. The driver now sees the trip as ready.')
          else if (shortOrders.isEmpty && trip.orders.any((o) => o.short))
            StatusNote(tone: Tone.green, icon: Icons.check_circle, title: 'Dispatcher decision received', text: trip.orders.where((o) => o.short).expand((o) => o.issues.map((i) => '${o.orderRef}: ${i.decisionLabel}${i.decisionNote.isNotEmpty ? ' · ${i.decisionNote}' : ''}')).join(' · '))
          else if (shortOrders.isNotEmpty && controller.waitingForDispatcher[trip.tripId] != null)
            StatusNote(tone: Tone.amber, icon: Icons.hourglass_top, title: 'Sent to dispatcher · waiting for a decision', text: '${controller.waitingForDispatcher[trip.tripId]}. Ready to depart stays locked until the dispatcher decides. Keep loading the other stops.')
          else if (shortOrders.isNotEmpty)
            StatusNote(tone: Tone.red, icon: Icons.error, title: '${shortOrders.length} unresolved shortfall(s) on this trip', text: '${shortOrders.map((o) => '${o.orderRef} · Stop ${o.stopSequence}: ${o.issues.map((i) => '${i.units} ${i.type.toLowerCase()}').join(', ')}').join('\n')}\nThe order is not reduced until the dispatcher decides.'),
          if (trip.planChanged) Padding(padding: const EdgeInsets.only(top: 12), child: StatusNote(tone: Tone.amber, title: 'Plan updated to v${trip.planVersion}', text: 'Acknowledge the revised load before you continue.', trailing: FilledButton(onPressed: () => controller.acknowledgePlan(trip), child: const Text('Acknowledge revised load')))),
          const SizedBox(height: 16),
        ]);

        if (tablet) {
          return Scaffold(
            body: Column(children: [
              LoaderTabletHeader(controller: controller, profile: profile, onSwitchUser: () => Navigator.pop(context)),
              Expanded(child: ListView(padding: const EdgeInsets.fromLTRB(28, 16, 28, 28), children: [
                header,
                Row(crossAxisAlignment: CrossAxisAlignment.start, children: [Expanded(child: guidance), const SizedBox(width: 20), SizedBox(width: 400, child: side)]),
              ])),
            ]),
          );
        }
        return Scaffold(
          body: ListView(padding: EdgeInsets.zero, children: [
            LoaderBanner(phone: true, title: '${trip.vehicleId} · Trip ${trip.tripNumber}', subtitle: '${trip.capability.isEmpty ? trip.vehicleType : trip.capability} · ${depotName(trip.depot)}'),
            Padding(padding: const EdgeInsets.all(16), child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [header, side, const SizedBox(height: 16), guidance])),
          ]),
          bottomNavigationBar: SafeArea(
            child: Container(
              decoration: const BoxDecoration(color: Wp.surface, border: Border(top: BorderSide(color: Wp.border))),
              padding: const EdgeInsets.fromLTRB(16, 12, 16, 12),
              child: Row(children: [
                OutlinedButton(style: OutlinedButton.styleFrom(foregroundColor: Wp.red, side: const BorderSide(color: Wp.red)), onPressed: trip.orders.isEmpty ? null : () => _report(context, trip, trip.orders.firstWhere((o) => !o.loaded, orElse: () => trip.orders.first), false), child: const Text('Report issue')),
                const SizedBox(width: 10),
                Expanded(child: FilledButton(style: FilledButton.styleFrom(backgroundColor: readyNow ? Wp.green : Wp.primary), onPressed: canReady ? () => _confirmReady(context, trip, false) : null, child: Text(readyNow ? 'Ready' : 'Ready to depart'))),
              ]),
            ),
          ),
        );
      }),
    );
  }

  Future<void> _confirmReady(BuildContext context, LoadingTrip trip, bool tablet) async {
    final groups = controller.stops(trip);
    final content = Column(mainAxisSize: MainAxisSize.min, crossAxisAlignment: CrossAxisAlignment.stretch, children: [
      const CircleAvatar(radius: 24, backgroundColor: Wp.green, child: Icon(Icons.check, color: Colors.white, size: 30)),
      const SizedBox(height: 12),
      Text('Confirm ${trip.vehicleId} is ready to depart?', style: const TextStyle(fontSize: 20, fontWeight: FontWeight.w700)),
      Text('Loaded against manifest v${trip.planVersion}. The driver and dispatcher are notified.', style: const TextStyle(color: Wp.muted)),
      const SizedBox(height: 12),
      Container(
        padding: const EdgeInsets.all(12),
        decoration: BoxDecoration(color: Wp.canvas, borderRadius: BorderRadius.circular(8)),
        child: Column(children: [
          KvRow('Manifest', 'v${trip.planVersion} · ${groups.length} stops · ${trip.orders.length} orders'),
          KvRow('Shortfalls', '${trip.shortfallCount} open'),
          if (trip.refrigerated) const KvRow('Chilled zone', 'Checked 2–4 °C'),
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
    if (ok == true) {
      final done = await controller.confirmReady(trip);
      if (context.mounted && !done) ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(controller.sync.state.detail)));
    }
  }

  Future<void> _checkOrder(BuildContext context, LoadingTrip trip) async {
    final input = TextEditingController();
    await showDialog<void>(context: context, builder: (ctx) => StatefulBuilder(builder: (ctx, setState) {
      final match = controller.findOrder(input.text);
      final wrong = match != null && match.$1.tripId != trip.tripId;
      return AlertDialog(
        title: const Text('Check an order number'),
        content: SizedBox(width: 460, child: Column(mainAxisSize: MainAxisSize.min, crossAxisAlignment: CrossAxisAlignment.stretch, children: [
          TextField(controller: input, autofocus: true, decoration: const InputDecoration(hintText: 'e.g. FR-4702'), onChanged: (_) => setState(() {})),
          const SizedBox(height: 12),
          if (input.text.trim().isNotEmpty && match == null) const StatusNote(tone: Tone.amber, text: 'This order is not on any load list at this dock today. Tell the dispatcher.'),
          if (wrong) StatusNote(tone: Tone.red, text: 'Stop. ${match.$2.orderRef} belongs on ${match.$1.vehicleId} (trip ${match.$1.tripNumber}), not this vehicle ${trip.vehicleId}.'),
          if (match != null && !wrong) StatusNote(tone: Tone.green, text: '${match.$2.orderRef} belongs on this vehicle · Stop ${match.$2.stopSequence}${match.$2.chilled ? ' · chilled zone' : ''}.'),
        ])),
        actions: [
          if (wrong) FilledButton(onPressed: () => Navigator.pop(ctx), child: const Text('Put back, not on this truck')),
          if (match != null && !wrong && !match.$2.loaded) FilledButton(onPressed: () { controller.markLoaded(trip, match.$2); Navigator.pop(ctx); }, child: const Text('Mark loaded')),
          OutlinedButton(onPressed: () => Navigator.pop(ctx), child: const Text('Close')),
        ],
      );
    }));
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
        : showWpSheet<bool>(context, title: 'Report missing or damaged', subtitle: 'Stop ${order.stopSequence} · ${order.orderRef}', builder: (ctx) => _ReportForm(controller: controller, trip: trip, initial: order, compact: true));
    final result = await pending;
    if (result == true && context.mounted) {
      await showDialog<void>(context: context, builder: (ctx) => AlertDialog(
        title: const Text('Sent to dispatcher · waiting for a decision'),
        content: SizedBox(width: 460, child: Column(mainAxisSize: MainAxisSize.min, crossAxisAlignment: CrossAxisAlignment.stretch, children: [
          Text(controller.waitingForDispatcher[trip.tripId] ?? '', style: const TextStyle(color: Wp.muted)),
          const SizedBox(height: 12),
          const StatusNote(tone: Tone.amber, text: 'Ready to depart stays locked until the dispatcher decides. Keep loading the other stops.'),
        ])),
        actions: [FilledButton(onPressed: () => Navigator.pop(ctx), child: const Text('Back to trip'))],
      ));
    }
  }
}

class _Guidance extends StatelessWidget {
  const _Guidance({required this.controller, required this.trip, required this.groups, required this.onReport});
  final LoaderController controller;
  final LoadingTrip trip;
  final List<StopGroup> groups;
  final ValueChanged<LoadingOrder> onReport;

  @override
  Widget build(BuildContext context) {
    final current = groups.indexWhere((g) => !g.loaded);
    return WpCard(
      padding: EdgeInsets.zero,
      child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
        Padding(
          padding: const EdgeInsets.all(16),
          child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
            Row(children: [const Expanded(child: Text('Stop-by-stop load guidance', style: TextStyle(fontSize: 18, fontWeight: FontWeight.w600))), StatusTag('${groups.where((g) => g.loaded).length} of ${groups.length} stops loaded', tone: groups.every((g) => g.loaded) ? Tone.green : Tone.primary)]),
            const SizedBox(height: 4),
            const Text('Load in reverse drop-off order. First in goes to the front of the cargo; the last stop sits by the doors.', style: TextStyle(fontSize: 13, color: Wp.muted)),
          ]),
        ),
        for (var i = 0; i < groups.length; i++)
          Container(
            decoration: BoxDecoration(color: i == current ? Wp.tint : null, border: const Border(top: BorderSide(color: Wp.border))),
            padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
            child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
              Row(crossAxisAlignment: CrossAxisAlignment.start, children: [
                CircleAvatar(
                  radius: 13,
                  backgroundColor: groups[i].loaded ? Wp.green : i == current ? Wp.primary : Wp.surface,
                  child: groups[i].loaded ? const Icon(Icons.check, size: 16, color: Colors.white) : Text('${i + 1}', style: TextStyle(fontSize: 12, fontWeight: FontWeight.w700, color: i == current ? Colors.white : Wp.muted)),
                ),
                const SizedBox(width: 12),
                Expanded(child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                  Text('Load ${i + 1} → Stop ${groups[i].stopSequence} · ${groups[i].outletId}', style: const TextStyle(fontWeight: FontWeight.w600, fontSize: 15)),
                  const SizedBox(height: 4),
                  Wrap(spacing: 6, runSpacing: 4, children: [
                    StatusTag(_zone(i, groups.length) + (groups[i].chilled ? ' · chilled zone' : ''), tone: Tone.primary),
                    if (groups[i].chilled) const StatusTag('Chilled', tone: Tone.cool),
                    if (groups[i].hasShortfall) const StatusTag('Shortfall reported', tone: Tone.amber),
                  ]),
                ])),
                Text('${groups[i].orders.length} line(s) · ${groups[i].expectedUnits} units', style: const TextStyle(fontSize: 12, color: Wp.muted)),
              ]),
              if (i == current || groups[i].hasShortfall) ...[
                const SizedBox(height: 10),
                for (final o in groups[i].orders)
                  Container(
                    margin: const EdgeInsets.only(bottom: 6),
                    padding: const EdgeInsets.all(10),
                    decoration: BoxDecoration(color: o.short ? Wp.redBg : Wp.surface, borderRadius: BorderRadius.circular(8), border: Border.all(color: o.short ? const Color(0xFFF1C4BD) : Wp.border)),
                    child: Row(children: [
                      Icon(o.loaded ? Icons.check_box : Icons.check_box_outline_blank, color: o.loaded ? Wp.primary : o.short ? Wp.red : Wp.border),
                      const SizedBox(width: 10),
                      Expanded(child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                        Row(children: [Flexible(child: Text(o.orderRef, style: const TextStyle(fontWeight: FontWeight.w600))), if (o.chilled) ...[const SizedBox(width: 6), const StatusTag('Chilled', tone: Tone.cool)]]),
                        Text('${o.outletId} · expected ${o.expectedUnits}', style: const TextStyle(fontSize: 12, color: Wp.muted)),
                        for (final iss in o.issues) Text('${iss.units} ${iss.type.toLowerCase()} · ${iss.decisionLabel}${iss.decisionNote.isNotEmpty ? ' · ${iss.decisionNote}' : ''}${iss.note.isNotEmpty ? ' · ${iss.note}' : ''}', style: TextStyle(fontSize: 12, color: iss.allowsDeparture ? Wp.green : Wp.red, fontWeight: FontWeight.w600)),
                      ])),
                      if (!o.loaded && !o.short) ...[
                        TextButton(onPressed: () => onReport(o), child: const Text('Report issue')),
                        FilledButton(onPressed: () => controller.markLoaded(trip, o), child: const Text('Loaded')),
                      ] else if (o.loaded)
                        const StatusTag('Loaded', tone: Tone.green),
                    ]),
                  ),
              ],
            ]),
          ),
      ]),
    );
  }
}

class _Capacity extends StatelessWidget {
  const _Capacity({required this.trip, required this.groups});
  final LoadingTrip trip;
  final List<StopGroup> groups;

  @override
  Widget build(BuildContext context) {
    final units = trip.orders.fold<int>(0, (s, o) => s + o.expectedUnits);
    final loadedUnits = trip.orders.where((o) => o.loaded).fold<int>(0, (s, o) => s + o.expectedUnits);
    final chilled = trip.orders.where((o) => o.chilled).length;
    double? w(String k) => trip.raw[k] is num ? (trip.raw[k] as num).toDouble() : null;
    final weight = w('loadedWeightKg'), weightCap = w('weightCapacityKg'), volume = w('loadedVolumeM3'), volumeCap = w('volumeCapacityM3');
    return WpCard(
      child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
        Row(children: [const Expanded(child: Text('Capacity summary', style: TextStyle(fontSize: 18, fontWeight: FontWeight.w600))), StatusTag('Plan v${trip.planVersion}', tone: Tone.primary)]),
        const SizedBox(height: 12),
        if (weight != null && weightCap != null && weightCap > 0) ...[Meter(label: 'Weight', valueText: '${weight.round()} / ${weightCap.round()} kg · ${(weight / weightCap * 100).round()}%', fraction: weight / weightCap), const SizedBox(height: 12)],
        if (volume != null && volumeCap != null && volumeCap > 0) ...[Meter(label: 'Volume', valueText: '${volume.toStringAsFixed(1)} / ${volumeCap.toStringAsFixed(1)} m³ · ${(volume / volumeCap * 100).round()}%', fraction: volume / volumeCap, color: volume / volumeCap >= 0.85 ? Wp.amber : Wp.primary), const SizedBox(height: 12)],
        Meter(label: 'Stops loaded', valueText: '${groups.where((g) => g.loaded).length} / ${groups.length}', fraction: groups.isEmpty ? 0 : groups.where((g) => g.loaded).length / groups.length, color: Wp.green),
        const SizedBox(height: 12),
        Meter(label: 'Units loaded', valueText: '$loadedUnits / $units', fraction: units == 0 ? 0 : loadedUnits / units),
        if (trip.refrigerated) ...[const SizedBox(height: 12), Meter(label: 'Chilled orders', valueText: '$chilled of ${trip.orders.length}', fraction: trip.orders.isEmpty ? 0 : chilled / trip.orders.length, color: Wp.cool)],
        const SizedBox(height: 10),
        Text('Totals follow manifest plan v${trip.planVersion} agreed by the dispatcher. Weight and volume are checked separately when planned.', style: const TextStyle(fontSize: 12, color: Wp.muted)),
      ]),
    );
  }
}

class _ReadyCard extends StatelessWidget {
  const _ReadyCard({required this.checks, required this.ready, required this.canReady, required this.onReady, required this.shortCount, required this.profile});
  final List<(String, String)> checks;
  final bool ready;
  final bool canReady;
  final VoidCallback onReady;
  final int shortCount;
  final Profile profile;

  @override
  Widget build(BuildContext context) {
    return WpCard(
      borderColor: ready ? Wp.green : Wp.border,
      borderWidth: ready ? 2 : 1,
      child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
        const Text('Ready to depart', style: TextStyle(fontSize: 18, fontWeight: FontWeight.w600)),
        const SizedBox(height: 8),
        for (final c in checks) CheckRow(state: c.$1, label: c.$2),
        const SizedBox(height: 12),
        FilledButton(style: FilledButton.styleFrom(backgroundColor: ready ? Wp.green : Wp.primary), onPressed: canReady ? onReady : null, child: Text(ready ? 'Ready · confirmed' : 'Confirm ready to depart')),
        const SizedBox(height: 8),
        Text(
          ready ? 'Logged as ${profile.displayName ?? profile.userId}. The driver sees this trip as ready.' : shortCount > 0 ? 'Resolve $shortCount shortfall(s) with the dispatcher and finish loading to continue.' : 'Finish loading every stop to continue.',
          style: TextStyle(fontSize: 12, color: ready ? Wp.muted : Wp.red),
        ),
      ]),
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
  String type = 'MISSING';
  int units = 1;
  final note = TextEditingController();

  @override
  void dispose() {
    note.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final form = Column(mainAxisSize: MainAxisSize.min, crossAxisAlignment: CrossAxisAlignment.stretch, children: [
      if (!widget.compact) ...[
        Row(children: [
          const Expanded(child: Text('Report missing or damaged item', style: TextStyle(fontSize: 22, fontWeight: FontWeight.w700))),
          IconButton(onPressed: () => Navigator.pop(context, false), icon: const Icon(Icons.close), tooltip: 'Close'),
        ]),
        Text('${widget.trip.vehicleId} · Trip ${widget.trip.tripNumber} · Stop ${order.stopSequence} · ${order.orderRef}', style: const TextStyle(color: Wp.muted, fontSize: 13)),
        const SizedBox(height: 18),
      ],
      DropdownButtonFormField<String>(
        initialValue: order.orderId,
        decoration: const InputDecoration(labelText: 'Item'),
        items: [for (final o in widget.trip.orders) DropdownMenuItem(value: o.orderId, child: Text('${o.orderRef} · Stop ${o.stopSequence}${o.chilled ? ' · Chilled' : ''}'))],
        onChanged: (v) => setState(() => order = widget.trip.orders.firstWhere((o) => o.orderId == v)),
      ),
      Padding(padding: const EdgeInsets.only(top: 4, bottom: 12), child: Text('Expected ${order.expectedUnits} for this stop in plan v${widget.trip.planVersion}', style: const TextStyle(fontSize: 12, color: Wp.muted))),
      const Text('What happened?', style: TextStyle(fontWeight: FontWeight.w600)),
      const SizedBox(height: 6),
      SegmentedButton<String>(
        segments: const [ButtonSegment(value: 'MISSING', label: Text('Missing')), ButtonSegment(value: 'DAMAGED', label: Text('Damaged'))],
        selected: {type},
        onSelectionChanged: (s) => setState(() => type = s.first),
      ),
      const SizedBox(height: 14),
      const Text('Quantity affected', style: TextStyle(fontWeight: FontWeight.w600)),
      Row(children: [
        IconButton.outlined(onPressed: units > 1 ? () => setState(() => units--) : null, icon: const Icon(Icons.remove), tooltip: 'Fewer'),
        SizedBox(width: 56, child: Text('$units', textAlign: TextAlign.center, style: const TextStyle(fontSize: 20, fontWeight: FontWeight.w600))),
        IconButton.outlined(onPressed: order.expectedUnits == 0 || units < order.expectedUnits ? () => setState(() => units++) : null, icon: const Icon(Icons.add), tooltip: 'More'),
        const SizedBox(width: 12),
        Text('of ${order.expectedUnits} ordered', style: const TextStyle(color: Wp.muted)),
      ]),
      const SizedBox(height: 12),
      TextField(controller: note, maxLines: 3, maxLength: 500, decoration: const InputDecoration(labelText: 'Note', hintText: 'e.g. only 4 crates in chiller B')),
      const SizedBox(height: 8),
      const StatusNote(icon: Icons.info, text: 'The order is not reduced automatically. The dispatcher approves a partial load, sends replacement stock or moves the line — you will see the decision here and the trip stays "not ready" until then.'),
      const SizedBox(height: 16),
      Row(children: [
        Expanded(child: OutlinedButton(onPressed: () => Navigator.pop(context, false), child: const Text('Cancel'))),
        const SizedBox(width: 12),
        Expanded(child: FilledButton(
          style: FilledButton.styleFrom(backgroundColor: Wp.red),
          onPressed: () async {
            await widget.controller.reportIssue(widget.trip, order, type: type, units: units, note: note.text.trim());
            if (context.mounted) Navigator.pop(context, true);
          },
          child: const Text('Send to dispatcher'),
        )),
      ]),
    ]);
    return widget.compact ? form : SingleChildScrollView(child: form);
  }
}
