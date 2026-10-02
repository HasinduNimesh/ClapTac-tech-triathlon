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
  final TripInfo _baseTrip;
  final DateTime Function() _clock;

  late final DeliveryRepository _deliveries = DeliveryRepository(database, queue);
  late final ProofRepository _proofs = ProofRepository(queue);

  final Map<int, DeliveryDraft> _results = {};
  final List<UpdateItem> updates = [];

  bool signedIn = false;
  bool loadConfirmed = false;
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

  bool get routeComplete => _results.length >= _baseTrip.stops.length;

  int get pendingPhotoUploads => _results.values.where((draft) => draft.hasPhoto).length;

  void signIn() {
    signedIn = true;
    tab = DriverTab.route;
    notifyListeners();
  }

  /// Ends the trip and returns to sign-in, clearing what was recorded today.
  void finishTrip() {
    signedIn = false;
    loadConfirmed = false;
    tab = DriverTab.route;
    _results.clear();
    updates.removeWhere((item) => item.id != planConflictId);
    notifyListeners();
  }

  void confirmLoad() {
    loadConfirmed = true;
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

  void reportProblem(ProblemReport report, {StopInfo? stop}) {
    updates.insert(
      0,
      UpdateItem(
        title: 'Problem reported',
        detail: '${problemLabel(report.kind)}${stop == null ? '' : ' · ${stop.outletCode} ${stop.name}'} · saved on this phone',
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
