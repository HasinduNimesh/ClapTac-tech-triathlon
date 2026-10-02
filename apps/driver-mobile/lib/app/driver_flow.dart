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
        if (!session.signedIn) {
          // Placeholder: staff credentials are not verified yet (ThunderID sign-in is not connected).
          return SignInScreen(onSignIn: (staffId, password) => session.signIn());
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

  @override
  void initState() {
    super.initState();
    if (!session.loadConfirmed) {
      WidgetsBinding.instance.addPostFrameCallback((_) {
        if (!mounted || session.loadConfirmed) return;
        showTruckCheckoutSheet(
          context,
          trip: session.trip,
          onConfirm: session.confirmLoad,
          onMissingItem: () {
            session.confirmLoad();
            _notify('Dispatch and the loader will be told something is missing.');
          },
        );
      });
    }
  }

  void _notify(String message) {
    ScaffoldMessenger.of(context)
      ..hideCurrentSnackBar()
      ..showSnackBar(SnackBar(content: Text(message)));
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
      onSend: (report) {
        session.reportProblem(report, stop: stop);
        _notify('Sent to dispatcher. It is saved on this phone until you reconnect.');
      },
    );
  }

  void _startStop() {
    final stop = session.trip.nextStop;
    if (stop == null) return;
    Navigator.of(context).push(MaterialPageRoute<void>(
      builder: (context) => SafeStopScreen(
        trip: session.trip,
        stop: stop,
        earlyMinutes: 0,
        onTabSelected: _selectTab,
        onStoppedSafely: () => Navigator.of(context).pushReplacement(MaterialPageRoute<void>(
          builder: (context) => _stopDetails(context, stop),
        )),
      ),
    ));
  }

  Widget _stopDetails(BuildContext context, StopInfo stop) {
    return StopDetailsScreen(
      stop: stop,
      onTabSelected: _selectTab,
      onReportIssue: () => _reportProblem(stop),
      onSave: (draft) {
        switch (draft.outcome) {
          case DeliveryOutcome.delivered:
            _commit(context, stop, draft);
          case DeliveryOutcome.partial:
            Navigator.of(context).push(MaterialPageRoute<void>(
              builder: (context) => RecordDeliveryScreen(
                stop: stop,
                orderRef: stop.orderRef,
                expectedQuantity: stop.cartons,
                unit: 'cartons',
                initialOutcome: DeliveryOutcome.partial,
                onTabSelected: _selectTab,
                onRejected: () => _takeBack(context, stop, DeliveryDraft(outcome: DeliveryOutcome.refused, hasPhoto: draft.hasPhoto, hasSignature: draft.hasSignature)),
                onSave: (partial) => _commit(context, stop, DeliveryDraft(
                  outcome: partial.outcome,
                  quantity: partial.quantity,
                  notes: draft.notes,
                  hasPhoto: partial.hasPhoto || draft.hasPhoto,
                  hasSignature: partial.hasSignature || draft.hasSignature,
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
          hasPhoto: draft.hasPhoto,
          hasSignature: draft.hasSignature,
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

  void _openUpdate(UpdateItem item) {
    if (item.id != DriverSession.planConflictId) return;
    Navigator.of(context).push(MaterialPageRoute<void>(
      builder: (context) => PlanConflictScreen(
        info: samplePlanConflict,
        onTabSelected: _selectTab,
        onReturnToRoute: () => _selectTab(DriverTab.route),
        onSendForReview: () {
          _selectTab(DriverTab.route);
          _notify('Both versions were saved to send to dispatch.');
        },
      ),
    ));
  }

  void _finishTrip() {
    Navigator.of(context).popUntil((route) => route.isFirst);
    session.finishTrip();
  }

  @override
  Widget build(BuildContext context) {
    return ListenableBuilder(
      listenable: session,
      builder: (context, _) {
        switch (session.tab) {
          case DriverTab.route:
            return RouteHomeScreen(
              trip: session.trip,
              onViewStop: _startStop,
              onReportProblem: () => _reportProblem(session.trip.nextStop),
              onTabSelected: session.selectTab,
            );
          case DriverTab.updates:
            return UpdatesScreen(items: session.updates, onOpen: _openUpdate, onTabSelected: session.selectTab);
          case DriverTab.summary:
            if (!session.routeComplete) return _InProgressSummary(trip: session.trip, onTabSelected: session.selectTab);
            return EndOfDayScreen(
              summary: session.summary,
              onFinishTrip: _finishTrip,
              onRetryUpload: () => _notify('Uploads are queued on this phone and retry when you are online.'),
              onTabSelected: session.selectTab,
            );
        }
      },
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
