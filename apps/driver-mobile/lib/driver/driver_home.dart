import 'package:flutter/material.dart';

import '../shared/models.dart';
import '../sync/sync.dart';
import '../theme/tokens.dart';
import '../widgets/common.dart';
import 'driver_controller.dart';
import 'sheets.dart';
import 'stop_flow.dart';
import 'summary_view.dart';

class DriverHome extends StatefulWidget {
  const DriverHome({super.key, required this.controller, required this.onSignOut});

  final DriverController controller;
  final VoidCallback onSignOut;

  @override
  State<DriverHome> createState() => _DriverHomeState();
}

class _DriverHomeState extends State<DriverHome> {
  int tab = 0;
  bool hideOrderTip = false;

  @override
  void initState() {
    super.initState();
    widget.controller.load();
  }

  @override
  Widget build(BuildContext context) {
    final c = widget.controller;
    return ListenableBuilder(
      listenable: Listenable.merge([c, c.sync]),
      builder: (context, _) {
        final unread = c.messages.where((m) => m.acknowledgedAt == null).length;
        return Scaffold(
          body: Column(children: [
            WpPhoneHeader(trailing: todayLabel()),
            Expanded(
              child: Transform.translate(
                offset: const Offset(0, -14),
                child: Container(
                  decoration: const BoxDecoration(color: Wp.surface, borderRadius: BorderRadius.vertical(top: Radius.circular(20))),
                  child: RefreshIndicator(
                    onRefresh: c.load,
                    child: switch (tab) {
                      0 => _RouteView(controller: c, hideTip: hideOrderTip, onHideTip: () => setState(() => hideOrderTip = true)),
                      1 => UpdatesView(controller: c),
                      _ => SummaryView(controller: c, onSignOut: widget.onSignOut),
                    },
                  ),
                ),
              ),
            ),
          ]),
          bottomNavigationBar: NavigationBar(
            selectedIndex: tab,
            onDestinationSelected: (i) => setState(() => tab = i),
            destinations: [
              const NavigationDestination(icon: Icon(Icons.map_outlined), selectedIcon: Icon(Icons.map), label: 'Route'),
              NavigationDestination(icon: Badge(isLabelVisible: unread > 0, label: Text('$unread'), child: const Icon(Icons.notifications_none)), label: 'Updates'),
              const NavigationDestination(icon: Icon(Icons.bar_chart), label: 'Summary'),
            ],
          ),
        );
      },
    );
  }
}

class ConnectionBanner extends StatelessWidget {
  const ConnectionBanner({super.key, required this.state, required this.offline, this.onRetry});

  final SyncState state;
  final bool offline;
  final VoidCallback? onRetry;

  @override
  Widget build(BuildContext context) {
    if (offline || state.phase == SyncPhase.offline) {
      return StatusNote(tone: Tone.amber, icon: Icons.wifi_off, title: 'No connection', text: state.pending > 0 ? '${state.pending} item(s) saved on this phone. You can still record deliveries.' : 'You can still record this delivery.');
    }
    return switch (state.phase) {
      SyncPhase.syncing => StatusNote(tone: Tone.primary, icon: Icons.cloud_upload_outlined, title: 'Connection restored', text: 'Syncing ${state.pending} saved item(s)…'),
      SyncPhase.error => StatusNote(tone: Tone.red, icon: Icons.warning_amber, title: 'Upload needs attention', text: state.detail, trailing: onRetry == null ? null : TextButton(onPressed: onRetry, child: const Text('Retry'))),
      SyncPhase.paused => StatusNote(tone: Tone.red, icon: Icons.lock_outline, title: 'Sync paused', text: state.detail),
      SyncPhase.synced => const StatusNote(tone: Tone.green, icon: Icons.wifi, title: "You're online", text: 'All updates synced.'),
      _ => const SizedBox.shrink(),
    };
  }
}

class _RouteView extends StatelessWidget {
  const _RouteView({required this.controller, required this.hideTip, required this.onHideTip});

  final DriverController controller;
  final bool hideTip;
  final VoidCallback onHideTip;

  @override
  Widget build(BuildContext context) {
    final c = controller;
    final t = c.trip;
    return ListView(padding: const EdgeInsets.all(Wp.space16), children: [
      Row(children: [
        const Expanded(child: StatusNote(tone: Tone.red, icon: Icons.shield_outlined, text: 'Use only when safely parked.')),
        const SizedBox(width: 8),
        OutlinedButton.icon(
          style: OutlinedButton.styleFrom(foregroundColor: Wp.red, side: const BorderSide(color: Wp.red), minimumSize: const Size(0, 44), shape: const StadiumBorder()),
          onPressed: t == null ? null : () => showReportProblemSheet(context, c, c.trip?.nextStop),
          icon: const Icon(Icons.warning_amber, size: 18),
          label: const Text('Report a problem', style: TextStyle(fontSize: 13)),
        ),
      ]),
      const SizedBox(height: Wp.space12),
      ConnectionBanner(state: c.sync.state, offline: c.offline, onRetry: c.retrySync),
      if (c.fromCache) const Padding(padding: EdgeInsets.only(top: 8), child: StatusNote(tone: Tone.muted, icon: Icons.phone_android, text: 'Route opened from the copy saved on this phone.')),
      if (c.error.isNotEmpty) Padding(padding: const EdgeInsets.only(top: 8), child: StatusNote(tone: Tone.red, text: c.error)),
      const SizedBox(height: Wp.space12),
      if (c.loading && t == null) const Padding(padding: EdgeInsets.all(32), child: Center(child: CircularProgressIndicator())),
      if (!c.loading && t == null) const WpCard(child: Text('No trip is assigned to you today. Pull down to refresh.')),
      if (t != null) ...[
        if (t.needsAcknowledgement)
          Padding(
            padding: const EdgeInsets.only(bottom: 12),
            child: StatusNote(tone: Tone.amber, icon: Icons.update, title: 'Plan updated to v${t.planVersion}', text: 'Stops or quantities changed. Read the new plan before the next stop.', trailing: TextButton(onPressed: c.acknowledgePlan, child: const Text('Acknowledge'))),
          ),
        _VehicleCard(trip: t),
        const SizedBox(height: Wp.space16),
        Text('${t.stops.where((s) => s.done).length} of ${t.stops.length} stops completed', style: const TextStyle(fontWeight: FontWeight.w500)),
        const SizedBox(height: 6),
        WpProgress(value: t.stops.isEmpty ? 0 : t.stops.where((s) => s.done).length / t.stops.length, label: 'Stops completed'),
        const SizedBox(height: Wp.space16),
        if (!c.checkedOut && !t.started)
          WpCard(
            color: Wp.amberBg,
            borderColor: const Color(0xFFF0D49B),
            child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
              const Text('Check the load before you leave', style: TextStyle(fontWeight: FontWeight.w700, fontSize: 16)),
              const SizedBox(height: 4),
              const Text('Confirm the load matches the list so a shortfall is found at the depot, not at the store.', style: TextStyle(fontSize: 13)),
              const SizedBox(height: 12),
              FilledButton(onPressed: () => showCheckOutSheet(context, c), child: const Text('Check load')),
            ]),
          )
        else if (t.nextStop != null)
          _NextStopCard(stop: t.nextStop!, onOpen: () => openStop(context, c, t.nextStop!))
        else
          const StatusNote(tone: Tone.green, icon: Icons.check_circle, title: 'Route complete', text: "All stops have an outcome. Finish the trip from Summary."),
        const SizedBox(height: Wp.space16),
        const SectionTitle("Today's route"),
        WpCard(
          padding: EdgeInsets.zero,
          child: Column(children: [
            for (final s in t.stops)
              ListTile(
                onTap: c.checkedOut || t.started ? () => openStop(context, c, s) : null,
                leading: CircleAvatar(
                  radius: 14,
                  backgroundColor: s.done ? Wp.green : (identical(s, t.nextStop) || s.id == t.nextStop?.id) ? Wp.primary : Wp.surface,
                  foregroundColor: s.done || s.id == t.nextStop?.id ? Colors.white : Wp.ink,
                  child: s.done ? const Icon(Icons.check, size: 16) : Text('${s.sequence}', style: const TextStyle(fontSize: 13, fontWeight: FontWeight.w700)),
                ),
                title: Text(s.outletName, style: const TextStyle(fontWeight: FontWeight.w600)),
                subtitle: s.done ? Text(s.outcomeCode ?? 'Completed', style: TextStyle(color: s.delivered ? Wp.green : Wp.amber)) : (s.chilled ? const Text('Chilled', style: TextStyle(color: Wp.cool)) : null),
                trailing: Text('${hhmm(s.windowOpen)} - ${hhmm(s.windowClose)}', style: const TextStyle(fontWeight: FontWeight.w500)),
              ),
          ]),
        ),
        if (!hideTip) Padding(padding: const EdgeInsets.only(top: 12), child: StatusNote(icon: Icons.info_outline, text: 'Follow the planned stop order.', onDismiss: onHideTip)),
      ],
    ]);
  }
}

class _VehicleCard extends StatelessWidget {
  const _VehicleCard({required this.trip});

  final DeliveryTrip trip;

  @override
  Widget build(BuildContext context) {
    return WpCard(
      color: const Color(0xFFF3F3F0),
      borderColor: const Color(0xFFC9A84A),
      child: Row(children: [
        Container(
          width: 64,
          height: 52,
          decoration: BoxDecoration(color: Wp.surface, borderRadius: BorderRadius.circular(8)),
          child: const Icon(Icons.local_shipping, size: 36, color: Wp.muted, semanticLabel: 'Truck'),
        ),
        const SizedBox(width: 14),
        Expanded(
          child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
            Text(trip.vehicleId, style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 16)),
            Text('Trip ${trip.planRef.isEmpty ? trip.tripId : '${trip.planRef}-${trip.tripNumber}'}'),
            Text('${depotName(trip.depot)} Depot'),
            if (trip.stops.isNotEmpty) Text('${hhmm(trip.stops.first.windowOpen)}-${hhmm(trip.stops.last.windowClose)}'),
          ]),
        ),
      ]),
    );
  }
}

class _NextStopCard extends StatelessWidget {
  const _NextStopCard({required this.stop, required this.onOpen});

  final DeliveryStop stop;
  final VoidCallback onOpen;

  @override
  Widget build(BuildContext context) {
    return WpCard(
      color: const Color(0xFFDDE7FB),
      borderColor: const Color(0xFF9DB3EE),
      child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
        Row(children: [
          const Icon(Icons.location_on, size: 44, color: Wp.primary),
          const SizedBox(width: 12),
          Expanded(
            child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
              const Text('Next Stop', style: TextStyle(color: Wp.primary, fontWeight: FontWeight.w700)),
              Text('${stop.outletId} - ${stop.outletName}', style: const TextStyle(fontWeight: FontWeight.w700)),
              Text('${hhmm(stop.windowOpen)} - ${hhmm(stop.windowClose)}'),
              Text(stop.expectedUnits != null ? '${stop.expectedUnits} units' : stop.orderRef),
            ]),
          ),
        ]),
        const SizedBox(height: 12),
        FilledButton(onPressed: onOpen, child: const Row(mainAxisAlignment: MainAxisAlignment.center, children: [Text('View Stop'), SizedBox(width: 8), Icon(Icons.chevron_right)])),
      ]),
    );
  }
}

class UpdatesView extends StatelessWidget {
  const UpdatesView({super.key, required this.controller});

  final DriverController controller;

  @override
  Widget build(BuildContext context) {
    final c = controller;
    return ListView(padding: const EdgeInsets.all(Wp.space16), children: [
      const SectionTitle('Updates from dispatch'),
      if (c.trip?.needsAcknowledgement ?? false)
        Padding(padding: const EdgeInsets.only(bottom: 12), child: StatusNote(tone: Tone.amber, title: 'Plan v${c.trip!.planVersion} published', text: 'Acknowledge it so dispatch knows you have the current route.', trailing: TextButton(onPressed: c.acknowledgePlan, child: const Text('Acknowledge')))),
      if (c.messages.isEmpty) const WpCard(child: Text('No messages from dispatch yet.', style: TextStyle(color: Wp.muted))),
      for (final m in c.messages)
        Padding(
          padding: const EdgeInsets.only(bottom: 10),
          child: WpCard(
            child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
              Text(m.body),
              const SizedBox(height: 6),
              Text('${m.sentBy} · ${hhmm(m.createdAt)}', style: const TextStyle(fontSize: 12, color: Wp.muted)),
              const SizedBox(height: 6),
              m.acknowledgedAt != null
                  ? StatusTag('Acknowledged ${hhmm(m.acknowledgedAt)}', tone: Tone.green)
                  : Align(alignment: Alignment.centerLeft, child: OutlinedButton(onPressed: () => c.acknowledgeMessage(m), child: const Text('Acknowledge'))),
            ]),
          ),
        ),
    ]);
  }
}
