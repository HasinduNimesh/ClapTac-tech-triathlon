import 'package:flutter/material.dart';

import '../shared/models.dart';
import '../sync/sync.dart';
import '../theme/tokens.dart';
import '../widgets/common.dart';
import 'loader_controller.dart';
import 'trip_detail_screen.dart';

class LoaderHome extends StatefulWidget {
  const LoaderHome({super.key, required this.controller, required this.profile, required this.onSwitchUser});

  final LoaderController controller;
  final Profile profile;
  final VoidCallback onSwitchUser;

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
    Navigator.of(context).push(MaterialPageRoute(builder: (_) => TripDetailScreen(controller: widget.controller, tripId: trip.tripId, profile: widget.profile)));
  }

  @override
  Widget build(BuildContext context) {
    final c = widget.controller;
    return ListenableBuilder(
      listenable: Listenable.merge([c, c.sync]),
      builder: (context, _) => LayoutBuilder(builder: (context, box) {
        final tablet = box.maxWidth >= Wp.tabletBreakpoint;
        final changed = c.details.values.where((t) => t.planChanged).toList();
        final issues = c.details.values.fold<int>(0, (s, t) => s + t.shortfallCount);
        final body = switch (tab) {
          1 => _PlanUpdates(controller: c, trips: changed),
          2 => _Issues(controller: c, onOpen: _open),
          _ => _LoadList(controller: c, tablet: tablet, onOpen: _open),
        };
        if (tablet) {
          return Scaffold(
            body: Column(children: [
              LoaderTabletHeader(controller: c, profile: widget.profile, onSwitchUser: widget.onSwitchUser),
              Container(
                color: Wp.surface,
                padding: const EdgeInsets.symmetric(horizontal: 28),
                child: Row(children: [
                  _Tab('Load list', 0, tab, (i) => setState(() => tab = i)),
                  _Tab('Plan updates', 1, tab, (i) => setState(() => tab = i), count: changed.length, tone: Tone.amber),
                  _Tab('Reported issues', 2, tab, (i) => setState(() => tab = i), count: issues, tone: Tone.red),
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
            onDestinationSelected: (i) => i == 3 ? widget.onSwitchUser() : setState(() => tab = i),
            destinations: [
              const NavigationDestination(icon: Icon(Icons.inventory_2_outlined), label: 'Load list'),
              NavigationDestination(icon: Badge(isLabelVisible: changed.isNotEmpty, label: Text('${changed.length}'), child: const Icon(Icons.update)), label: 'Plan updates'),
              NavigationDestination(icon: Badge(isLabelVisible: issues > 0, label: Text('$issues'), child: const Icon(Icons.report_outlined)), label: 'Issues'),
              const NavigationDestination(icon: Icon(Icons.person_outline), label: 'Switch user'),
            ],
          ),
        );
      }),
    );
  }
}

class LoaderTabletHeader extends StatelessWidget {
  const LoaderTabletHeader({super.key, required this.controller, required this.profile, required this.onSwitchUser});

  final LoaderController controller;
  final Profile profile;
  final VoidCallback onSwitchUser;

  @override
  Widget build(BuildContext context) {
    final s = controller.sync.state;
    final offline = controller.offline || s.phase == SyncPhase.offline;
    final name = profile.displayName ?? profile.userId;
    return Container(
      color: Wp.surface,
      padding: EdgeInsets.fromLTRB(28, MediaQuery.of(context).padding.top + 10, 28, 10),
      child: Row(children: [
        Image.asset('assets/waypoint-logo.png', height: 36, semanticLabel: 'Waypoint'),
        const SizedBox(width: 16),
        const Text('Loader workspace', style: TextStyle(color: Wp.muted, fontWeight: FontWeight.w500)),
        const Spacer(),
        StatusTag(offline ? 'Offline · ${s.pending} saved on this tablet' : s.phase == SyncPhase.error ? 'Sync needs attention' : 'Live · plan synced', tone: offline ? Tone.amber : s.phase == SyncPhase.error ? Tone.red : Tone.green, icon: offline ? Icons.wifi_off : Icons.circle),
        const SizedBox(width: 12),
        Container(padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8), decoration: BoxDecoration(border: Border.all(color: Wp.border), borderRadius: BorderRadius.circular(8)), child: Text('${depotName(profile.depot)} DC')),
        const SizedBox(width: 12),
        CircleAvatar(backgroundColor: Wp.tint, foregroundColor: Wp.primary, child: Text(name.isEmpty ? 'L' : name.substring(0, name.length >= 2 ? 2 : 1).toUpperCase(), style: const TextStyle(fontWeight: FontWeight.w700))),
        const SizedBox(width: 10),
        Column(crossAxisAlignment: CrossAxisAlignment.start, mainAxisSize: MainAxisSize.min, children: [Text(name, style: const TextStyle(fontWeight: FontWeight.w600)), const Text('Loader', style: TextStyle(fontSize: 12, color: Wp.muted))]),
        const SizedBox(width: 12),
        TextButton.icon(onPressed: onSwitchUser, icon: const Icon(Icons.switch_account_outlined), label: const Text('Switch user')),
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
  const LoaderBanner({super.key, required this.title, required this.subtitle, this.phone = false});
  final String title;
  final String subtitle;
  final bool phone;

  @override
  Widget build(BuildContext context) {
    return Container(
      width: double.infinity,
      padding: EdgeInsets.fromLTRB(phone ? 20 : 28, phone ? MediaQuery.of(context).padding.top + 16 : 22, phone ? 20 : 28, 22),
      decoration: const BoxDecoration(gradient: LinearGradient(colors: [Color(0xFF0B2A8F), Color(0xFF1E4FD8)], begin: Alignment.centerLeft, end: Alignment.centerRight)),
      child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
        Text(title, style: TextStyle(color: Colors.white, fontSize: phone ? 24 : 26, fontWeight: FontWeight.w700)),
        const SizedBox(height: 4),
        Text(subtitle, style: const TextStyle(color: Colors.white)),
      ]),
    );
  }
}

class _LoadList extends StatelessWidget {
  const _LoadList({required this.controller, required this.tablet, required this.onOpen});
  final LoaderController controller;
  final bool tablet;
  final ValueChanged<LoadingTrip> onOpen;

  @override
  Widget build(BuildContext context) {
    final c = controller;
    final trips = c.trips.map((t) => c.details[t.tripId] ?? t).toList();
    final changed = trips.where((t) => t.planChanged).toList();
    final selected = c.selected;
    final cards = [
      for (final t in trips) Padding(padding: const EdgeInsets.only(bottom: 12), child: _VehicleCard(controller: c, trip: t, selected: t.tripId == c.selectedTripId, tablet: tablet, onOpen: () => onOpen(t))),
    ];
    final content = <Widget>[
      for (final t in changed) Padding(padding: const EdgeInsets.only(bottom: 16), child: _ChangeBanner(controller: c, trip: t, compact: !tablet, onView: () => onOpen(t))),
      if (c.error.isNotEmpty) Padding(padding: const EdgeInsets.only(bottom: 12), child: StatusNote(tone: Tone.red, text: c.error)),
      if (c.offline) const Padding(padding: EdgeInsets.only(bottom: 12), child: StatusNote(tone: Tone.amber, icon: Icons.wifi_off, title: 'No connection', text: 'Showing the load list saved on this device. Loading and reports are saved and sent when connected.')),
      _Stats(trips: trips, tablet: tablet),
      const SizedBox(height: 16),
      SectionTitle('Assigned vehicles & trips', trailing: StatusTag('All ${trips.length}', tone: Tone.primary)),
      if (c.loading && trips.isEmpty) const Padding(padding: EdgeInsets.all(32), child: Center(child: CircularProgressIndicator())),
      if (!c.loading && trips.isEmpty) const WpCard(child: Text('No trips are assigned to your dock today.')),
    ];
    return ListView(padding: EdgeInsets.zero, children: [
      LoaderBanner(phone: !tablet, title: "Today's load list", subtitle: '${todayLabel()} · ${trips.isNotEmpty ? '${depotName(trips.first.depot)} · ' : ''}Manifest plan v${selected?.planVersion ?? 1}'),
      Padding(
        padding: EdgeInsets.all(tablet ? 28 : 16),
        child: tablet && selected != null
            ? Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
                ...content,
                Row(crossAxisAlignment: CrossAxisAlignment.start, children: [
                  Expanded(child: Column(children: cards)),
                  const SizedBox(width: 20),
                  SizedBox(width: 440, child: StopSequencePanel(controller: c, trip: selected, onOpen: () => onOpen(selected))),
                ]),
              ])
            : Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [...content, ...cards]),
      ),
    ]);
  }
}

class _ChangeBanner extends StatelessWidget {
  const _ChangeBanner({required this.controller, required this.trip, required this.compact, required this.onView});
  final LoaderController controller;
  final LoadingTrip trip;
  final bool compact;
  final VoidCallback onView;

  @override
  Widget build(BuildContext context) {
    final buttons = [
      OutlinedButton(onPressed: onView, child: const Text('View changes')),
      const SizedBox(width: 10, height: 10),
      FilledButton(onPressed: () => controller.acknowledgePlan(trip), child: Text(compact ? 'Acknowledge' : 'Acknowledge revised load')),
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
              Text('Plan updated to v${trip.planVersion} — recheck ${trip.vehicleId} before loading further', style: const TextStyle(fontWeight: FontWeight.w600, fontSize: 16)),
              const SizedBox(height: 4),
              Text('You acknowledged v${trip.acknowledgedVersion}. Stops, quantities or load order may have changed.', style: const TextStyle(fontSize: 14)),
              const Text('Goods already loaded may need to move.', style: TextStyle(fontSize: 12, color: Wp.amber)),
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
  const _Stats({required this.trips, required this.tablet});
  final List<LoadingTrip> trips;
  final bool tablet;

  @override
  Widget build(BuildContext context) {
    final ready = trips.where((t) => t.ready).length;
    final loading = trips.where((t) => t.started && !t.ready).length;
    final shortfalls = trips.fold<int>(0, (s, t) => s + t.shortfallCount);
    final items = [
      ('Vehicles assigned', '${trips.map((t) => t.vehicleId).toSet().length}', '${trips.length} trips today', Wp.ink),
      ('Ready to depart', '$ready of ${trips.length}', ready > 0 ? 'Confirmed' : 'None yet', Wp.green),
      ('Loading now', '$loading', 'In progress', Wp.primary),
      ('Open shortfalls', '$shortfalls', 'Awaiting dispatcher decision', Wp.red),
    ];
    final tiles = [
      for (final i in (tablet ? items : [items[1], items[3]]))
        Expanded(child: WpCard(child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [Text(i.$1, style: const TextStyle(fontSize: 13, color: Wp.muted, fontWeight: FontWeight.w500)), Text(i.$2, style: TextStyle(fontSize: 26, fontWeight: FontWeight.w700, color: i.$4)), Text(i.$3, style: const TextStyle(fontSize: 12, color: Wp.muted))]))),
    ];
    return Row(children: [for (var i = 0; i < tiles.length; i++) ...[if (i > 0) SizedBox(width: tablet ? 16 : 10), tiles[i]]]);
  }
}

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
    final (String status, Tone tone) = trip.ready ? ('Ready', Tone.green) : trip.shortfallCount > 0 ? ('${trip.shortfallCount} shortfall', Tone.red) : trip.started ? ('Loading', Tone.primary) : ('Not started', Tone.muted);
    final code = trip.refrigerated ? 'RT' : trip.vehicleType.toLowerCase().contains('van') ? 'SV' : 'DB';
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
            Text([trip.vehicleType, trip.capability].where((s) => s.isNotEmpty).join(' · '), style: const TextStyle(fontSize: 13, color: Wp.muted)),
          ])),
          Column(crossAxisAlignment: CrossAxisAlignment.end, children: [StatusTag(status, tone: tone), const SizedBox(height: 4), Text(trip.planRef, style: const TextStyle(fontSize: 13, color: Wp.muted))]),
        ]),
        const SizedBox(height: 12),
        Wrap(spacing: 6, runSpacing: 6, children: [
          StatusTag(trip.refrigerated ? 'Chilled' : 'Ambient', tone: trip.refrigerated ? Tone.cool : Tone.muted),
          StatusTag('Trip ${trip.tripNumber} of 2'),
          StatusTag('${groups.length} stops'),
          if (trip.planChanged) StatusTag('Changed in v${trip.planVersion}', tone: Tone.amber),
        ]),
        const SizedBox(height: 12),
        Row(children: [Text('Loaded $loadedStops of ${groups.length} stops', style: const TextStyle(fontWeight: FontWeight.w500)), const Spacer(), Text('${trip.loadedCount} / ${trip.orders.length} orders', style: const TextStyle(color: Wp.muted, fontSize: 13))]),
        const SizedBox(height: 8),
        WpProgress(value: groups.isEmpty ? 0 : loadedStops / groups.length, color: tone == Tone.green ? Wp.green : tone == Tone.red ? Wp.red : Wp.primary, label: 'Stops loaded ${trip.vehicleId}'),
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
                  Expanded(child: Text(g.outletId, style: const TextStyle(fontWeight: FontWeight.w500))),
                  if (g.chilled) const StatusTag('Chilled', tone: Tone.cool),
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
            Text('${trip.vehicleId} · Trip ${trip.tripNumber} · ${groups.length} stops', style: const TextStyle(fontSize: 13, color: Wp.muted)),
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
                Text(g.outletId, style: const TextStyle(fontWeight: FontWeight.w600)),
                const SizedBox(height: 4),
                Wrap(spacing: 6, runSpacing: 4, children: [if (g.chilled) const StatusTag('Chilled', tone: Tone.cool), if (g.hasShortfall) const StatusTag('Shortfall reported', tone: Tone.amber), if (g.loaded) const StatusTag('Loaded', tone: Tone.green)]),
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
  const _PlanUpdates({required this.controller, required this.trips});
  final LoaderController controller;
  final List<LoadingTrip> trips;

  @override
  Widget build(BuildContext context) {
    return ListView(padding: const EdgeInsets.all(20), children: [
      const SectionTitle('Plan updates'),
      if (trips.isEmpty) const WpCard(child: Text('Every load list matches the latest plan version.', style: TextStyle(color: Wp.muted))),
      for (final t in trips) Padding(padding: const EdgeInsets.only(bottom: 12), child: _ChangeBanner(controller: controller, trip: t, compact: true, onView: () {})),
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
      for (final t in controller.details.values)
        for (final o in t.orders)
          for (final i in o.issues) (t, o, i),
    ];
    return ListView(padding: const EdgeInsets.all(20), children: [
      const SectionTitle('Reported issues'),
      if (rows.isEmpty) const WpCard(child: Text('No missing or damaged items reported today.', style: TextStyle(color: Wp.muted))),
      for (final r in rows)
        Padding(
          padding: const EdgeInsets.only(bottom: 10),
          child: WpCard(
            onTap: () => onOpen(r.$1),
            child: Row(children: [
              const Icon(Icons.report, color: Wp.red),
              const SizedBox(width: 12),
              Expanded(child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                Text('${r.$2.orderRef} · ${r.$3.units} ${r.$3.type.toLowerCase()}', style: const TextStyle(fontWeight: FontWeight.w600)),
                Text('${r.$1.vehicleId} · Stop ${r.$2.stopSequence}${r.$3.note.isNotEmpty ? ' · ${r.$3.note}' : ''}', style: const TextStyle(fontSize: 13, color: Wp.muted)),
              ])),
              StatusTag(r.$3.decisionLabel, tone: r.$3.allowsDeparture ? Tone.green : Tone.red),
            ]),
          ),
        ),
    ]);
  }
}
