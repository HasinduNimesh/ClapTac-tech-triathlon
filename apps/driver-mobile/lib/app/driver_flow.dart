import 'package:flutter/material.dart';

import '../data/driver_models.dart';
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
  const DriverFlow({super.key, required this.session});

  final DriverSession session;

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
            busy: session.signingIn,
            onSignIn: (staffId, password) => session.signIn(),
          );
        }
        return _DriverHome(session: session);
      },
    );
  }
}

class _DriverHome extends StatefulWidget {
  const _DriverHome({required this.session});

  final DriverSession session;

  @override
  State<_DriverHome> createState() => _DriverHomeState();
}

class _DriverHomeState extends State<_DriverHome> {
  DriverSession get session => widget.session;

  /// The load check opens once per route, whether the route was there from the start or arrived
  /// later from the server.
  bool _loadCheckOpened = false;

  void _openLoadCheckOnce() {
    if (_loadCheckOpened || !session.hasRoute || session.loadResolved) return;
    _loadCheckOpened = true;
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
        onStoppedSafely: () async {
          final navigator = Navigator.of(context);
          await session.markArrived(stop);
          if (!mounted) return;
          navigator.pushReplacement(MaterialPageRoute<void>(builder: (context) => _stopDetails(context, stop)));
        },
      ),
    ));
  }

  void _proofUnavailable(ProofKind kind) {
    _notify('Photo and signature capture is not available in this build yet, so no proof is stored.');
  }

  Widget _stopDetails(BuildContext context, StopInfo stop) {
    return StopDetailsScreen(
      stop: stop,
      onTabSelected: _selectTab,
      onReportIssue: () => _reportProblem(stop),
      onProofRequested: _proofUnavailable,
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
                onProofRequested: _proofUnavailable,
                onTabSelected: _selectTab,
                onRejected: () => _takeBack(context, stop, const DeliveryDraft(outcome: DeliveryOutcome.refused)),
                onSave: (partial) => _commit(context, stop, DeliveryDraft(
                  outcome: partial.outcome,
                  quantity: partial.quantity,
                  notes: draft.notes,
                  reason: partial.reason,
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
          hasPhoto: false,
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

  void _openUpdate(UpdateItem item) {
    if (item.id != DriverSession.planConflictId) return;
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
    await session.finishTrip();
  }

  @override
  Widget build(BuildContext context) {
    return ListenableBuilder(
      listenable: session,
      builder: (context, _) {
        if (!session.hasRoute && session.tab != DriverTab.updates) {
          _loadCheckOpened = false;
          return _NoRoute(session: session);
        }
        _openLoadCheckOnce();
        switch (session.tab) {
          case DriverTab.route:
            return RouteHomeScreen(
              trip: session.trip,
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
          AppButton(label: 'Sign out', outline: true, onPressed: () => session.finishTrip()),
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
