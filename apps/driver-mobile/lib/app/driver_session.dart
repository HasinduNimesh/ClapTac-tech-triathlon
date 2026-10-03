import 'package:flutter/foundation.dart';

import '../auth/auth_gateway.dart';
import '../auth/profile_api.dart';
import '../data/driver_models.dart';
import '../data/sample_data.dart';
import '../offline/local_database.dart';
import '../screens/states/end_of_day_screen.dart';
import '../screens/updates/updates_screen.dart';
import '../sync/operations.dart';
import '../sync/sync.dart';
import '../sync/sqlite_sync_queue.dart';
import '../sync/sync_worker.dart';
import '../trips/trip_source.dart';
import '../widgets/driver_shell.dart';
import '../widgets/note_banner.dart';

String _two(int value) => value.toString().padLeft(2, '0');

String clockLabel(DateTime time) => '${_two(time.hour)}:${_two(time.minute)}';

/// A local action remains queued until Waypoint confirms it was applied.
const savedNotSent = 'saved on this phone, not sent yet';

/// App state for the signed-in driver: today's trip, what has been recorded on this phone, and the
/// updates list.
///
/// Driver operations are queued with their server IDs and stable operation IDs.
class DriverSession extends ChangeNotifier {
  DriverSession({
    required this.database,
    required this.queue,
    TripInfo? trip,
    DateTime Function()? clock,
    String Function()? newId,
    bool demoUpdates = false,
    this.demoAuth = false,
    this.demoRoute = false,
    this.auth,
    this.trips,
    this.worker,
  })  : _initialTrip = trip ?? ((demoRoute || (auth == null && demoAuth)) ? sampleTrip : null),
        _clock = clock ?? DateTime.now,
        _newId = newId ?? newOperationId {
    worker?.onProgress = (progress, detail) {
      syncProgress = progress;
      syncDetail = detail;
      if (signedIn) _refreshPending();
      notifyListeners();
    };
    if (demoUpdates) {
      updates.add(const UpdateItem(
        id: planConflictId,
        title: 'Plan update needs review',
        detail: 'Demo only: sample plan change, nothing is sent to dispatch.',
        time: '06:32',
        tone: NoteTone.amber,
      ));
    }
  }

  static const planConflictId = 'plan-conflict';

  final LocalDatabase database;
  final SyncQueue queue;
  final DeliverySyncWorker? worker;

  @override
  void dispose() {
    worker?.onProgress = null;
    super.dispose();
  }

  /// Accept any credentials. Only for demos and tests: staff accounts are not connected yet.
  final bool demoAuth;

  /// Show the sample route to a signed-in driver. Only for demos: assigned trips are not loaded yet.
  final bool demoRoute;

  /// Real sign-in through the identity provider. When set it replaces [demoAuth].
  final AuthGateway? auth;

  /// Where the driver's route comes from after a real sign-in. Null in demo and test sessions.
  final TripSource? trips;

  final TripInfo? _initialTrip;
  late TripInfo? _baseTrip = _initialTrip;
  final DateTime Function() _clock;
  final String Function() _newId;

  /// One id per kind of thing the driver did (per stop where it applies), created once and kept so a
  /// repeat or a retry is recognised as the same operation.
  final Map<String, String> _operationIds = {};

  final Map<String, DeliveryDraft> _results = {};
  final List<UpdateItem> updates = [];
  int _incidents = 0;
  int _pendingCount = 0;
  SyncProgress syncProgress = SyncProgress.idle;
  String? syncDetail;

  bool signedIn = false;
  bool signingIn = false;
  bool restoring = false;

  /// Loading today's route from the server, and why it failed (null when it did not).
  bool tripsLoading = false;
  String? tripsError;
  DriverProfile? identity;
  LoadCheck loadCheck = LoadCheck.unchecked;
  String? signInError;
  DriverTab tab = DriverTab.route;

  /// Whether there is a route to show. Without one (a real sign-in, no trips loaded yet) the app
  /// says so instead of showing sample data.
  bool get hasRoute => _baseTrip != null;

  TripInfo get trip {
    final base = _baseTrip!;
    return TripInfo(
      tripId: base.tripId,
      runId: base.runId,
      vehicleCode: base.vehicleCode,
      plate: base.plate,
      tripRef: base.tripRef,
      depot: base.depot,
      window: base.window,
      stops: base.stops,
      completedStops: base.completedStops + _results.length,
      completedStopIds: base.completedStopIds,
    );
  }

  /// The route can only be started once the load is confirmed, or the driver has
  /// explicitly chosen to depart after reporting a discrepancy.
  bool get loadResolved => loadCheck == LoadCheck.confirmed || loadCheck == LoadCheck.overridden;

  bool get routeComplete => hasRoute && trip.completedStops >= _baseTrip!.stops.length;

  int get pendingUploads => queue is SqliteSyncQueue ? _pendingCount : _results.length + _incidents;

  List<UpdateItem> get visibleUpdates {
    if (worker == null) return updates;
    final (title, detail, tone) = switch (syncProgress) {
      SyncProgress.idle => ('Sync up to date', 'No unsent updates are waiting.', NoteTone.success),
      SyncProgress.syncing => ('Syncing', 'Sending saved updates to Waypoint in order.', NoteTone.info),
      SyncProgress.waitingForSignIn => ('Sign-in needed', 'Saved updates remain on this phone until you sign in.', NoteTone.amber),
      SyncProgress.waitingForTripStart => ('Trip not started', 'Saved stop updates will sync after the plan is acknowledged and the trip is started.', NoteTone.amber),
      SyncProgress.waitingForProof => ('Proof needed', 'The outcome stays on this phone until its photo or signature is uploaded.', NoteTone.amber),
      SyncProgress.offline => ('Waiting for connection', syncDetail ?? 'Saved updates will retry when Waypoint can be reached.', NoteTone.offline),
      SyncProgress.needsAttention => ('Sync needs attention', syncDetail ?? 'A saved update needs review before later updates can sync.', NoteTone.danger),
    };
    return [UpdateItem(title: title, detail: detail, time: clockLabel(_clock()), tone: tone), ...updates];
  }

  Future<void> _refreshPending() async {
    if (queue is! SqliteSyncQueue || identity == null || !signedIn) return;
    try {
      _pendingCount = (await queue.pending()).length;
      if (signedIn) notifyListeners();
    } on StateError {
      // Sign-out can clear the queue owner while a refresh is in flight.
    }
  }

  Future<void> _useIdentity(DriverProfile profile) async {
    if (queue is SqliteSyncQueue) {
      await (queue as SqliteSyncQueue).useOwner(profile.userId);
      await _refreshPending();
    }
    worker?.start();
  }

  void _kickSync() {
    _refreshPending();
    worker?.syncNow();
  }

  Future<void> retrySync() async {
    await worker?.syncNow();
    await _refreshPending();
  }

  bool get usesIdentityProvider => auth != null;

  String _operationId(String key) => _operationIds.putIfAbsent(key, _newId);

  /// Opens a previous sign-in that is still valid, so a driver who is already signed in
  /// (possibly without signal) does not have to sign in again.
  Future<void> restore() async {
    final gateway = auth;
    if (gateway == null) return;
    restoring = true;
    notifyListeners();
    final profile = await gateway.restore();
    restoring = false;
    if (profile != null) {
      identity = profile;
      signedIn = true;
      await _useIdentity(profile);
    }
    notifyListeners();
    if (profile != null) await loadTrips();
  }

  /// Loads today's route for the signed-in driver. Safe to call again to retry.
  Future<void> loadTrips() async {
    final source = trips;
    if (source == null || tripsLoading || !signedIn) return;
    tripsLoading = true;
    tripsError = null;
    notifyListeners();
    final result = await source.loadToday();
    tripsLoading = false;
    if (!signedIn) {
      notifyListeners();
      return;
    }
    final loaded = result.trip;
    if (loaded != null) {
      _baseTrip = loaded;
      loadCheck = LoadCheck.unchecked;
      await _restoreQueuedActions(loaded);
    } else if (result.signInExpired) {
      // The stored sign-in is no longer accepted: go back to sign-in instead of showing a dead end.
      worker?.stop();
      signedIn = false;
      if (queue is SqliteSyncQueue) await (queue as SqliteSyncQueue).useOwner(null);
      await auth?.signOut();
      identity = null;
      signInError = result.failure;
    } else {
      tripsError = result.failure;
    }
    notifyListeners();
  }

  /// Rebuild the local route overlay and stable operation IDs from SQLite after
  /// a process restart. Server-completed stops are not counted a second time.
  Future<void> _restoreQueuedActions(TripInfo loaded) async {
    if (queue is! SqliteSyncQueue) return;
    _results.clear();
    _operationIds.clear();
    final pending = await queue.pending();
    final completedIds = loaded.completedStopIds;
    for (final event in pending) {
      final operation = event.payload;
      if (operation['tripId'] != loaded.tripId) continue;
      final type = operation['type'];
      final stopId = operation['stopId'] as String?;
      if (type == OperationType.arrived && stopId != null) {
        _operationIds['arrived:$stopId'] = event.idempotencyKey;
      } else if (type == OperationType.routeCompleted) {
        _operationIds['route-completed'] = event.idempotencyKey;
      } else if (type == OperationType.stopOutcome && stopId != null) {
        _operationIds['outcome:$stopId'] = event.idempotencyKey;
        if (completedIds.contains(stopId)) continue;
        final payload = operation['payload'];
        if (payload is! Map) continue;
        final fields = payload.cast<String, Object?>();
        final outcome = switch (fields['code']) {
          'DELIVERED' => DeliveryOutcome.delivered,
          'PARTIAL' => DeliveryOutcome.partial,
          'FAILED' => DeliveryOutcome.failed,
          'REFUSED' => DeliveryOutcome.refused,
          _ => null,
        };
        if (outcome == null) continue;
        final note = fields['note'] as String? ?? '';
        final quantityMatch = RegExp(r'^Received (\d+) of [^.]+\.\s*').firstMatch(note);
        _results[stopId] = DeliveryDraft(
          outcome: outcome,
          quantity: (fields['deliveredUnits'] as num?)?.toInt() ??
              (quantityMatch == null ? null : int.tryParse(quantityMatch.group(1)!)),
          notes: quantityMatch == null ? note : note.substring(quantityMatch.end),
          reason: fields['reason'] as String?,
        );
      }
    }
  }

  /// Returns whether the driver was let in. A real sign-in goes through the identity provider
  /// and the Waypoint profile; without one this only succeeds in a demo build, so a normal
  /// build cannot open the workspace.
  Future<bool> signIn() async {
    final gateway = auth;
    if (gateway != null) {
      if (signingIn) return false;
      signingIn = true;
      signInError = null;
      notifyListeners();
      final outcome = await gateway.signIn();
      signingIn = false;
      final profile = outcome.profile;
      if (profile == null) {
        signInError = outcome.failure?.message;
        notifyListeners();
        return false;
      }
      identity = profile;
      signedIn = true;
      await _useIdentity(profile);
      tab = DriverTab.route;
      notifyListeners();
      await loadTrips();
      return true;
    }
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

  /// Ends the trip and signs out. If every stop was recorded, route completion is queued first.
  /// What is already queued stays queued: finishing the trip does not discard anything unsent.
  Future<void> finishTrip() async {
    if (routeComplete) {
      await queue.enqueue(Operations.routeCompleted(operationId: _operationId('route-completed'), trip: trip, occurredAt: _clock()).toSyncEvent());
      _kickSync();
    }
    worker?.stop();
    signedIn = false;
    if (queue is SqliteSyncQueue) await (queue as SqliteSyncQueue).useOwner(null);
    final gateway = auth;
    if (gateway != null) await gateway.signOut();
    identity = null;
    loadCheck = LoadCheck.unchecked;
    tab = DriverTab.route;
    _results.clear();
    _baseTrip = _initialTrip;
    tripsError = null;
    _operationIds.clear();
    _incidents = 0;
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

  /// The driver parked at the stop. The server needs an arrival before it accepts an outcome.
  Future<void> markArrived(StopInfo stop) async {
    await queue.enqueue(Operations.arrived(operationId: _operationId('arrived:${stop.stopId}'), trip: trip, stop: stop, occurredAt: _clock()).toSyncEvent());
    _kickSync();
  }

  /// Queues the outcome as a `STOP_OUTCOME`. No proof is attached: photo and signature capture is
  /// not built, so there is nothing to upload and no `dependsOnOperationId` to claim. The server
  /// requires proof for delivered and partial outcomes, so those will be rejected when sent until
  /// capture exists.
  Future<void> recordDelivery(StopInfo stop, DeliveryDraft draft) async {
    await queue.enqueue(Operations.stopOutcome(
      operationId: _operationId('outcome:${stop.stopId}'),
      trip: trip,
      stop: stop,
      draft: draft,
      occurredAt: _clock(),
    ).toSyncEvent());
    _kickSync();
    _results[stop.stopId] = draft;
    updates.insert(
      0,
      UpdateItem(
        title: 'Delivery $savedNotSent',
        detail: '${stop.labelWith(' ')} · ${outcomeLabel(draft.outcome)}',
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
      id: _operationId('load-discrepancy'),
      category: 'GOODS',
      description: 'Load check: an item on the driver\'s load list is missing.',
    );
    loadCheck = LoadCheck.discrepancy;
    updates.insert(
      0,
      UpdateItem(
        title: 'Missing load item reported',
        detail: '${trip.vehicleCode} · ${trip.tripRef} · $savedNotSent',
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
      id: _operationId('load-override'),
      category: 'GOODS',
      description: 'Driver departed without resolving a reported missing load item.',
    );
    loadCheck = LoadCheck.overridden;
    updates.insert(
      0,
      UpdateItem(
        title: 'Departed with a load discrepancy',
        detail: '${trip.vehicleCode} · ${trip.tripRef} · decision $savedNotSent',
        time: clockLabel(_clock()),
        tone: NoteTone.danger,
      ),
    );
    notifyListeners();
  }

  Future<void> _queueIncident({required String id, required String category, required String description, StopInfo? stop}) async {
    await queue.enqueue(Operations.incident(
      operationId: id,
      trip: trip,
      category: category,
      description: description,
      occurredAt: _clock(),
      stop: stop,
    ).toSyncEvent());
    _kickSync();
    _incidents++;
  }

  Future<void> reportProblem(ProblemReport report, {StopInfo? stop}) async {
    await _queueIncident(
      id: _newId(),
      category: incidentCategory(report.kind),
      description: report.note.isEmpty ? problemLabel(report.kind) : '${problemLabel(report.kind)}: ${report.note}',
      stop: stop,
    );
    updates.insert(
      0,
      UpdateItem(
        title: 'Problem reported',
        detail: '${problemLabel(report.kind)}${stop == null ? '' : ' · ${stop.labelWith(' ')}'} · $savedNotSent',
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
    final base = _baseTrip!;
    for (final stop in base.stops) {
      final draft = _results[stop.stopId];
      if (draft == null) continue;
      switch (draft.outcome) {
        case DeliveryOutcome.delivered:
          delivered++;
          completed.add(StopResult(name: stop.name, window: stop.window, status: 'Delivered'));
        case DeliveryOutcome.partial:
          partial++;
          final expected = stop.units;
          final short = draft.quantity == null || expected == null ? null : expected - draft.quantity!;
          unresolved.add(StopResult(
            name: stop.name,
            window: stop.window,
            status: 'Partial delivery',
            detail: short == null || short <= 0 ? 'Saved on this phone, not sent yet' : '$short units short - saved on this phone, not sent yet',
          ));
        case DeliveryOutcome.failed:
        case DeliveryOutcome.refused:
          failed++;
          unresolved.add(StopResult(
            name: stop.name,
            window: stop.window,
            status: draft.outcome == DeliveryOutcome.failed ? 'Failed delivery' : 'Refused',
            detail: draft.notes.isEmpty ? 'Saved on this phone, not sent yet' : '${draft.notes} - saved on this phone, not sent yet',
          ));
      }
    }
    return EndOfDaySummary(
      totalStops: base.stops.length,
      delivered: delivered,
      partial: partial,
      failedOrRefused: failed,
      completed: completed,
      unresolved: unresolved,
      pendingUploads: pendingUploads,
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
