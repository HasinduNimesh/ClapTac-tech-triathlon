import 'package:flutter/foundation.dart';

import '../data/driver_models.dart';
import '../deliveries/deliveries.dart';
import '../offline/local_database.dart';
import '../proof/proof.dart';
import '../screens/states/end_of_day_screen.dart';
import '../screens/updates/updates_screen.dart';
import '../shared/models.dart';
import '../sync/sync.dart';
import '../data/sample_data.dart';
import '../widgets/driver_shell.dart';
import '../widgets/note_banner.dart';

String _two(int value) => value.toString().padLeft(2, '0');

String clockLabel(DateTime time) => '${_two(time.hour)}:${_two(time.minute)}';

/// App state for the signed-in driver: today's trip, what has been recorded on
/// this phone, and the updates list. Deliveries are written to the local
/// database and queued for sync; nothing is uploaded yet.
class DriverSession extends ChangeNotifier {
  DriverSession({
    required this.database,
    required this.queue,
    TripInfo? trip,
    DateTime Function()? clock,
    bool demoUpdates = false,
    this.demoAuth = false,
  })  : _baseTrip = trip ?? sampleTrip,
        _clock = clock ?? DateTime.now {
    if (demoUpdates) {
      updates.add(const UpdateItem(
        id: planConflictId,
        title: 'Plan update needs review',
        detail: 'Dispatch published Plan v4. Review how it affects your saved stop.',
        time: '06:32',
        tone: NoteTone.amber,
      ));
    }
  }

  static const planConflictId = 'plan-conflict';

  final LocalDatabase database;
  final SyncQueue queue;

  /// Accept any credentials. Only for demos and tests: staff accounts are not connected yet.
  final bool demoAuth;
  final TripInfo _baseTrip;
  final DateTime Function() _clock;

  late final DeliveryRepository _deliveries = DeliveryRepository(database, queue);
  late final ProofRepository _proofs = ProofRepository(queue);

  final Map<int, DeliveryDraft> _results = {};
  final List<UpdateItem> updates = [];

  bool signedIn = false;
  LoadCheck loadCheck = LoadCheck.unchecked;
  String? signInError;
  DriverTab tab = DriverTab.route;

  TripInfo get trip => TripInfo(
        vehicleCode: _baseTrip.vehicleCode,
        plate: _baseTrip.plate,
        tripRef: _baseTrip.tripRef,
        depot: _baseTrip.depot,
        window: _baseTrip.window,
        stops: _baseTrip.stops,
        completedStops: _results.length,
      );

  /// The route can only be started once the load is confirmed, or the driver has
  /// explicitly chosen to depart after reporting a discrepancy.
  bool get loadResolved => loadCheck == LoadCheck.confirmed || loadCheck == LoadCheck.overridden;

  bool get routeComplete => _results.length >= _baseTrip.stops.length;

  int get pendingPhotoUploads => _results.values.where((draft) => draft.hasPhoto).length;

  /// Returns whether the driver was let in. Without a verified identity provider
  /// this only succeeds in a demo build, so a normal build cannot open the workspace.
  bool signIn() {
    if (!demoAuth) {
      signInError = 'Sign-in is not available yet. Staff accounts are not connected to this build.';
      notifyListeners();
      return false;
    }
    signInError = null;
    signedIn = true;
    tab = DriverTab.route;
    notifyListeners();
    return true;
  }

  /// Ends the trip and returns to sign-in, clearing what was recorded today.
  void finishTrip() {
    signedIn = false;
    loadCheck = LoadCheck.unchecked;
    tab = DriverTab.route;
    _results.clear();
    updates.removeWhere((item) => item.id != planConflictId);
    notifyListeners();
  }

  void confirmLoad() {
    loadCheck = LoadCheck.confirmed;
    notifyListeners();
  }

  void selectTab(DriverTab next) {
    if (tab == next) return;
    tab = next;
    notifyListeners();
  }

  Future<void> recordDelivery(StopInfo stop, DeliveryDraft draft) async {
    await _deliveries.recordOutcome(Delivery(id: stop.outletCode, tripId: trip.tripRef, status: draft.outcome.name));
    if (draft.hasPhoto) {
      await _proofs.queueProof(ProofRecord(deliveryId: stop.outletCode, objectKey: 'local/${stop.outletCode}.jpg'));
    }
    _results[stop.sequence] = draft;
    updates.insert(
      0,
      UpdateItem(
        title: 'Delivery saved on this phone',
        detail: '${stop.outletCode} ${stop.name} · ${outcomeLabel(draft.outcome)}${draft.hasPhoto ? ' + 1 photo' : ''} · waiting to upload',
        time: clockLabel(_clock()),
        tone: NoteTone.offline,
      ),
    );
    notifyListeners();
  }

  /// The driver says something on the load list is missing. There is no driver-facing
  /// load-shortfall operation yet, so it is queued as a GOODS incident. The load stays
  /// unconfirmed until the driver confirms it again or explicitly departs anyway.
  Future<void> reportLoadDiscrepancy() async {
    await _queueIncident(
      key: 'load-discrepancy-${trip.tripRef}',
      category: 'GOODS',
      description: 'Load check: an item on the driver\'s load list is missing.',
    );
    loadCheck = LoadCheck.discrepancy;
    updates.insert(
      0,
      UpdateItem(
        title: 'Missing load item reported',
        detail: '${trip.vehicleCode} · ${trip.tripRef} · queued on this phone, not sent yet',
        time: clockLabel(_clock()),
        tone: NoteTone.danger,
      ),
    );
    notifyListeners();
  }

  /// The driver chose to depart even though a missing item is unresolved. The decision is
  /// recorded as its own incident so it is not lost.
  Future<void> overrideLoadCheck() async {
    await _queueIncident(
      key: 'load-override-${trip.tripRef}',
      category: 'GOODS',
      description: 'Driver departed without resolving a reported missing load item.',
    );
    loadCheck = LoadCheck.overridden;
    updates.insert(
      0,
      UpdateItem(
        title: 'Departed with a load discrepancy',
        detail: '${trip.vehicleCode} · ${trip.tripRef} · decision queued on this phone, not sent yet',
        time: clockLabel(_clock()),
        tone: NoteTone.danger,
      ),
    );
    notifyListeners();
  }

  Future<void> _queueIncident({required String key, required String category, required String description, StopInfo? stop}) {
    return queue.enqueue(SyncEvent(
      eventId: key,
      idempotencyKey: key,
      action: 'INCIDENT_REPORT',
      resourceType: 'trip',
      resourceId: trip.tripRef,
      occurredAt: _clock(),
      payload: {'category': category, 'description': description, if (stop != null) 'stopId': stop.outletCode},
    ));
  }

  Future<void> reportProblem(ProblemReport report, {StopInfo? stop}) async {
    final stamp = _clock().millisecondsSinceEpoch;
    await _queueIncident(
      key: 'incident-${trip.tripRef}-${stop?.outletCode ?? 'trip'}-${report.kind.name}-$stamp',
      category: incidentCategory(report.kind),
      description: report.note.isEmpty ? problemLabel(report.kind) : '${problemLabel(report.kind)}: ${report.note}',
      stop: stop,
    );
    updates.insert(
      0,
      UpdateItem(
        title: 'Problem reported',
        detail: '${problemLabel(report.kind)}${stop == null ? '' : ' · ${stop.outletCode} ${stop.name}'} · queued on this phone, not sent yet',
        time: clockLabel(_clock()),
        tone: NoteTone.danger,
      ),
    );
    notifyListeners();
  }

  EndOfDaySummary get summary {
    final completed = <StopResult>[];
    final unresolved = <StopResult>[];
    var delivered = 0;
    var partial = 0;
    var failed = 0;
    for (final stop in _baseTrip.stops) {
      final draft = _results[stop.sequence];
      if (draft == null) continue;
      switch (draft.outcome) {
        case DeliveryOutcome.delivered:
          delivered++;
          completed.add(StopResult(name: stop.name, window: stop.window, status: 'Delivered'));
        case DeliveryOutcome.partial:
          partial++;
          final short = draft.quantity == null ? null : stop.cartons - draft.quantity!;
          unresolved.add(StopResult(
            name: stop.name,
            window: stop.window,
            status: 'Partial delivery',
            detail: short == null || short <= 0 ? 'Dispatcher notified' : '$short cartons short - Dispatcher notified',
          ));
        case DeliveryOutcome.failed:
        case DeliveryOutcome.refused:
          failed++;
          unresolved.add(StopResult(
            name: stop.name,
            window: stop.window,
            status: draft.outcome == DeliveryOutcome.failed ? 'Failed delivery' : 'Refused',
            detail: draft.notes.isEmpty ? 'Dispatcher notified' : '${draft.notes} - Dispatcher notified',
          ));
      }
    }
    return EndOfDaySummary(
      totalStops: _baseTrip.stops.length,
      delivered: delivered,
      partial: partial,
      failedOrRefused: failed,
      completed: completed,
      unresolved: unresolved,
      pendingUploads: pendingPhotoUploads,
    );
  }
}

enum LoadCheck { unchecked, confirmed, discrepancy, overridden }

/// Category names used by the delivery sync contract for INCIDENT_REPORT.
String incidentCategory(ProblemKind kind) {
  switch (kind) {
    case ProblemKind.vehicleBreakdown:
      return 'VEHICLE';
    case ProblemKind.roadBlocked:
      return 'ROAD';
    case ProblemKind.outletClosed:
      return 'OUTLET';
    case ProblemKind.loadIssue:
      return 'GOODS';
    case ProblemKind.safetyConcern:
      return 'SAFETY';
  }
}

String outcomeLabel(DeliveryOutcome outcome) {
  switch (outcome) {
    case DeliveryOutcome.delivered:
      return 'Delivered';
    case DeliveryOutcome.partial:
      return 'Partial delivery';
    case DeliveryOutcome.failed:
      return 'Failed';
    case DeliveryOutcome.refused:
      return 'Refused';
  }
}

String problemLabel(ProblemKind kind) {
  switch (kind) {
    case ProblemKind.vehicleBreakdown:
      return 'Vehicle breakdown';
    case ProblemKind.roadBlocked:
      return 'Road blocked or flooded';
    case ProblemKind.outletClosed:
      return 'Outlet closed or no access';
    case ProblemKind.loadIssue:
      return 'Load wrong, missing or damaged';
    case ProblemKind.safetyConcern:
      return 'Safety concern';
  }
}
