import 'package:flutter/material.dart';

import '../theme/tokens.dart';
import '../widgets/common.dart';
import 'driver_controller.dart';
import 'sheets.dart';

/// End of day: delivered, partial and failed stops, pending uploads and finish.
class SummaryView extends StatelessWidget {
  const SummaryView({super.key, required this.controller, required this.onSignOut});

  final DriverController controller;
  final VoidCallback onSignOut;

  @override
  Widget build(BuildContext context) {
    final c = controller;
    final t = c.trip;
    if (t == null) return ListView(padding: const EdgeInsets.all(16), children: [const WpCard(child: Text('No trip today.')), const SizedBox(height: 12), OutlinedButton(onPressed: onSignOut, child: const Text('Sign out'))]);
    final stops = t.stops;
    final completed = stops.where((s) => s.done && (s.outcomeCode == 'DELIVERED')).toList();
    final unresolved = stops.where((s) => !s.done || s.outcomeCode != 'DELIVERED').toList();
    final partial = stops.where((s) => s.outcomeCode == 'PARTIAL').length;
    final failed = stops.where((s) => ['FAILED', 'REFUSED', 'NOT_DELIVERED'].contains(s.outcomeCode)).length;
    final allDone = stops.every((s) => s.done);
    final pending = c.sync.state;
    return ListView(padding: const EdgeInsets.all(Wp.space16), children: [
      WpCard(
        color: allDone ? Wp.greenBg : Wp.tint,
        borderColor: allDone ? const Color(0xFFB9E3CC) : Wp.border,
        child: Row(children: [
          CircleAvatar(radius: 26, backgroundColor: allDone ? Wp.green : Wp.primary, child: Icon(allDone ? Icons.check : Icons.route, color: Colors.white, size: 30)),
          const SizedBox(width: 14),
          Expanded(child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
            Text(allDone ? 'Route Complete' : 'Route in progress', style: const TextStyle(fontSize: 20, fontWeight: FontWeight.w700)),
            Text(allDone ? "You've finished today's route" : '${stops.where((s) => !s.done).length} stop(s) still need an outcome'),
          ])),
        ]),
      ),
      const SizedBox(height: 12),
      Row(children: [
        Expanded(child: WpCard(color: Wp.greenBg, borderColor: const Color(0xFFB9E3CC), child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [const Text('Completed'), Text('${completed.length}', style: const TextStyle(fontSize: 22, fontWeight: FontWeight.w700))]))),
        const SizedBox(width: 10),
        Expanded(child: WpCard(color: Wp.amberBg, borderColor: const Color(0xFFF0D49B), child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [const Text('Unresolved'), Text('${unresolved.length}', style: const TextStyle(fontSize: 22, fontWeight: FontWeight.w700))]))),
      ]),
      const SizedBox(height: 12),
      WpCard(
        color: Wp.tint,
        child: Column(children: [
          KvRow('Stops:', '${stops.length}'),
          KvRow('Delivered:', '${completed.length}'),
          KvRow('Partial:', '$partial'),
          KvRow('Failed / Refused:', '$failed'),
        ]),
      ),
      if (completed.isNotEmpty) ...[
        const SectionTitle('Completed stops'),
        WpCard(color: Wp.greenBg, borderColor: const Color(0xFFB9E3CC), padding: EdgeInsets.zero, child: Column(children: [
          for (final s in completed) ListTile(leading: const Icon(Icons.check_circle, color: Wp.green), title: Text(s.outletName, style: const TextStyle(fontWeight: FontWeight.w600)), subtitle: const Text('Delivered', style: TextStyle(color: Wp.green)), trailing: Text('${hhmm(s.windowOpen)} - ${hhmm(s.windowClose)}')),
        ])),
      ],
      if (unresolved.isNotEmpty) ...[
        const SectionTitle('Unresolved stops'),
        WpCard(color: Wp.amberBg, borderColor: const Color(0xFFF0D49B), padding: EdgeInsets.zero, child: Column(children: [
          for (final s in unresolved)
            ListTile(
              leading: const Icon(Icons.error, color: Color(0xFFD9822B)),
              title: Text(s.outletName, style: const TextStyle(fontWeight: FontWeight.w600)),
              subtitle: Text(s.done ? '${s.outcomeCode}${(s.outcomeReason ?? '').isNotEmpty ? ' · ${s.outcomeReason}' : ''} · Dispatcher notified' : 'No outcome yet', style: const TextStyle(color: Wp.amber)),
              trailing: Text('${hhmm(s.windowOpen)} - ${hhmm(s.windowClose)}'),
            ),
        ])),
      ],
      const SectionTitle('Pending uploads'),
      Row(children: [
        Expanded(child: WpCard(child: Row(children: [const Icon(Icons.cloud_upload_outlined, color: Wp.primary), const SizedBox(width: 8), Expanded(child: Text(pending.pending == 0 ? 'Nothing waiting to upload' : '${pending.pending} item(s) waiting to be uploaded'))]))),
        if (pending.pending > 0) ...[const SizedBox(width: 8), OutlinedButton(onPressed: c.retrySync, child: const Text('Retry Upload'))],
      ]),
      const SizedBox(height: 10),
      StatusNote(icon: Icons.info_outline, text: pending.pending == 0 ? 'All delivery records are saved on the server.' : 'Delivery records saved. ${pending.pending} upload(s) pending.'),
      if (t.needsAcknowledgement && pending.pending > 0) ...[
        const SizedBox(height: 10),
        OutlinedButton(onPressed: () => Navigator.of(context).push(MaterialPageRoute(builder: (_) => PlanConflictScreen(controller: c))), child: const Text('Review plan conflict')),
      ],
      const SizedBox(height: 16),
      FilledButton(
        onPressed: allDone && !t.completed
            ? () async {
                final ok = await c.finishTrip();
                if (context.mounted) ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(ok ? 'Trip finished. Completion is sent when connected.' : 'Every stop needs an outcome first.')));
              }
            : null,
        child: Text(t.completed ? 'Trip finished' : 'Finish trip'),
      ),
      const SizedBox(height: 10),
      OutlinedButton(onPressed: onSignOut, child: const Text('Sign out')),
    ]);
  }
}
