import 'dart:async';

import 'package:flutter/foundation.dart';

import '../auth/auth_gateway.dart';
import '../auth/profile_api.dart';
import '../data/driver_models.dart';
import '../data/sample_data.dart';
import '../messages/messages.dart';
import '../offline/local_database.dart';
import '../screens/states/end_of_day_screen.dart';
import '../screens/updates/updates_screen.dart';
import '../sync/operations.dart';
import '../sync/sync.dart';
import '../sync/sqlite_sync_queue.dart';
import '../sync/sync_worker.dart';
import '../trips/trip_source.dart';
import '../trips/trip_start.dart';
import '../widgets/driver_shell.dart';
import '../widgets/note_banner.dart';

String _two(int value) => value.toString().padLeft(2, '0');

String clockLabel(DateTime time) => '${_two(time.hour)}:${_two(time.minute)}';

/// Waypoint's business clock (Asia/Colombo, fixed UTC+05:30), for times the server decided, so they
/// read the same whatever timezone the phone is set to.
String businessClockLabel(DateTime time) => clockLabel(time.toUtc().add(const Duration(hours: 5, minutes: 30)));

/// Updates entries that stand for a dispatcher message have an id starting with this.
const messageUpdatePrefix = 'msg:';

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
    this.starter,
    this.messageSource,
    this.messagePollInterval = const Duration(seconds: 30),
  })  : _initialTrip = trip ?? ((demoRoute || (auth == null && demoAuth)) ? sampleTrip : null),
        _clock = clock ?? DateTime.now,
        _newId = newId ?? newOperationId {
    worker?.onProgress = (progress, detail) {
      syncProgress = progress;
      syncDetail = detail;
      // Each tick is a chance to start a trip that could not be started without a connection.
      if (tripStartState == TripStartState.waiting && !tripStarting && loadResolved) unawaited(startTrip());
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

  /// Reads and acknowledges dispatch's messages about the trip. Null in demo and test sessions.
  final MessageSource? messageSource;
  final Duration messagePollInterval;
  Timer? _messageTimer;
  bool _fetchingMessages = false;

  /// What dispatch has sent about this trip, oldest first.
  List<DispatcherMessage> messages = const [];

  /// Counts up whenever a message the driver has not seen before arrives, so the screen can tell them.
  int newMessageSerial = 0;
  final Set<String> _seenMessageIds = {};

  int get unreadMessages => messages.where((message) => !message.acknowledged).length;

  /// Reads the trip's messages. Called when the route loads, on a timer, and when the connection
  /// returns. A failure keeps what was already shown: messages are never lost by a missed refresh.
  Future<void> refreshMessages() async {
    final source = messageSource;
    if (source == null || !signedIn || !hasRoute || _fetchingMessages) return;
    final tripId = _baseTrip!.tripId;
    _fetchingMessages = true;
    final result = await source.load(tripId);
    _fetchingMessages = false;
    if (!signedIn || _baseTrip?.tripId != tripId) return;
    final loaded = result.messages;
    if (loaded == null) {
      if (result.signInExpired) await _signInExpired(result.failure);
      notifyListeners();
      return;
    }
    // The first read after signing in is not news: only messages that arrive afterwards are.
    final firstRead = _seenMessageIds.isEmpty && messages.isEmpty;
    var arrived = false;
    for (final message in loaded) {
      if (_seenMessageIds.add(message.id) && !message.acknowledged && !firstRead) arrived = true;
    }
    messages = loaded;
    if (arrived) newMessageSerial++;
    notifyListeners();
  }

  /// Marks a message as read on the server. Needs a connection: when there is none the message stays
  /// unread and the reason is returned for the driver. Returns null when it worked.
  Future<String?> acknowledgeMessage(String messageId) async {
    final source = messageSource;
    final index = messages.indexWhere((message) => message.id == messageId);
    if (source == null || index < 0) return 'That message is no longer available.';
    if (messages[index].acknowledged) return null;
    final result = await source.acknowledge(messages[index].tripId.isEmpty ? _baseTrip!.tripId : messages[index].tripId, messageId);
    if (result.signInExpired) {
      await _signInExpired(result.failure);
      notifyListeners();
      return result.failure;
    }
    if (!result.isDone) return result.failure;
    messages = [for (final message in messages) message.id == messageId ? message.asAcknowledged() : message];
    notifyListeners();
    return null;
  }

  void _startMessagePolling() {
    _messageTimer?.cancel();
    if (messageSource == null) return;
    _messageTimer = Timer.periodic(messagePollInterval, (_) => unawaited(refreshMessages()));
    unawaited(refreshMessages());
  }

  void _stopMessagePolling() {
    _messageTimer?.cancel();
    _messageTimer = null;
    messages = const [];
    _seenMessageIds.clear();
  }

  final LocalDatabase database;
  final SyncQueue queue;
  final DeliverySyncWorker? worker;

  /// Acknowledges the plan and starts the run on the server. Null in demo and test sessions.
  final TripStarter? starter;

  @override
  void dispose() {
    _messageTimer?.cancel();
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

  /// Whether the server run has been started, and why not when it was refused.
  TripStartState tripStartState = TripStartState.notStarted;
  String? tripStartMessage;
  bool get tripStarting => tripStartState == TripStartState.starting;
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
      planId: base.planId,
      planVersion: base.planVersion,
      runStatus: base.runStatus,
    );
  }

  /// The route can only be started once the load is confirmed, or the driver has
  /// explicitly chosen to depart after reporting a discrepancy.
  bool get loadResolved => loadCheck == LoadCheck.confirmed || loadCheck == LoadCheck.overridden;

  bool get routeComplete => hasRoute && trip.completedStops >= _baseTrip!.stops.length;

  int get pendingUploads => queue is SqliteSyncQueue ? _pendingCount : _results.length + _incidents;

  List<UpdateItem> get visibleUpdates {
    if (worker == null) return [..._messageItems, ...updates];
    final (title, detail, tone) = switch (syncProgress) {
      SyncProgress.idle => ('Sync up to date', 'No unsent updates are waiting.', NoteTone.success),
      SyncProgress.syncing => ('Syncing', 'Sending saved updates to Waypoint in order.', NoteTone.info),
      SyncProgress.waitingForSignIn => ('Sign-in needed', 'Saved updates remain on this phone until you sign in.', NoteTone.amber),
      SyncProgress.waitingForTripStart => ('Trip not started', tripStartMessage ?? 'Saved stop updates will sync after the plan is acknowledged and the trip is started.', NoteTone.amber),
      SyncProgress.waitingForProof => ('Proof needed', 'The outcome stays on this phone until its photo or signature is uploaded.', NoteTone.amber),
      SyncProgress.offline => ('Waiting for connection', syncDetail ?? 'Saved updates will retry when Waypoint can be reached.', NoteTone.offline),
      SyncProgress.needsAttention => ('Sync needs attention', syncDetail ?? 'A saved update needs review before later updates can sync.', NoteTone.danger),
    };
    return [UpdateItem(title: title, detail: detail, time: clockLabel(_clock()), tone: tone), ..._messageItems, ...updates];
  }

  /// Dispatch's messages as Updates entries, unread ones first. Opening one shows it in full and, when
  /// it is unread, offers to acknowledge it.
  List<UpdateItem> get _messageItems {
    final unread = [for (final message in messages.reversed) if (!message.acknowledged) message];
    final read = [for (final message in messages.reversed) if (message.acknowledged) message];
    UpdateItem item(DispatcherMessage message) => UpdateItem(
          id: '$messageUpdatePrefix${message.id}',
          title: message.acknowledged ? 'Message from dispatch' : 'Message from dispatch - please acknowledge',
          detail: message.body,
          time: businessClockLabel(message.createdAt),
          tone: message.acknowledged ? NoteTone.success : NoteTone.amber,
        );
    return [for (final message in unread) item(message), for (final message in read) item(message)];
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
      final sameTrip = _baseTrip?.tripId == loaded.tripId;
      _baseTrip = loaded;
      if (loaded.started) {
        // Already started (the app was restarted mid-run): do not ask for the load check again.
        loadCheck = LoadCheck.confirmed;
        tripStartState = TripStartState.started;
        tripStartMessage = null;
      } else if (!sameTrip) {
        loadCheck = LoadCheck.unchecked;
        tripStartState = TripStartState.notStarted;
        tripStartMessage = null;
      }
      await _restoreQueuedActions(loaded);
      _startMessagePolling();
      // A reload after the server refused the start (the plan changed) tries again with the new version.
      if (sameTrip && loadResolved && !loaded.started) {
        notifyListeners();
        await startTrip();
        return;
      }
    } else if (result.signInExpired) {
      await _signInExpired(result.failure);
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
      } else if (type == OperationType.proofUpload && stopId != null) {
        final kind = (operation['payload'] as Map?)?['proofType'] == 'SIGNATURE' ? ProofKind.signature : ProofKind.photo;
        _operationIds['proof:$stopId:${kind.name}'] = event.idempotencyKey;
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
  ///
  /// With a real sync queue it first tries to send everything that is waiting, including the route
  /// completion, because sign-out stops the sync worker. If something could not be sent (no signal,
  /// or an update the server refused) the driver stays signed in and the number of unsent updates is
  /// returned, so the screen can ask. Pass [force] to sign out anyway: nothing is discarded, the
  /// updates stay saved on this phone and are sent the next time this driver signs in.
  ///
  /// Returns 0 when the driver was signed out.
  Future<int> finishTrip({bool force = false}) async {
    if (routeComplete) {
      await queue.enqueue(Operations.routeCompleted(operationId: _operationId('route-completed'), trip: trip, occurredAt: _clock()).toSyncEvent());
    }
    final syncing = worker;
    if (syncing != null && queue is SqliteSyncQueue && signedIn && !force) {
      await syncing.syncNow();
      await _refreshPending();
      if (_pendingCount > 0) {
        notifyListeners();
        return _pendingCount;
      }
    }
    _stopMessagePolling();
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
    return 0;
  }

  /// The stored sign-in is no longer accepted: go back to sign-in instead of showing a dead end.
  Future<void> _signInExpired(String? message) async {
    _stopMessagePolling();
    worker?.stop();
    signedIn = false;
    if (queue is SqliteSyncQueue) await (queue as SqliteSyncQueue).useOwner(null);
    await auth?.signOut();
    identity = null;
    signInError = message;
  }

  /// Confirming the load is also the driver setting off, so it starts the trip on the server.
  Future<void> confirmLoad() async {
    loadCheck = LoadCheck.confirmed;
    notifyListeners();
    await startTrip();
  }

  /// Acknowledges the current plan and starts the server run. Needs a connection: when there is
  /// none it waits and is retried on each sync tick, and nothing the driver recorded is lost.
  Future<void> startTrip() async {
    final source = starter;
    if (source == null || !hasRoute || tripStarting || tripStartState == TripStartState.started) return;
    final base = _baseTrip!;
    tripStartState = TripStartState.starting;
    tripStartMessage = null;
    notifyListeners();
    final result = await source.start(trip, operationId: _operationId('start:${base.tripId}'));
    if (!signedIn || _baseTrip?.tripId != base.tripId) {
      tripStartState = TripStartState.notStarted;
      return;
    }
    switch (result.status) {
      case TripStartStatus.started:
        tripStartState = TripStartState.started;
        _baseTrip = _baseTrip!.withRunStatus('in_progress');
        unawaited(worker?.syncNow());
      case TripStartStatus.offline:
        tripStartState = TripStartState.waiting;
        tripStartMessage = 'The trip starts when Waypoint can be reached. Saved updates stay on this phone until then.';
      case TripStartStatus.signInNeeded:
        tripStartState = TripStartState.notStarted;
        await _signInExpired(result.message);
      case TripStartStatus.refused:
        tripStartState = TripStartState.refused;
        tripStartMessage = result.message;
    }
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
    // Captured proof is queued first, in order, because the server wants it before it accepts a
    // delivered or partial outcome. The outcome depends on the last proof.
    String? lastProofId;
    for (final proof in draft.proofs) {
      lastProofId = _operationId('proof:${stop.stopId}:${proof.kind.name}');
      await queue.enqueue(Operations.proofUpload(operationId: lastProofId, trip: trip, stop: stop, proof: proof, occurredAt: _clock()).toSyncEvent());
    }
    await queue.enqueue(Operations.stopOutcome(
      operationId: _operationId('outcome:${stop.stopId}'),
      trip: trip,
      stop: stop,
      draft: draft,
      occurredAt: _clock(),
      proofOperationId: lastProofId,
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
    notifyListeners();
    await startTrip();
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

/// `waiting`: no connection yet, will retry. `refused`: the server said no (see the message).
enum TripStartState { notStarted, starting, waiting, started, refused }

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
