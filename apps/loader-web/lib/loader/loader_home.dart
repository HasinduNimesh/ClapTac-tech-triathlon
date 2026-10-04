import 'package:flutter/material.dart';

import '../shared/models.dart';
import '../theme/tokens.dart';
import '../widgets/common.dart';
import 'loader_controller.dart';
import 'trip_detail_screen.dart';

class LoaderHome extends StatefulWidget {
  const LoaderHome({super.key, required this.controller, required this.profile, required this.onSignOut});

  final LoaderController controller;
  final Profile profile;
  final VoidCallback onSignOut;

  @override
  State<LoaderHome> createState() => _LoaderHomeState();
}

class _LoaderHomeState extends State<LoaderHome> {
  int tab = 0;

  @override
  void initState() {
    super.initState();
    widget.controller.load();
  }

  void _open(LoadingTrip trip) {
    widget.controller.select(trip.tripId);
    Navigator.of(context).push(MaterialPageRoute(builder: (_) => TripDetailScreen(controller: widget.controller, tripId: trip.tripId, profile: widget.profile, onSignOut: widget.onSignOut)));
  }

  @override
  Widget build(BuildContext context) {
    final c = widget.controller;
    return ListenableBuilder(
      listenable: c,
      builder: (context, _) => LayoutBuilder(builder: (context, box) {
        final tablet = box.maxWidth >= Wp.tabletBreakpoint;
        final trips = c.allTrips;
        final changed = trips.where((t) => t.planChanged || (t.needsAck && t.planVersion > 1)).toList();
        final issues = trips.fold<int>(0, (s, t) => s + t.orders.where((o) => o.short).length);
        final notices = c.actionCount;
        final body = switch (tab) {
          1 => _PlanUpdates(controller: c, trips: changed, onOpen: _open),
          2 => _Issues(controller: c, onOpen: _open),
          3 => _Notifications(controller: c, onOpen: _open),
          _ => _LoadList(controller: c, tablet: tablet, onOpen: _open),
        };
        if (tablet) {
          return Scaffold(
            body: Column(children: [
              LoaderHeader(controller: c, profile: widget.profile, onSignOut: widget.onSignOut, onNotifications: () => setState(() => tab = 3)),
              Container(
                color: Wp.surface,
                padding: const EdgeInsets.symmetric(horizontal: 28),
                child: Row(children: [
                  _Tab('Load list', 0, tab, (i) => setState(() => tab = i)),
                  _Tab('Plan updates', 1, tab, (i) => setState(() => tab = i), count: changed.length, tone: Tone.amber),
                  _Tab('Reported issues', 2, tab, (i) => setState(() => tab = i), count: issues, tone: Tone.red),
                  _Tab('Notifications', 3, tab, (i) => setState(() => tab = i), count: notices, tone: Tone.primary),
                ]),
              ),
              Expanded(child: RefreshIndicator(onRefresh: c.load, child: body)),
            ]),
          );
        }
        return Scaffold(
          body: RefreshIndicator(onRefresh: c.load, child: body),
          bottomNavigationBar: NavigationBar(
            selectedIndex: tab,
            onDestinationSelected: (i) => i == 4 ? widget.onSignOut() : setState(() => tab = i),
            destinations: [
              const NavigationDestination(icon: Icon(Icons.inventory_2_outlined), label: 'Load list'),
              NavigationDestination(icon: Badge(isLabelVisible: changed.isNotEmpty, label: Text('${changed.length}'), child: const Icon(Icons.update)), label: 'Plan updates'),
              NavigationDestination(icon: Badge(isLabelVisible: issues > 0, label: Text('$issues'), child: const Icon(Icons.report_outlined)), label: 'Issues'),
              NavigationDestination(icon: Badge(isLabelVisible: notices > 0, label: Text('$notices'), child: const Icon(Icons.notifications_outlined)), label: 'Alerts'),
              const NavigationDestination(icon: Icon(Icons.logout), label: 'Sign out'),
            ],
          ),
        );
      }),
    );
  }
}

/// Tablet header: logo, connection state, depot, notifications and the signed-in loader.
class LoaderHeader extends StatelessWidget {
  const LoaderHeader({super.key, required this.controller, required this.profile, required this.onSignOut, this.onNotifications});

  final LoaderController controller;
  final Profile profile;
  final VoidCallback onSignOut;
  final VoidCallback? onNotifications;

  @override
  Widget build(BuildContext context) {
    final name = profile.displayName ?? profile.userId;
    final lost = controller.connectionLost;
    final count = controller.actionCount;
    return Container(
      color: Wp.surface,
      padding: const EdgeInsets.fromLTRB(28, 10, 28, 10),
      child: Row(children: [
        Image.asset('assets/waypoint-logo.png', height: 36, semanticLabel: 'Waypoint'),
        const SizedBox(width: 16),
        const Text('Loader workspace', style: TextStyle(color: Wp.muted, fontWeight: FontWeight.w500)),
        const Spacer(),
        StatusTag(lost ? 'Connection lost · actions paused' : 'Live · plan synced ${clock(controller.refreshedAt)}', tone: lost ? Tone.red : Tone.green, icon: lost ? Icons.wifi_off : Icons.circle),
        const SizedBox(width: 12),
        Container(padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8), decoration: BoxDecoration(border: Border.all(color: Wp.border), borderRadius: BorderRadius.circular(8)), child: Text('${depotName(profile.depot)} DC')),
        const SizedBox(width: 8),
        IconButton(
          tooltip: 'Notifications',
          onPressed: onNotifications,
          icon: Badge(isLabelVisible: count > 0, label: Text('$count'), child: const Icon(Icons.notifications_outlined)),
        ),
        const SizedBox(width: 8),
        PopupMenuButton<String>(
          tooltip: 'Account',
          onSelected: (v) => v == 'out' ? onSignOut() : null,
          itemBuilder: (_) => [const PopupMenuItem(value: 'out', child: Text('Sign out'))],
          child: Row(children: [
            CircleAvatar(backgroundColor: Wp.tint, foregroundColor: Wp.primary, child: Text(name.isEmpty ? 'L' : name.substring(0, name.length >= 2 ? 2 : 1).toUpperCase(), style: const TextStyle(fontWeight: FontWeight.w700))),
            const SizedBox(width: 10),
            Column(crossAxisAlignment: CrossAxisAlignment.start, mainAxisSize: MainAxisSize.min, children: [Text(name, style: const TextStyle(fontWeight: FontWeight.w600)), Text('Loader · ${depotName(profile.depot)}', style: const TextStyle(fontSize: 12, color: Wp.muted))]),
            const Icon(Icons.arrow_drop_down, color: Wp.muted),
          ]),
        ),
      ]),
    );
  }
}

class _Tab extends StatelessWidget {
  const _Tab(this.label, this.index, this.current, this.onTap, {this.count = 0, this.tone = Tone.primary});
  final String label;
  final int index;
  final int current;
  final ValueChanged<int> onTap;
  final int count;
  final Tone tone;

  @override
  Widget build(BuildContext context) {
    final on = index == current;
    return InkWell(
      onTap: () => onTap(index),
      child: Container(
        padding: const EdgeInsets.fromLTRB(2, 14, 2, 12),
        margin: const EdgeInsets.only(right: 28),
        decoration: BoxDecoration(border: Border(bottom: BorderSide(color: on ? Wp.primary : Colors.transparent, width: 3))),
        child: Row(children: [
          Text(label, style: TextStyle(fontSize: 15, fontWeight: on ? FontWeight.w600 : FontWeight.w500, color: on ? Wp.primary : Wp.muted)),
          if (count > 0) ...[const SizedBox(width: 8), StatusTag('$count', tone: tone)],
        ]),
      ),
    );
  }
}

class LoaderBanner extends StatelessWidget {
  const LoaderBanner({super.key, required this.title, required this.subtitle, this.phone = false, this.trailing});
  final String title;
  final String subtitle;
  final bool phone;
  final Widget? trailing;

  @override
  Widget build(BuildContext context) {
    return Container(
      width: double.infinity,
      padding: EdgeInsets.fromLTRB(phone ? 20 : 28, phone ? MediaQuery.of(context).padding.top + 16 : 22, phone ? 20 : 28, 22),
      decoration: const BoxDecoration(gradient: LinearGradient(colors: [Color(0xFF0B2A8F), Color(0xFF1E4FD8)], begin: Alignment.centerLeft, end: Alignment.centerRight)),
      child: Row(children: [
        Expanded(
          child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
            Text(title, style: TextStyle(color: Colors.white, fontSize: phone ? 24 : 26, fontWeight: FontWeight.w700)),
            const SizedBox(height: 4),
            Text(subtitle, style: const TextStyle(color: Colors.white)),
          ]),
        ),
        if (trailing != null) trailing!,
      ]),
    );
  }
}

class ConnectionNote extends StatelessWidget {
  const ConnectionNote({super.key, required this.controller});
  final LoaderController controller;

  @override
  Widget build(BuildContext context) {
    if (!controller.connectionLost) return const SizedBox.shrink();
    return Padding(
      padding: const EdgeInsets.only(bottom: 12),
      child: StatusNote(tone: Tone.red, icon: Icons.wifi_off, title: 'Connection lost', text: 'The loader app works online only. Nothing is saved until the connection is back — retry, then repeat the last step.', trailing: TextButton(onPressed: controller.load, child: const Text('Retry'))),
    );
  }
}

class _LoadList extends StatefulWidget {
  const _LoadList({required this.controller, required this.tablet, required this.onOpen});
  final LoaderController controller;
  final bool tablet;
  final ValueChanged<LoadingTrip> onOpen;

  @override
  State<_LoadList> createState() => _LoadListState();
}

class _LoadListState extends State<_LoadList> {
  String filter = 'all';

  @override
  Widget build(BuildContext context) {
    final c = widget.controller;
    final tablet = widget.tablet;
    final trips = c.allTrips;
    final changed = trips.where((t) => t.planChanged || (t.needsAck && t.planVersion > 1)).toList();
    final toLoad = trips.where((t) => !t.ready).toList();
    final ready = trips.where((t) => t.ready).toList();
    final shown = switch (filter) { 'load' => toLoad, 'ready' => ready, _ => trips };
    final selected = c.selected;
    final cards = [
      for (final t in shown) Padding(padding: const EdgeInsets.only(bottom: 12), child: _VehicleCard(controller: c, trip: t, selected: t.tripId == c.selectedTripId, tablet: tablet, onOpen: () => widget.onOpen(t))),
    ];
    final content = <Widget>[
      ConnectionNote(controller: c),
      for (final t in changed) Padding(padding: const EdgeInsets.only(bottom: 16), child: ChangeBanner(controller: c, trip: t, compact: !tablet, onView: () => showChanges(context, t, onOpen: () => widget.onOpen(t)))),
      if (c.error.isNotEmpty) Padding(padding: const EdgeInsets.only(bottom: 12), child: StatusNote(tone: Tone.red, text: c.error)),
      _Stats(controller: c, trips: trips, tablet: tablet),
      const SizedBox(height: 16),
      SectionTitle('Assigned vehicles & trips', trailing: Wrap(spacing: 6, children: [
        _Chip('All ${trips.length}', filter == 'all', () => setState(() => filter = 'all')),
        _Chip('To load ${toLoad.length}', filter == 'load', () => setState(() => filter = 'load')),
        _Chip('Ready ${ready.length}', filter == 'ready', () => setState(() => filter = 'ready')),
      ])),
      if (c.loading && trips.isEmpty) const Padding(padding: EdgeInsets.all(32), child: Center(child: CircularProgressIndicator())),
      if (!c.loading && trips.isEmpty) const WpCard(child: Text('No trips are assigned to your depot for this date. Trips appear once the dispatcher locks the plan.')),
    ];
    final first = trips.where((t) => t.departAt != null).map((t) => t.departAt!).fold<DateTime?>(null, (a, b) => a == null || b.isBefore(a) ? b : a);
    return ListView(padding: EdgeInsets.zero, children: [
      LoaderBanner(
        phone: !tablet,
        title: "Today's load list",
        subtitle: '${c.date} · ${trips.isNotEmpty ? '${depotName(trips.first.depot)} · ' : ''}Manifest plan v${selected?.planVersion ?? 1}${first != null ? ' · first departure ${clock(first)}' : ''}',
        trailing: _DatePicker(controller: c),
      ),
      Padding(
        padding: EdgeInsets.all(tablet ? 28 : 16),
        child: tablet && selected != null
            ? Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
                ...content,
                Row(crossAxisAlignment: CrossAxisAlignment.start, children: [
                  Expanded(child: Column(children: cards)),
                  const SizedBox(width: 20),
                  SizedBox(width: 440, child: StopSequencePanel(controller: c, trip: selected, onOpen: () => widget.onOpen(selected))),
                ]),
              ])
            : Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [...content, ...cards]),
      ),
    ]);
  }
}

class _Chip extends StatelessWidget {
  const _Chip(this.label, this.on, this.onTap);
  final String label;
  final bool on;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) => ChoiceChip(label: Text(label), selected: on, onSelected: (_) => onTap(), showCheckmark: false, selectedColor: Wp.tint, labelStyle: TextStyle(color: on ? Wp.primary : Wp.muted, fontWeight: FontWeight.w600, fontSize: 13));
}

class _DatePicker extends StatelessWidget {
  const _DatePicker({required this.controller});
  final LoaderController controller;

  @override
  Widget build(BuildContext context) {
    return OutlinedButton.icon(
      style: OutlinedButton.styleFrom(foregroundColor: Colors.white, side: const BorderSide(color: Colors.white54)),
      onPressed: () async {
        final current = DateTime.tryParse(controller.date) ?? DateTime.now();
        final picked = await showDatePicker(context: context, initialDate: current, firstDate: current.subtract(const Duration(days: 30)), lastDate: current.add(const Duration(days: 30)));
        if (picked != null) controller.setDate('${picked.year}-${picked.month.toString().padLeft(2, '0')}-${picked.day.toString().padLeft(2, '0')}');
      },
      icon: const Icon(Icons.calendar_today, size: 16),
      label: Text(controller.date),
    );
  }
}

/// "What changed" in the newer plan version, from the server's comparison.
Future<void> showChanges(BuildContext context, LoadingTrip trip, {VoidCallback? onOpen}) {
  final changes = trip.changes;
  return showDialog<void>(context: context, builder: (ctx) => AlertDialog(
    title: Text('Plan v${trip.planVersion} · ${trip.vehicleId} Trip ${trip.tripNumber}'),
    content: SizedBox(width: 480, child: Column(mainAxisSize: MainAxisSize.min, crossAxisAlignment: CrossAxisAlignment.stretch, children: [
      Text('Published${trip.planPublishedBy.isNotEmpty ? ' by dispatcher ${trip.planPublishedBy}' : ''}${trip.planPublishedAt != null ? ' at ${clock(trip.planPublishedAt)}' : ''}.', style: const TextStyle(color: Wp.muted)),
      const SizedBox(height: 12),
      if (changes.isEmpty)
        Text(trip.started ? 'No stop, order or load-order change on this trip. Acknowledge to confirm you have the latest version.' : 'Loading has not started, so the load list already follows v${trip.planVersion}.')
      else
        for (final ch in changes)
          Padding(padding: const EdgeInsets.only(bottom: 8), child: Row(crossAxisAlignment: CrossAxisAlignment.start, children: [
            Icon(ch.kind == 'REMOVED' ? Icons.remove_circle_outline : ch.kind == 'ADDED' ? Icons.add_circle_outline : Icons.swap_vert, size: 20, color: ch.wasLoaded ? Wp.amber : Wp.primary),
            const SizedBox(width: 8),
            Expanded(child: Text(ch.text)),
          ])),
    ])),
    actions: [
      if (onOpen != null) OutlinedButton(onPressed: () { Navigator.pop(ctx); onOpen(); }, child: const Text('Open trip')),
      FilledButton(onPressed: () => Navigator.pop(ctx), child: const Text('Close')),
    ],
  ));
}

class ChangeBanner extends StatelessWidget {
  const ChangeBanner({super.key, required this.controller, required this.trip, required this.compact, required this.onView});
  final LoaderController controller;
  final LoadingTrip trip;
  final bool compact;
  final VoidCallback onView;

  @override
  Widget build(BuildContext context) {
    Future<void> ack() async {
      final err = await controller.acknowledgePlan(trip);
      if (context.mounted) ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(err ?? 'Plan v${trip.planVersion} acknowledged. The load list now follows v${trip.planVersion}.'), backgroundColor: err == null ? null : Wp.red));
    }

    final changes = trip.changes;
    final buttons = [
      OutlinedButton(onPressed: onView, child: const Text('View changes')),
      const SizedBox(width: 10, height: 10),
      FilledButton(onPressed: controller.busy ? null : ack, child: Text(compact ? 'Acknowledge' : 'Acknowledge revised load')),
    ];
    return WpCard(
      color: Wp.amberBg,
      borderColor: const Color(0xFFF0D49B),
      child: Flex(direction: compact ? Axis.vertical : Axis.horizontal, crossAxisAlignment: compact ? CrossAxisAlignment.stretch : CrossAxisAlignment.center, children: [
        Flexible(
          fit: compact ? FlexFit.loose : FlexFit.tight,
          child: Row(crossAxisAlignment: CrossAxisAlignment.start, children: [
            const CircleAvatar(radius: 14, backgroundColor: Wp.amber, child: Text('!', style: TextStyle(color: Colors.white, fontWeight: FontWeight.w700))),
            const SizedBox(width: 12),
            Expanded(child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
              Text('Plan updated to v${trip.planVersion}${trip.planPublishedAt != null ? ' at ${clock(trip.planPublishedAt)}' : ''} — recheck ${trip.vehicleId} before loading further', style: const TextStyle(fontWeight: FontWeight.w600, fontSize: 16)),
              const SizedBox(height: 4),
              if (changes.isEmpty)
                Text(trip.started ? 'You are loading against v${trip.preparedPlanVersion}.' : 'Acknowledge v${trip.planVersion} before you load this trip.', style: const TextStyle(fontSize: 14))
              else
                for (final ch in changes.take(3)) Text('• ${ch.text}', style: const TextStyle(fontSize: 14)),
              if (changes.length > 3) Text('+ ${changes.length - 3} more', style: const TextStyle(fontSize: 13, color: Wp.muted)),
              Text('Published${trip.planPublishedBy.isNotEmpty ? ' by ${trip.planPublishedBy}' : ''} · Dispatcher.', style: const TextStyle(fontSize: 12, color: Wp.amber)),
            ])),
          ]),
        ),
        if (compact) const SizedBox(height: 12),
        compact ? Row(children: [Expanded(child: buttons[0]), const SizedBox(width: 10), Expanded(child: buttons[2])]) : Row(children: buttons),
      ]),
    );
  }
}

class _Stats extends StatelessWidget {
  const _Stats({required this.controller, required this.trips, required this.tablet});
  final LoaderController controller;
  final List<LoadingTrip> trips;
  final bool tablet;

  @override
  Widget build(BuildContext context) {
    final ready = trips.where((t) => t.ready).toList()..sort((a, b) => (b.readyAt ?? DateTime(0)).compareTo(a.readyAt ?? DateTime(0)));
    final loading = trips.where((t) => t.started && !t.ready).length;
    final waiting = trips.fold<int>(0, (s, t) => s + t.orders.where((o) => o.unresolved).length);
    final next = trips.where((t) => !t.ready && t.departAt != null).map((t) => t.departAt!).fold<DateTime?>(null, (a, b) => a == null || b.isBefore(a) ? b : a);
    final items = [
      ('Vehicles assigned', '${trips.map((t) => t.vehicleId).toSet().length}', '${trips.length} trips today', Wp.ink),
      ('Ready to depart', '${ready.length} of ${trips.length}', ready.isEmpty ? 'None yet' : '${ready.first.vehicleId} confirmed ${clock(ready.first.readyAt)}', Wp.green),
      ('Loading now', '$loading', next == null ? 'In progress' : 'Next departure ${clock(next)}', Wp.primary),
      ('Open shortfalls', '$waiting', 'Awaiting dispatcher decision', Wp.red),
    ];
    final tiles = [
      for (final i in (tablet ? items : [items[1], items[3]]))
        Expanded(child: WpCard(child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [Text(i.$1, style: const TextStyle(fontSize: 13, color: Wp.muted, fontWeight: FontWeight.w500)), Text(i.$2, style: TextStyle(fontSize: 26, fontWeight: FontWeight.w700, color: i.$4)), Text(i.$3, style: const TextStyle(fontSize: 12, color: Wp.muted))]))),
    ];
    return Row(children: [for (var i = 0; i < tiles.length; i++) ...[if (i > 0) SizedBox(width: tablet ? 16 : 10), tiles[i]]]);
  }
}

String vehicleLine(LoadingTrip t) => [t.vehicleId, t.vehicleType, t.capability, t.brand].where((s) => s.isNotEmpty).join(' · ');

class _VehicleCard extends StatelessWidget {
  const _VehicleCard({required this.controller, required this.trip, required this.selected, required this.tablet, required this.onOpen});
  final LoaderController controller;
  final LoadingTrip trip;
  final bool selected;
  final bool tablet;
  final VoidCallback onOpen;

  @override
  Widget build(BuildContext context) {
    final groups = controller.stops(trip);
    final loadedStops = groups.where((g) => g.loaded).length;
    final open = trip.orders.where((o) => o.unresolved).length;
    final (String status, Tone tone) = trip.ready ? ('Ready', Tone.green) : open > 0 ? ('$open shortfall', Tone.red) : trip.started ? ('Loading', Tone.primary) : ('Not started', Tone.muted);
    final code = trip.refrigerated ? 'RT' : trip.vehicleType.toLowerCase().contains('van') ? 'SV' : 'DB';
    final sameVehicle = controller.vehicleTrips(trip);
    final after = controller.earlierTripReturn(trip);
    return WpCard(
      onTap: () => tablet ? controller.select(trip.tripId) : onOpen(),
      borderColor: selected ? Wp.primary : Wp.border,
      borderWidth: selected ? 2 : 1,
      child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
        Row(children: [
          Container(width: 44, height: 44, alignment: Alignment.center, decoration: BoxDecoration(color: Wp.tint, borderRadius: BorderRadius.circular(8)), child: Text(code, style: const TextStyle(color: Wp.primary, fontWeight: FontWeight.w700))),
          const SizedBox(width: 12),
          Expanded(child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
            Text(trip.vehicleId, style: const TextStyle(fontSize: 17, fontWeight: FontWeight.w600)),
            Text(vehicleLine(trip), style: const TextStyle(fontSize: 13, color: Wp.muted)),
          ])),
          Column(crossAxisAlignment: CrossAxisAlignment.end, children: [
            StatusTag(status, tone: tone),
            const SizedBox(height: 4),
            Text(trip.departAt != null ? 'Depart ${clock(trip.departAt)}' : trip.planRef, style: const TextStyle(fontSize: 13, color: Wp.muted)),
          ]),
        ]),
        const SizedBox(height: 12),
        Wrap(spacing: 6, runSpacing: 6, children: [
          StatusTag(trip.refrigerated ? 'Chilled' : 'Ambient', tone: trip.refrigerated ? Tone.cool : Tone.muted),
          StatusTag('Trip ${trip.tripNumber} of ${sameVehicle.length < trip.tripNumber ? trip.tripNumber : sameVehicle.length}'),
          if (trip.vanOnly) const StatusTag('Van-only stops', tone: Tone.amber),
          StatusTag('${groups.length} stops${trip.areas.isNotEmpty ? ' · ${trip.areas.first}' : ''}'),
          if (trip.planChanged || (trip.needsAck && trip.planVersion > 1)) StatusTag('Changed in v${trip.planVersion}', tone: Tone.amber),
        ]),
        if (after != null && !trip.started) Padding(padding: const EdgeInsets.only(top: 8), child: Text('Loads after Trip ${trip.tripNumber - 1} returns ~${clock(after)}', style: const TextStyle(fontSize: 13, color: Wp.muted))),
        const SizedBox(height: 12),
        Row(children: [
          Text('Loaded $loadedStops of ${groups.length} stops', style: const TextStyle(fontWeight: FontWeight.w500)),
          const Spacer(),
          if (trip.weightKg > 0) Text('${kg(trip.weightKg)} · ${m3(trip.volumeM3)}', style: const TextStyle(color: Wp.muted, fontSize: 13)),
        ]),
        const SizedBox(height: 8),
        WpProgress(value: groups.isEmpty ? 0 : loadedStops / groups.length, color: tone == Tone.green ? Wp.green : tone == Tone.red ? Wp.red : Wp.primary, label: 'Stops loaded ${trip.vehicleId}'),
        if (trip.ready && trip.readyAt != null) Padding(padding: const EdgeInsets.only(top: 8), child: Text('Confirmed ${clock(trip.readyAt)}', style: const TextStyle(fontSize: 13, color: Wp.green))),
        if (!tablet && selected) ...[
          const SizedBox(height: 12),
          Container(
            padding: const EdgeInsets.all(12),
            decoration: BoxDecoration(color: Wp.canvas, borderRadius: BorderRadius.circular(8)),
            child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
              Text('Stop sequence · load in reverse (${groups.length} → 1)', style: const TextStyle(fontSize: 12, fontWeight: FontWeight.w600, color: Wp.muted)),
              for (final g in groups)
                Padding(padding: const EdgeInsets.only(top: 6), child: Row(children: [
                  CircleAvatar(radius: 9, backgroundColor: Wp.primary, child: Text('${g.stopSequence}', style: const TextStyle(fontSize: 10, color: Colors.white))),
                  const SizedBox(width: 8),
                  Expanded(child: Text('${g.outletLabel}${g.head.window.isNotEmpty ? ' · ${g.head.window}' : ''}', style: const TextStyle(fontWeight: FontWeight.w500))),
                  if (g.changeNote.isNotEmpty) StatusTag(g.changeNote, tone: Tone.amber) else if (g.chilled) const StatusTag('Chilled', tone: Tone.cool),
                ])),
            ]),
          ),
          const SizedBox(height: 12),
          FilledButton(onPressed: onOpen, child: const Text('Open trip & continue loading')),
        ],
      ]),
    );
  }
}

class StopSequencePanel extends StatelessWidget {
  const StopSequencePanel({super.key, required this.controller, required this.trip, required this.onOpen});
  final LoaderController controller;
  final LoadingTrip trip;
  final VoidCallback onOpen;

  @override
  Widget build(BuildContext context) {
    final groups = controller.stops(trip);
    return WpCard(
      padding: EdgeInsets.zero,
      child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
        Padding(
          padding: const EdgeInsets.all(16),
          child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
            Row(children: [const Expanded(child: Text('Stop sequence', style: TextStyle(fontSize: 18, fontWeight: FontWeight.w600))), StatusTag('Plan v${trip.planVersion}', tone: Tone.primary)]),
            Text('${trip.vehicleId} · Trip ${trip.tripNumber}${trip.departAt != null ? ' · Depart ${clock(trip.departAt)}' : ''} · ${groups.length} stops', style: const TextStyle(fontSize: 13, color: Wp.muted)),
            const SizedBox(height: 8),
            StatusNote(icon: Icons.info, text: 'Drop-off order shown. Load in reverse — Stop ${groups.length} goes in first (front), Stop 1 last (by the doors).'),
          ]),
        ),
        for (final g in groups)
          Container(
            decoration: BoxDecoration(color: g.hasShortfall ? Wp.amberBg : null, border: const Border(top: BorderSide(color: Wp.border))),
            padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
            child: Row(crossAxisAlignment: CrossAxisAlignment.start, children: [
              CircleAvatar(radius: 13, backgroundColor: Wp.primary, child: Text('${g.stopSequence}', style: const TextStyle(fontSize: 12, color: Colors.white, fontWeight: FontWeight.w700))),
              const SizedBox(width: 12),
              Expanded(child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                Text(g.outletLabel, style: const TextStyle(fontWeight: FontWeight.w600)),
                Text([g.outletId, g.head.window, g.head.dock].where((s) => s.isNotEmpty).join(' · '), style: const TextStyle(fontSize: 12, color: Wp.muted)),
                const SizedBox(height: 4),
                Wrap(spacing: 6, runSpacing: 4, children: [
                  if (g.head.dock.isNotEmpty) StatusTag(g.head.dock),
                  if (g.changeNote.isNotEmpty) StatusTag(g.changeNote, tone: Tone.amber),
                  if (g.chilled) const StatusTag('Chilled', tone: Tone.cool),
                  if (g.hasShortfall) const StatusTag('Shortfall reported', tone: Tone.amber),
                  if (g.loaded) const StatusTag('Loaded', tone: Tone.green),
                ]),
                const SizedBox(height: 4),
                Text(g.orders.map((o) => '${o.orderRef} × ${o.expectedUnits}').join(' · '), style: const TextStyle(fontSize: 13)),
              ])),
            ]),
          ),
        Container(
          decoration: const BoxDecoration(border: Border(top: BorderSide(color: Wp.border))),
          padding: const EdgeInsets.all(16),
          child: FilledButton(onPressed: onOpen, child: const Text('Open trip & continue loading')),
        ),
      ]),
    );
  }
}

class _PlanUpdates extends StatelessWidget {
  const _PlanUpdates({required this.controller, required this.trips, required this.onOpen});
  final LoaderController controller;
  final List<LoadingTrip> trips;
  final ValueChanged<LoadingTrip> onOpen;

  @override
  Widget build(BuildContext context) {
    return ListView(padding: const EdgeInsets.all(20), children: [
      ConnectionNote(controller: controller),
      const SectionTitle('Plan updates'),
      if (trips.isEmpty) const WpCard(child: Text('Every load list matches the latest plan version.', style: TextStyle(color: Wp.muted))),
      for (final t in trips) Padding(padding: const EdgeInsets.only(bottom: 12), child: ChangeBanner(controller: controller, trip: t, compact: true, onView: () => showChanges(context, t, onOpen: () => onOpen(t)))),
    ]);
  }
}

class _Issues extends StatelessWidget {
  const _Issues({required this.controller, required this.onOpen});
  final LoaderController controller;
  final ValueChanged<LoadingTrip> onOpen;

  @override
  Widget build(BuildContext context) {
    final rows = [
      for (final t in controller.allTrips)
        for (final o in t.orders)
          for (final i in o.issues) (t, o, i),
    ];
    return ListView(padding: const EdgeInsets.all(20), children: [
      ConnectionNote(controller: controller),
      const SectionTitle('Reported issues'),
      if (rows.isEmpty) const WpCard(child: Text('No missing, damaged or wrong items reported for this date.', style: TextStyle(color: Wp.muted))),
      for (final r in rows)
        Padding(
          padding: const EdgeInsets.only(bottom: 10),
          child: WpCard(
            onTap: () => onOpen(r.$1),
            child: Row(children: [
              Icon(r.$3.hasPhoto ? Icons.photo_camera : Icons.report, color: r.$3.allowsDeparture ? Wp.green : Wp.red),
              const SizedBox(width: 12),
              Expanded(child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                Text('${r.$2.orderRef} · ${r.$3.units} of ${r.$2.expectedUnits} ${r.$3.typeLabel}', style: const TextStyle(fontWeight: FontWeight.w600)),
                Text('${r.$1.vehicleId} · Stop ${r.$2.stopSequence} · reported ${clock(r.$3.reportedAt)}${r.$3.note.isNotEmpty ? ' · ${r.$3.note}' : ''}', style: const TextStyle(fontSize: 13, color: Wp.muted)),
                Text(r.$3.decision != null ? 'Dispatcher ${r.$3.decidedBy ?? ''} · ${clock(r.$3.decidedAt)}${r.$3.decisionNote.isNotEmpty ? ' · ${r.$3.decisionNote}' : ''}' : r.$3.seenAt != null ? 'Seen by the dispatcher at ${clock(r.$3.seenAt)}' : 'Not seen by the dispatcher yet', style: const TextStyle(fontSize: 13, color: Wp.muted)),
              ])),
              StatusTag(r.$3.decisionLabel, tone: r.$3.allowsDeparture ? Tone.green : r.$3.decision == 'HOLD' ? Tone.amber : Tone.red),
            ]),
          ),
        ),
    ]);
  }
}

class _Notifications extends StatelessWidget {
  const _Notifications({required this.controller, required this.onOpen});
  final LoaderController controller;
  final ValueChanged<LoadingTrip> onOpen;

  @override
  Widget build(BuildContext context) {
    final rows = controller.notices();
    Tone tone(String t) => switch (t) { 'amber' => Tone.amber, 'green' => Tone.green, 'red' => Tone.red, 'muted' => Tone.muted, _ => Tone.primary };
    return ListView(padding: const EdgeInsets.all(20), children: [
      ConnectionNote(controller: controller),
      const SectionTitle('Notifications'),
      if (rows.isEmpty) const WpCard(child: Text('Plan updates and dispatcher decisions for this date appear here.', style: TextStyle(color: Wp.muted))),
      for (final n in rows)
        Padding(
          padding: const EdgeInsets.only(bottom: 10),
          child: WpCard(
            onTap: () => onOpen(n.trip),
            child: Row(children: [
              StatusTag(clock(n.at).isEmpty ? '—' : clock(n.at), tone: tone(n.tone)),
              const SizedBox(width: 12),
              Expanded(child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                Text(n.title, style: const TextStyle(fontWeight: FontWeight.w600)),
                Text(n.text, style: const TextStyle(fontSize: 13, color: Wp.muted)),
              ])),
              const Icon(Icons.chevron_right, color: Wp.muted),
            ]),
          ),
        ),
    ]);
  }
}
