import 'package:flutter/material.dart';

import '../data/driver_models.dart';
import '../maps/map_launcher.dart';
import '../maps/stop_directions.dart';
import '../proof/proof_capturer.dart';
import '../screens/delivery/record_delivery_screen.dart';
import '../screens/delivery/stop_details_screen.dart';
import '../screens/delivery/take_back_sheet.dart';
import '../screens/route/report_problem_sheet.dart';
import '../screens/route/route_home_screen.dart';
import '../screens/route/safe_stop_screen.dart';
import '../screens/route/truck_checkout_sheet.dart';
import '../screens/sign_in/sign_in_screen.dart';
import '../screens/states/end_of_day_screen.dart';
import '../screens/states/plan_conflict_screen.dart';
import '../screens/states/sync_state_screen.dart';
import '../screens/updates/updates_screen.dart';
import '../theme/tokens.dart';
import '../widgets/app_buttons.dart';
import '../widgets/driver_shell.dart';
import '../widgets/note_banner.dart';
import 'driver_session.dart';

/// Signed-out → SignInScreen; signed-in → the tabbed driver home.
class DriverFlow extends StatelessWidget {
  const DriverFlow({super.key, required this.session, this.capturer, this.mapLauncher});

  final DriverSession session;

  /// Takes real photos and signatures. Null in demo and test builds, where the proof buttons say
  /// that capture is not available.
  final ProofCapturer? capturer;

  /// Opens directions to a stop in the phone's maps app. Null in tests that do not exercise it, where
  /// the button says maps are not available.
  final MapLauncher? mapLauncher;

  @override
  Widget build(BuildContext context) {
    return ListenableBuilder(
      listenable: session,
      builder: (context, _) {
        if (session.restoring) {
          return const Scaffold(body: Center(child: CircularProgressIndicator()));
        }
        if (!session.signedIn) {
          return SignInScreen(
            errorMessage: session.signInError,
            identityProviderMode: session.usesIdentityProvider,
            // Signing in needs the browser and the identity provider, so without a network it waits.
            noSignal: !session.deviceOnline,
            busy: session.signingIn,
            onSignIn: (staffId, password) => session.signIn(),
          );
        }
        return _DriverHome(session: session, capturer: capturer, mapLauncher: mapLauncher);
      },
    );
  }
}

class _DriverHome extends StatefulWidget {
  const _DriverHome({required this.session, this.capturer, this.mapLauncher});

  final DriverSession session;
  final ProofCapturer? capturer;
  final MapLauncher? mapLauncher;

  @override
  State<_DriverHome> createState() => _DriverHomeState();
}

class _DriverHomeState extends State<_DriverHome> {
  DriverSession get session => widget.session;

  /// The load check opens once per route, whether the route was there from the start or arrived
  /// later from the server.
  String? _loadCheckTripId;

  void _openLoadCheckOnce() {
    // Once per trip: a driver who finishes one trip and is given the next confirms that load too.
    if (!session.hasRoute || session.loadResolved || _loadCheckTripId == session.trip.tripId) return;
    _loadCheckTripId = session.trip.tripId;
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted) _showLoadCheck();
    });
  }

  void _showLoadCheck() {
    if (session.loadResolved) return;
    showTruckCheckoutSheet(
      context,
      trip: session.trip,
      onConfirm: () async {
        await session.confirmLoad();
        if (!mounted) return;
        _notify(_afterLoadDecision('Load confirmed'));
      },
      onMissingItem: _reportMissingLoadItem,
    );
  }

  Future<void> _reportMissingLoadItem() async {
    await session.reportLoadDiscrepancy();
    if (!mounted) return;
    final departAnyway = await showDialog<bool>(
      context: context,
      barrierDismissible: false,
      builder: (dialogContext) => AlertDialog(
        title: const Text('Missing item reported'),
        content: const Text(
          'The report is queued on this phone only. It has not reached the loader or dispatch.\n\n'
          'Do not leave until the load matches your list, unless you accept departing with it as it is.',
        ),
        actions: [
          TextButton(onPressed: () => Navigator.of(dialogContext).pop(false), child: const Text('Back to load check')),
          TextButton(onPressed: () => Navigator.of(dialogContext).pop(true), child: const Text('Depart anyway')),
        ],
      ),
    );
    if (!mounted) return;
    if (departAnyway == true) {
      await session.overrideLoadCheck();
      if (!mounted) return;
      _notify(_afterLoadDecision('Departure recorded'));
    } else {
      _showLoadCheck();
    }
  }

  /// What the driver is told after confirming the load or departing anyway, including whether the
  /// trip actually started on the server.
  String _afterLoadDecision(String decision) {
    switch (session.tripStartState) {
      case TripStartState.started:
        return '$decision and trip started.';
      case TripStartState.waiting:
        return '$decision on this phone. The trip is not started yet: it starts when Waypoint can be reached.';
      case TripStartState.refused:
        return '$decision on this phone, but the trip did not start. ${session.tripStartMessage ?? ''}'.trim();
      case TripStartState.starting:
      case TripStartState.notStarted:
        return '$decision on this phone. Nothing has been sent to dispatch.';
    }
  }

  void _notify(String message) {
    ScaffoldMessenger.of(context)
      ..hideCurrentSnackBar()
      ..showSnackBar(SnackBar(
        content: Text(message),
        behavior: SnackBarBehavior.floating,
        duration: const Duration(seconds: 3),
        showCloseIcon: true,
        margin: const EdgeInsets.fromLTRB(16, 0, 16, 16),
      ));
  }

  void _selectTab(DriverTab tab) {
    Navigator.of(context).popUntil((route) => route.isFirst);
    session.selectTab(tab);
  }

  void _reportProblem(StopInfo? stop) {
    showReportProblemSheet(
      context,
      trip: session.trip,
      stop: stop,
      onSend: (report) async {
        await session.reportProblem(report, stop: stop);
        if (!mounted) return;
        _notify('Report queued on this phone. It has not been sent to dispatch yet.');
      },
    );
  }

  /// Opens the stop in the phone's maps app. An exact recorded position is navigated to; otherwise it
  /// searches for the shop by name and says the exact location is not recorded (never driving the
  /// driver to a district centre as if it were the shop).
  Future<void> _openMaps(StopInfo stop) async {
    final launcher = widget.mapLauncher;
    if (launcher == null) {
      _notify('Maps are not available in this build.');
      return;
    }
    final directions = StopDirections.forStop(stop);
    if (!directions.exact) {
      _notify('Exact location not recorded. Searching for ${directions.searchText}; check the name and area before driving, or ask dispatch.');
    }
    final opened = await launcher.open(directions.uri);
    if (!mounted || opened) return;
    _notify('Could not open a maps app on this phone. Address: ${stop.label}.');
  }

  void _startStop() {
    if (!session.loadResolved) {
      _showLoadCheck();
      return;
    }
    final stop = session.trip.nextStop;
    if (stop == null) return;
    Navigator.of(context).push(MaterialPageRoute<void>(
      builder: (context) => SafeStopScreen(
        trip: session.trip,
        stop: stop,
        earlyMinutes: 0,
        onTabSelected: _selectTab,
        onOpenMaps: () => _openMaps(stop),
        onStoppedSafely: () async {
          final navigator = Navigator.of(context);
          await session.markArrived(stop);
          if (!mounted) return;
          // Rebuilt when the connection changes, so the offline variant appears and goes away by itself.
          navigator.pushReplacement(MaterialPageRoute<void>(
            builder: (context) => ListenableBuilder(listenable: session, builder: (context, _) => _stopDetails(context, stop)),
          ));
        },
      ),
    ));
  }

  /// Asks for a real photo or signature, or says capture is not available in this build.
  Future<CapturedProof?> _captureProof(ProofKind kind) async {
    final capturer = widget.capturer;
    if (capturer == null) {
      _notify('Photo and signature capture is not available in this build, so no proof is stored.');
      return null;
    }
    return capturer.capture(context, kind);
  }

  void _discardProof(CapturedProof proof) => widget.capturer?.discard(proof);

  Widget _stopDetails(BuildContext context, StopInfo stop) {
    return StopDetailsScreen(
      stop: stop,
      offline: session.offline,
      onTabSelected: _selectTab,
      onReportIssue: () => _reportProblem(stop),
      onProofRequested: _captureProof,
      onProofDiscarded: _discardProof,
      requireProof: widget.capturer != null,
      onSave: (draft) {
        switch (draft.outcome) {
          case DeliveryOutcome.delivered:
            _commit(context, stop, draft);
          case DeliveryOutcome.partial:
            final expected = stop.units;
            if (expected == null) {
              _notify('The expected quantity for this stop is not recorded, so a partial delivery cannot be entered. Report a problem instead.');
              return;
            }
            Navigator.of(context).push(MaterialPageRoute<void>(
              builder: (context) => RecordDeliveryScreen(
                stop: stop,
                orderRef: stop.orderRef,
                expectedQuantity: expected,
                unit: stop.unitLabel,
                initialOutcome: DeliveryOutcome.partial,
                onProofRequested: _captureProof,
                onProofDiscarded: _discardProof,
                requireProof: widget.capturer != null,
                initialProofs: draft.proofs,
                onTabSelected: _selectTab,
                onRejected: () => _takeBack(context, stop, const DeliveryDraft(outcome: DeliveryOutcome.refused)),
                onSave: (partial) => _commit(context, stop, DeliveryDraft(
                  outcome: partial.outcome,
                  quantity: partial.quantity,
                  notes: draft.notes,
                  reason: partial.reason,
                  hasPhoto: partial.hasPhoto,
                  hasSignature: partial.hasSignature,
                  proofs: partial.proofs,
                )),
              ),
            ));
          case DeliveryOutcome.failed:
          case DeliveryOutcome.refused:
            _takeBack(context, stop, draft);
        }
      },
    );
  }

  void _takeBack(BuildContext context, StopInfo stop, DeliveryDraft draft) {
    showTakeBackSheet(
      context,
      stop: stop,
      orderRef: stop.orderRef,
      onSave: (reattempt) => _commit(
        context,
        stop,
        DeliveryDraft(
          outcome: draft.outcome,
          quantity: draft.quantity,
          notes: reattempt ? 'Re-attempt on the next run' : 'Dispatcher asked to defer',
          reason: draft.reason,
        ),
      ),
    );
  }

  Future<void> _commit(BuildContext context, StopInfo stop, DeliveryDraft draft) async {
    final navigator = Navigator.of(context);
    await session.recordDelivery(stop, draft);
    if (!mounted) return;
    navigator.pushAndRemoveUntil(
      MaterialPageRoute<void>(
        builder: (context) => SyncStateScreen(
          kind: SyncStateKind.savedOffline,
          stop: stop,
          savedAt: clockLabel(DateTime.now()),
          outcomeText: _outcomeText(stop, draft),
          hasPhoto: draft.hasPhoto,
          onTabSelected: _selectTab,
          onPrimary: () {
            Navigator.of(context).popUntil((route) => route.isFirst);
            session.selectTab(session.routeComplete ? DriverTab.summary : DriverTab.route);
          },
        ),
      ),
      (route) => route.isFirst,
    );
  }

  String _outcomeText(StopInfo stop, DeliveryDraft draft) {
    switch (draft.outcome) {
      case DeliveryOutcome.delivered:
        return 'Delivered - ${stop.unitsText}';
      case DeliveryOutcome.partial:
        return 'Partial - ${draft.quantity ?? 0} of ${stop.unitsText}';
      case DeliveryOutcome.refused:
        return 'Refused - ${stop.unitsText} taken back';
      case DeliveryOutcome.failed:
        return 'Not delivered - ${stop.unitsText} taken back';
    }
  }

  /// Tells the driver, wherever they are in the app, that dispatch sent something new.
  late int _noticedMessageSerial = session.newMessageSerial;

  void _noticeNewMessage() {
    if (session.newMessageSerial == _noticedMessageSerial) return;
    _noticedMessageSerial = session.newMessageSerial;
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted) _notify('Dispatch sent you a message. Open Updates to read and acknowledge it.');
    });
  }

  void _openUpdate(UpdateItem item) {
    final id = item.id;
    if (id != null && id.startsWith(messageUpdatePrefix)) {
      showDialog<void>(
        context: context,
        builder: (dialogContext) => _MessageDialog(session: session, messageId: id.substring(messageUpdatePrefix.length)),
      );
      return;
    }
    if (id != DriverSession.planConflictId) return;
    Navigator.of(context).push(MaterialPageRoute<void>(
      builder: (context) => PlanConflictScreen(
        info: samplePlanConflict,
        onTabSelected: _selectTab,
        onReturnToRoute: () => _selectTab(DriverTab.route),
        onSendForReview: () {
          _selectTab(DriverTab.route);
          _notify('Demo only: nothing was sent to dispatch.');
        },
      ),
    ));
  }

  Future<void> _finishTrip() async {
    Navigator.of(context).popUntil((route) => route.isFirst);
    final result = await session.completeTrip();
    if (!mounted) return;
    switch (result.kind) {
      case TripWrapUp.signedOut:
        return;
      case TripWrapUp.nextTrip:
        _notify('Trip ${result.finished!.tripRef} is complete and sent. Your next trip is ${result.next!.tripRef}: check its load to start it.');
      case TripWrapUp.unsent:
        await askAboutUnsent(context, session, result.unsent);
    }
  }

  @override
  Widget build(BuildContext context) {
    return ListenableBuilder(
      listenable: session,
      builder: (context, _) {
        if (!session.hasRoute && session.tab != DriverTab.updates) {
          _loadCheckTripId = null;
          return _NoRoute(session: session);
        }
        _openLoadCheckOnce();
        _noticeNewMessage();
        switch (session.tab) {
          case DriverTab.route:
            return RouteHomeScreen(
              trip: session.trip,
              offline: session.offline,
              savedCopy: session.showingSavedRoute,
              onViewStop: _startStop,
              onReportProblem: () => _reportProblem(session.trip.nextStop),
              onTabSelected: session.selectTab,
            );
          case DriverTab.updates:
            return UpdatesScreen(items: session.visibleUpdates, onOpen: _openUpdate, onTabSelected: session.selectTab);
          case DriverTab.summary:
            if (!session.routeComplete) return _InProgressSummary(trip: session.trip, onTabSelected: session.selectTab);
            return EndOfDayScreen(
              summary: session.summary,
              onFinishTrip: _finishTrip,
              onRetryUpload: () => session.retrySync(),
              onTabSelected: session.selectTab,
            );
        }
      },
    );
  }
}

/// Signs out, but first lets the driver know when updates are still saved only on this phone. Those
/// are never discarded: signing out anyway keeps them queued, to be sent at the next sign-in.
Future<void> finishOrAsk(BuildContext context, DriverSession session) async {
  final unsent = await session.finishTrip();
  if (unsent == 0 || !context.mounted) return;
  await askAboutUnsent(context, session, unsent);
}

/// Explains that [unsent] updates are saved only on this phone and offers to stay signed in or to
/// sign out anyway.
Future<void> askAboutUnsent(BuildContext context, DriverSession session, int unsent) async {
  final signOutAnyway = await showDialog<bool>(
    context: context,
    builder: (dialogContext) => AlertDialog(
      title: const Text('Updates not sent yet'),
      content: Text(
        '${unsent == 1 ? '1 update is' : '$unsent updates are'} still saved on this phone and could not be sent to Waypoint.\n\n'
        'Stay signed in and reconnect to send ${unsent == 1 ? 'it' : 'them'}, or sign out now. Nothing is lost: '
        '${unsent == 1 ? 'it' : 'they'} will be sent the next time you sign in.',
      ),
      actions: [
        TextButton(onPressed: () => Navigator.of(dialogContext).pop(false), child: const Text('Stay signed in')),
        TextButton(onPressed: () => Navigator.of(dialogContext).pop(true), child: const Text('Sign out anyway')),
      ],
    ),
  );
  if (signOutAnyway == true) await session.finishTrip(force: true);
}

/// Shown when a driver is signed in but there is no route to show: it is still loading, it could
/// not be loaded, or the server has no trip for this driver today. Sample data is only used in
/// explicit demo builds.
class _NoRoute extends StatelessWidget {
  const _NoRoute({required this.session});

  final DriverSession session;

  @override
  Widget build(BuildContext context) {
    final identity = session.identity;
    final who = identity == null
        ? ''
        : ' Signed in as ${identity.userId}${identity.vehicleId == null ? '' : ' (vehicle ${identity.vehicleId})'}.';
    final error = session.tripsError;
    final loading = session.tripsLoading;
    return DriverShell(
      tab: session.tab,
      onTabSelected: session.selectTab,
      body: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Semantics(header: true, child: Text(loading ? 'Loading your route' : 'No route loaded', style: AppText.of(22, FontWeight.w700))),
          const SizedBox(height: 12),
          if (loading)
            const Center(child: Padding(padding: EdgeInsets.all(24), child: CircularProgressIndicator()))
          else if (error != null)
            NoteBanner(title: 'Could not load your route', text: error, tone: NoteTone.danger)
          else if (session.trips == null)
            NoteBanner(
              title: 'Assigned trips are not connected',
              text: 'This build signs you in but does not load your trips, so there is no route to show.$who',
            )
          else
            NoteBanner(
              title: 'No trip for you today',
              text: 'Waypoint has no loaded trip for your vehicle today. Ask dispatch if you expected one.$who',
            ),
          const SizedBox(height: 24),
          if (!loading && session.trips != null) ...[
            AppButton(label: 'Try again', onPressed: session.loadTrips),
            const SizedBox(height: 12),
          ],
          AppButton(label: 'Sign out', outline: true, onPressed: () => finishOrAsk(context, session)),
        ],
      ),
    );
  }
}

class _InProgressSummary extends StatelessWidget {
  const _InProgressSummary({required this.trip, required this.onTabSelected});

  final TripInfo trip;
  final ValueChanged<DriverTab> onTabSelected;

  @override
  Widget build(BuildContext context) {
    return DriverShell(
      tab: DriverTab.summary,
      onTabSelected: onTabSelected,
      body: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Semantics(header: true, child: Text('Summary', style: AppText.of(22, FontWeight.w700))),
          const SizedBox(height: 12),
          NoteBanner(
            title: 'Route in progress',
            text: '${trip.completedStops} of ${trip.stops.length} stops completed. The end-of-day summary appears when the last stop is recorded.',
          ),
        ],
      ),
    );
  }
}

/// One of dispatch's messages in full. An unread message can be acknowledged here, which needs a
/// connection; when it fails the reason is shown and the message stays unread.
class _MessageDialog extends StatefulWidget {
  const _MessageDialog({required this.session, required this.messageId});

  final DriverSession session;
  final String messageId;

  @override
  State<_MessageDialog> createState() => _MessageDialogState();
}

class _MessageDialogState extends State<_MessageDialog> {
  bool _busy = false;
  String? _error;

  Future<void> _acknowledge() async {
    setState(() {
      _busy = true;
      _error = null;
    });
    final problem = await widget.session.acknowledgeMessage(widget.messageId);
    if (!mounted) return;
    if (problem == null) {
      Navigator.of(context).pop();
    } else {
      setState(() {
        _busy = false;
        _error = problem;
      });
    }
  }

  @override
  Widget build(BuildContext context) {
    return ListenableBuilder(
      listenable: widget.session,
      builder: (context, _) {
        final matches = widget.session.messages.where((message) => message.id == widget.messageId);
        if (matches.isEmpty) {
          return AlertDialog(
            title: const Text('Message from dispatch'),
            content: const Text('This message is no longer available.'),
            actions: [TextButton(onPressed: () => Navigator.of(context).pop(), child: const Text('Close'))],
          );
        }
        final message = matches.first;
        final stops = widget.session.hasRoute ? widget.session.trip.stops.where((stop) => stop.stopId == message.stopId) : const <StopInfo>[];
        final about = stops.isEmpty ? 'About the whole trip' : 'About stop ${stops.first.sequence} - ${stops.first.name}';
        return AlertDialog(
          title: const Text('Message from dispatch'),
          content: SingleChildScrollView(
            child: Column(
              mainAxisSize: MainAxisSize.min,
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text('${businessClockLabel(message.createdAt)} \u00b7 $about', style: AppText.of(12, FontWeight.w400, color: AppColors.muted)),
                const SizedBox(height: 12),
                Text(message.body, style: AppText.of(16, FontWeight.w500)),
                if (message.acknowledged) ...[
                  const SizedBox(height: 12),
                  Text('You acknowledged this message.', style: AppText.of(13, FontWeight.w400, color: AppColors.green)),
                ],
                if (_error != null) ...[
                  const SizedBox(height: 12),
                  Semantics(liveRegion: true, child: Text(_error!, style: AppText.of(13, FontWeight.w500, color: AppColors.red))),
                ],
              ],
            ),
          ),
          actions: [
            TextButton(onPressed: _busy ? null : () => Navigator.of(context).pop(), child: const Text('Close')),
            if (!message.acknowledged) TextButton(onPressed: _busy ? null : _acknowledge, child: Text(_busy ? 'Acknowledging\u2026' : 'Acknowledge')),
          ],
        );
      },
    );
  }
}
