import 'package:flutter/material.dart';

import '../shared/models.dart';
import '../theme/tokens.dart';
import '../widgets/common.dart';
import 'driver_controller.dart';

/// Truck check-out: confirm the load matches the list before leaving.
Future<void> showCheckOutSheet(BuildContext context, DriverController c) {
  final trip = c.trip;
  if (trip == null) return Future.value();
  return showWpSheet(context, title: 'Check the load before you leave', subtitle: '${trip.vehicleId} · Trip ${trip.tripNumber} · Plan v${trip.planVersion}', builder: (ctx) {
    return Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
      for (final s in trip.stops)
        CheckRow(
          state: s.shortfalls.isNotEmpty ? 'warn' : 'ok',
          label: 'Stop ${s.sequence} · ${s.outletName} · ${s.orderRef}',
          detail: s.shortfalls.isNotEmpty ? s.shortfalls.map((sf) => '${sf['type']} ${sf['affectedUnits']} units${sf['note'] != null ? ' · ${sf['note']}' : ''}').join(', ') : (s.chilled ? 'Chilled zone' : null),
        ),
      if (trip.stops.any((s) => s.chilled)) const CheckRow(state: 'ok', label: 'Chilled zone 2 to 4 °C, seal number matches'),
      const SizedBox(height: 16),
      FilledButton(onPressed: () async { await c.confirmLoad(); if (ctx.mounted) Navigator.pop(ctx); }, child: const Text('Confirm load on board')),
      const SizedBox(height: 10),
      OutlinedButton(onPressed: () { Navigator.pop(ctx); showReportProblemSheet(context, c, null, initial: 'GOODS'); }, child: const Text("Something isn't on my list")),
      const SizedBox(height: 8),
      const Text('Logged with your name and time. The loader and dispatcher can see it.', textAlign: TextAlign.center, style: TextStyle(fontSize: 12, color: Wp.muted)),
    ]);
  });
}

/// Report a problem in two taps; saved offline and sent when connected.
Future<void> showReportProblemSheet(BuildContext context, DriverController c, DeliveryStop? stop, {String initial = 'VEHICLE'}) {
  final trip = c.trip;
  if (trip == null) return Future.value();
  const options = [
    ('VEHICLE', 'Vehicle breakdown', 'Dispatcher sees rescue options straight away'),
    ('ROAD', 'Road blocked or flooded', null),
    ('OUTLET', 'Outlet closed or no access', null),
    ('GOODS', 'Load wrong, missing or damaged', null),
    ('SAFETY', 'Safety concern', null),
  ];
  var category = initial;
  final note = TextEditingController();
  return showWpSheet(context, title: 'Report a problem', subtitle: '${stop != null ? 'Stop ${stop.sequence} · ${stop.outletName} · ' : ''}${trip.vehicleId}. Your stop, vehicle and time are attached.', builder: (ctx) {
    return StatefulBuilder(builder: (ctx, setState) {
      return Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
        for (final o in options) Padding(padding: const EdgeInsets.only(bottom: 8), child: OptionCard(title: o.$2, subtitle: o.$3, selected: category == o.$1, onTap: () => setState(() => category = o.$1))),
        TextField(controller: note, maxLength: 500, decoration: const InputDecoration(hintText: 'Add a note (optional)')),
        const StatusNote(text: 'No signal? It is saved on this phone and sent when you reconnect.'),
        const SizedBox(height: 12),
        FilledButton(
          onPressed: () async {
            final label = options.firstWhere((o) => o.$1 == category).$2;
            await c.reportProblem(category: category, description: note.text.trim().isEmpty ? label : '$label: ${note.text.trim()}', stop: stop);
            if (ctx.mounted) Navigator.pop(ctx);
            if (context.mounted) ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('Problem saved and sent to dispatch when connected.')));
          },
          child: const Text('Send to dispatcher'),
        ),
        const SizedBox(height: 8),
        OutlinedButton(onPressed: () => Navigator.pop(ctx), child: const Text('Cancel')),
      ]);
    });
  });
}

/// Rejected: record what goes back on the truck and book a re-attempt or deferral.
Future<bool?> showTakeBackSheet(BuildContext context, DriverController c, DeliveryStop stop) {
  var next = 'REATTEMPT';
  var reason = 'STORE_CLOSED';
  const reasons = {'STORE_CLOSED': 'Store closed', 'ARRIVED_LATE': 'Arrived after the receiving window', 'DAMAGED': 'Goods damaged', 'NOT_ORDERED': 'Store did not order this'};
  return showWpSheet<bool>(context, title: 'Rejected: record what goes back', subtitle: 'Stop ${stop.sequence} · ${stop.outletName} · ${stop.orderRef}', builder: (ctx) {
    return StatefulBuilder(builder: (ctx, setState) {
      return Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
        DropdownButtonFormField<String>(
          initialValue: reason,
          decoration: const InputDecoration(labelText: 'Reason'),
          items: [for (final e in reasons.entries) DropdownMenuItem(value: e.key, child: Text(e.value))],
          onChanged: (v) => setState(() => reason = v ?? reason),
        ),
        const SizedBox(height: 12),
        CheckRow(state: 'ok', label: '${stop.orderRef} · taking back ${stop.expectedUnits ?? 'all'} units', detail: stop.chilled ? 'Keep in the chilled zone' : null),
        const SectionTitle('What happens next'),
        OptionCard(title: "Re-attempt on tomorrow's first run", subtitle: 'Dispatcher confirms before the plan locks', selected: next == 'REATTEMPT', onTap: () => setState(() => next = 'REATTEMPT')),
        const SizedBox(height: 8),
        OptionCard(title: 'Ask dispatcher to defer', subtitle: 'The store gets the reason and the next run', selected: next == 'DEFER', onTap: () => setState(() => next = 'DEFER')),
        const SizedBox(height: 8),
        const CheckRow(state: 'ok', label: 'Photo of goods on the truck · store staff name'),
        const SizedBox(height: 12),
        FilledButton(
          onPressed: () async {
            await c.recordOutcome(stop, code: 'REFUSED', reason: reason, note: 'Taken back on the truck · next step: ${next == 'REATTEMPT' ? 're-attempt on next first run' : 'ask dispatcher to defer'}');
            if (ctx.mounted) Navigator.pop(ctx, true);
          },
          child: const Text('Save take-back offline'),
        ),
        const SizedBox(height: 8),
        OutlinedButton(onPressed: () => Navigator.pop(ctx, false), child: const Text('Back')),
      ]);
    });
  });
}

/// Driver was offline when a newer plan was published: the saved stop is kept
/// against its plan version and both are sent for dispatcher review.
class PlanConflictScreen extends StatelessWidget {
  const PlanConflictScreen({super.key, required this.controller});

  final DriverController controller;

  @override
  Widget build(BuildContext context) {
    final t = controller.trip;
    return Scaffold(
      appBar: AppBar(title: const Text('Plan update needs review')),
      body: ListView(padding: const EdgeInsets.all(Wp.space16), children: [
        Text('Your saved stop was recorded against Plan v${t?.acknowledgedVersion ?? 1}', style: const TextStyle(color: Wp.muted)),
        const SizedBox(height: 12),
        const StatusNote(tone: Tone.amber, title: 'Offline record is safe', text: "Do not overwrite the driver's saved event."),
        const SizedBox(height: 12),
        WpCard(
          child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
            const Text('Saved on this phone', style: TextStyle(fontWeight: FontWeight.w700, fontSize: 16)),
            const SizedBox(height: 6),
            StatusTag('PLAN V${t?.acknowledgedVersion ?? 1}', tone: Tone.cool),
            const SizedBox(height: 6),
            Text('${controller.sync.state.pending} saved item(s) waiting to send', style: const TextStyle(color: Wp.muted)),
            const Divider(height: 28),
            const Text('New dispatcher plan', style: TextStyle(fontWeight: FontWeight.w700, fontSize: 16)),
            const SizedBox(height: 6),
            StatusTag('PLAN V${t?.planVersion ?? 2}', tone: Tone.amber),
            const SizedBox(height: 6),
            const Text('Stop sequence or quantities changed.', style: TextStyle(color: Wp.muted)),
          ]),
        ),
        const SizedBox(height: 12),
        const StatusNote(title: 'What happens next', text: 'Keep the saved proof. Send both versions to dispatch so they can reconcile the stop.'),
        const SizedBox(height: 16),
        FilledButton(onPressed: () async { await controller.acknowledgePlan(); await controller.retrySync(); if (context.mounted) Navigator.pop(context); }, child: const Text('Send for dispatcher review')),
        const SizedBox(height: 10),
        OutlinedButton(onPressed: () => Navigator.pop(context), child: const Text('Return to route')),
        const SizedBox(height: 8),
        const Text('Local save ≠ server confirmation · pending until accepted', textAlign: TextAlign.center, style: TextStyle(fontSize: 12, color: Wp.muted)),
      ]),
    );
  }
}
