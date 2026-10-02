import 'package:flutter/material.dart';

import '../sync/sync.dart';
import '../theme/tokens.dart';
import '../widgets/common.dart';
import 'driver_controller.dart';

/// "Saved on this device" → "Syncing" → "Synced", or an actionable sync error.
class SyncStatusScreen extends StatelessWidget {
  const SyncStatusScreen({super.key, required this.controller, required this.stopId});

  final DriverController controller;
  final String stopId;

  @override
  Widget build(BuildContext context) {
    return ListenableBuilder(
      listenable: Listenable.merge([controller, controller.sync]),
      builder: (context, _) {
        final s = controller.sync.state;
        final stop = controller.trip?.stops.where((x) => x.id == stopId).firstOrNull;
        final offline = s.phase == SyncPhase.offline || controller.offline;
        final (Tone tone, IconData icon, String title, String text, String banner, String bannerText) = offline
            ? (Tone.amber, Icons.phone_android, 'Saved on this device', 'Delivery outcome and photo are stored on this phone. They have not reached the dispatcher yet.', 'Offline', "You're currently offline.")
            : switch (s.phase) {
                SyncPhase.syncing => (Tone.primary, Icons.cloud_upload_outlined, 'Syncing', 'Uploading your saved delivery update', 'Connection restored', "You're back online."),
                SyncPhase.error => (Tone.red, Icons.cloud_off, s.pendingProofs > 0 ? "Photo couldn't be uploaded" : 'Upload needs attention', '${s.detail} Your other records stay synced.', 'Upload needs attention', "Some items couldn't be uploaded."),
                SyncPhase.paused => (Tone.red, Icons.lock_outline, 'Sync paused', s.detail, 'Sign in again', 'Saved work is kept.'),
                _ => (Tone.green, Icons.check, 'Synced', 'The dispatcher has received your delivery outcome and photo', "You're online", 'All updates synced.'),
              };
        return Scaffold(
          body: Column(children: [
            WpPhoneHeader(trailing: todayLabel()),
            Expanded(
              child: Transform.translate(
                offset: const Offset(0, -14),
                child: Container(
                  decoration: const BoxDecoration(color: Wp.surface, borderRadius: BorderRadius.vertical(top: Radius.circular(20))),
                  child: ListView(padding: const EdgeInsets.all(Wp.space16), children: [
                    StatusNote(tone: tone, icon: offline ? Icons.wifi_off : Icons.wifi, title: banner, text: bannerText),
                    const SizedBox(height: 24),
                    Center(
                      child: Container(
                        width: 120,
                        height: 120,
                        decoration: BoxDecoration(color: tone.bg, shape: BoxShape.circle),
                        child: Center(
                          child: Container(
                            width: 76,
                            height: 76,
                            decoration: BoxDecoration(color: tone == Tone.green ? Wp.green : Colors.transparent, shape: BoxShape.circle),
                            child: Icon(icon, size: 48, color: tone == Tone.green ? Colors.white : tone.fg),
                          ),
                        ),
                      ),
                    ),
                    const SizedBox(height: 16),
                    Text(title, textAlign: TextAlign.center, style: const TextStyle(fontSize: 20, fontWeight: FontWeight.w700)),
                    const SizedBox(height: 4),
                    Text(text, textAlign: TextAlign.center),
                    const SizedBox(height: 24),
                    if (stop != null)
                      WpCard(
                        color: tone.bg.withValues(alpha: 0.6),
                        borderColor: tone.bg,
                        child: Column(children: [
                          ListTile(contentPadding: EdgeInsets.zero, leading: const Icon(Icons.location_on, color: Wp.primary), title: Text('${stop.outletId} - ${stop.outletName}', style: const TextStyle(fontWeight: FontWeight.w700))),
                          ListTile(contentPadding: EdgeInsets.zero, leading: const Icon(Icons.check_circle, color: Wp.green), title: Text('${stop.outcomeCode ?? 'Outcome'}${offline ? '' : ' · uploaded'}')),
                          ListTile(contentPadding: EdgeInsets.zero, leading: const Icon(Icons.schedule, color: Wp.cool), title: Text('Saved at ${TimeOfDay.now().format(context)}')),
                          if (s.pending > 0) ListTile(contentPadding: EdgeInsets.zero, leading: Icon(Icons.image_outlined, color: tone.fg), title: Text('Pending: ${s.pending} item(s)${s.pendingProofs > 0 ? ' · ${s.pendingProofs} photo' : ''}')),
                          if (s.phase == SyncPhase.syncing) const WpProgress(value: 0.75, label: 'Upload progress'),
                        ]),
                      ),
                    const SizedBox(height: 16),
                    if (s.phase == SyncPhase.synced && !offline) const StatusNote(tone: Tone.green, icon: Icons.check_circle, text: 'No pending updates'),
                    if (s.phase == SyncPhase.syncing) StatusNote(icon: Icons.description_outlined, text: '${s.pending} upload remaining'),
                    if (s.phase == SyncPhase.error || s.phase == SyncPhase.offline) ...[
                      FilledButton(onPressed: controller.retrySync, child: Text(s.pendingProofs > 0 ? 'Retry photo upload' : 'Retry upload')),
                      const SizedBox(height: 10),
                    ],
                    (s.phase == SyncPhase.synced && !offline)
                        ? FilledButton(onPressed: () => Navigator.pop(context), child: const Text('Continue route'))
                        : OutlinedButton(onPressed: () => Navigator.pop(context), child: const Text('Back to route')),
                    const SizedBox(height: 16),
                    if (s.phase == SyncPhase.error) const StatusNote(icon: Icons.info_outline, text: "Retry when you have a stable connection. Do not clear this app's data."),
                    if (offline) const StatusNote(icon: Icons.info_outline, text: "Updates will be uploaded when you're online."),
                  ]),
                ),
              ),
            ),
          ]),
        );
      },
    );
  }
}
