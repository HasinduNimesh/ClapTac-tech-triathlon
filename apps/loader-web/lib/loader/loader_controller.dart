import 'package:flutter/foundation.dart';

import '../api/api_client.dart';
import '../shared/models.dart';
import '../widgets/common.dart';

class StopGroup {
  StopGroup(this.stopSequence, this.orders);
  final int stopSequence;
  final List<LoadingOrder> orders;
  LoadingOrder get head => orders.first;
  String get outletId => head.outletId;
  /// "Waypoint Fresh · Borella" from the outlet name, or brand and district.
  String get outletLabel {
    if (head.outletName.isNotEmpty && !head.outletName.endsWith(head.outletId)) return head.outletName;
    final parts = [if (head.brand.isNotEmpty) 'Waypoint ${head.brand}', if (head.district.isNotEmpty) head.district];
    return parts.isEmpty ? outletId : parts.join(' · ');
  }
  bool get chilled => orders.any((o) => o.chilled);
  /// Loaded, or short with a dispatcher decision that lets the trip leave.
  bool get loaded => orders.every((o) => o.loaded || (o.short && !o.unresolved));
  bool get hasShortfall => orders.any((o) => o.short);
  int get expectedUnits => orders.fold(0, (s, o) => s + o.expectedUnits);
  double get weightKg => orders.fold(0.0, (s, o) => s + o.weightKg);
  double get volumeM3 => orders.fold(0.0, (s, o) => s + o.volumeM3);
  String get changeNote => orders.map((o) => o.changeNote).firstWhere((n) => n.isNotEmpty, orElse: () => '');
  int get changedInVersion => orders.map((o) => o.changedInVersion).fold(0, (a, b) => a > b ? a : b);
}

/// One row on the Notifications tab, built from the trips the loader can see.
class LoaderNotice {
  LoaderNotice(this.at, this.title, this.text, this.trip, {this.tone = 'primary'});
  final DateTime? at;
  final String title;
  final String text;
  final LoadingTrip trip;
  final String tone;
}

/// One open "report an issue" form. The idempotency key is made once; [issueId] is set as soon
/// as the report has been filed, so a retry after a failed photo upload only uploads.
class ReportAttempt {
  final String key = newOperationId();
  String? issueId;
  bool photoSent = false;

  /// The report exists on the server; only the photo is outstanding.
  bool get filed => issueId != null;
}

/// Departure checks entered on the Ready card before confirming.
class ReadyCheck {
  double? temperatureC;
  DateTime? temperatureAt;
  String seal = '';
}

/// Loader dock workflow against the loading, planning and order services.
/// Always online: every action goes straight to the server and the trip is
/// re-read afterwards, so the screen always shows the server's state.
class LoaderController extends ChangeNotifier {
  LoaderController({required this.api});

  final ApiClient api;

  List<LoadingTrip> trips = [];
  final Map<String, LoadingTrip> details = {};
  final Map<String, ReadyCheck> readyChecks = {};
  List<DockAlert> alerts = [];
  String selectedTripId = '';
  String date = todayIso();
  bool loading = true;
  bool busy = false;
  bool connectionLost = false;
  String error = '';
  DateTime? refreshedAt;

  final Set<ReportAttempt> _openReports = {};

  /// A report form is open: [attempt] is what the loader has started but not finished sending.
  void reportFormOpened(ReportAttempt attempt) => _openReports.add(attempt);
  void reportFormClosed(ReportAttempt attempt) => _openReports.remove(attempt);

  /// Something this loader started is not finished: an action is still on its way to the server,
  /// or a report form is open (not sent yet, or filed with its photo still to upload). The app
  /// keeps nothing for later, so signing out now would throw that away; the Switch user button
  /// asks first. Everything already sent is on the server under this loader's name.
  bool get hasUnsentWork => busy || _openReports.isNotEmpty;

  LoadingTrip? get selected => details[selectedTripId] ?? trips.where((t) => t.tripId == selectedTripId).firstOrNull;
  List<LoadingTrip> get allTrips => trips.map((t) => details[t.tripId] ?? t).toList();
  ReadyCheck checkFor(String tripId) => readyChecks.putIfAbsent(tripId, ReadyCheck.new);

  Future<void> load() async {
    loading = true;
    error = '';
    notifyListeners();
    try {
      final body = await api.get('/loading/trips?date=$date');
      trips = ((body as Map<String, dynamic>)['items'] as List? ?? const []).whereType<Map<String, dynamic>>().map(LoadingTrip.new).toList();
      details.clear();
      for (final t in trips) {
        details[t.tripId] = LoadingTrip(await api.get('/loading/trips/${t.tripId}') as Map<String, dynamic>);
      }
      final a = await api.get('/loading/alerts?date=$date');
      alerts = ((a as Map<String, dynamic>)['items'] as List? ?? const []).whereType<Map<String, dynamic>>().map(DockAlert.new).toList();
      if (selectedTripId.isEmpty || !details.containsKey(selectedTripId)) {
        selectedTripId = trips.isEmpty ? '' : (trips.where((t) => !t.ready).firstOrNull ?? trips.first).tripId;
      }
      connectionLost = false;
      refreshedAt = DateTime.now();
    } on ConnectionLostException {
      connectionLost = true;
    } on ApiException catch (e) {
      error = describe(e);
    } finally {
      loading = false;
      notifyListeners();
    }
  }

  Future<void> refreshTrip(String tripId) async {
    try {
      details[tripId] = LoadingTrip(await api.get('/loading/trips/$tripId') as Map<String, dynamic>);
      connectionLost = false;
      refreshedAt = DateTime.now();
    } on ConnectionLostException {
      connectionLost = true;
    } on ApiException catch (e) {
      error = describe(e);
    }
    notifyListeners();
  }

  void select(String tripId) {
    selectedTripId = tripId;
    notifyListeners();
  }

  void setDate(String value) {
    date = value;
    selectedTripId = '';
    load();
  }

  /// Runs one server action, then re-reads the trip. Returns a user-facing
  /// error message, or null on success.
  Future<String?> _act(String tripId, Future<void> Function() action) async {
    if (busy) return 'Please wait for the last action to finish.';
    busy = true;
    error = '';
    notifyListeners();
    try {
      await action();
      connectionLost = false;
      return null;
    } on ConnectionLostException {
      connectionLost = true;
      return 'Connection lost. Nothing was saved — check the network and try again.';
    } on ApiException catch (e) {
      return describe(e);
    } finally {
      busy = false;
      await refreshTrip(tripId);
    }
  }

  static String describe(ApiException e) {
    switch (e.type) {
      case 'dispatcher_decision_required':
        return 'A shortfall is waiting for the dispatcher\'s decision. Ready to depart stays locked until they decide.';
      case 'loading_incomplete':
        return 'Some orders are not loaded or reported yet.';
      case 'ACTIVE_LOADING_ISSUE':
        return 'This order has a reported shortfall. Withdraw the report first, or wait for the dispatcher.';
    }
    if (e.status == 401) return 'Your sign-in has expired. Sign in again.';
    if (e.status == 403) return 'Your account is not allowed to do this for this depot.';
    if (e.status == 413) return 'The photo is too large. Take it again at a lower resolution.';
    final d = e.detail;
    if (d.contains('chilled zone')) return d.replaceFirst('conflict: ', '');
    if (d.contains('plan version changed') || d.contains('acknowledge')) return 'The dispatcher published a new plan version. Acknowledge the revised load, then continue.';
    return d.isEmpty ? 'The request failed (${e.status}).' : d.replaceFirst(RegExp(r'^(invalid|conflict): '), '');
  }

  List<StopGroup> stops(LoadingTrip trip) {
    final map = <int, List<LoadingOrder>>{};
    for (final o in trip.orders) {
      map.putIfAbsent(o.stopSequence, () => []).add(o);
    }
    return map.entries.map((e) => StopGroup(e.key, e.value)).toList()..sort((a, b) => a.stopSequence.compareTo(b.stopSequence));
  }

  /// Load guidance walks the stops in reverse: the last stop goes in first.
  List<StopGroup> loadOrder(LoadingTrip trip) => stops(trip).reversed.toList();

  /// Trips on the list for the same vehicle, in trip order ("Trip 1 of 2").
  List<LoadingTrip> vehicleTrips(LoadingTrip trip) => allTrips.where((t) => t.vehicleId == trip.vehicleId).toList()..sort((a, b) => a.tripNumber.compareTo(b.tripNumber));

  /// When the vehicle's earlier trip is back, for "Loads after Trip 1 returns".
  DateTime? earlierTripReturn(LoadingTrip trip) => vehicleTrips(trip).where((t) => t.tripNumber < trip.tripNumber).map((t) => t.returnAt).whereType<DateTime>().fold<DateTime?>(null, (a, b) => a == null || b.isAfter(a) ? b : a);

  /// Acknowledges the current plan version. If loading already started on an
  /// older version, the loading session then moves onto the new one.
  Future<String?> acknowledgePlan(LoadingTrip trip) => _act(trip.tripId, () async {
        await api.post('/planning/plans/${trip.planId}/acknowledgements', body: {'version': trip.planVersion});
        if (trip.planChanged) {
          await api.post('/loading/trips/${trip.tripId}/sync', headers: {'If-Match': '${trip.planVersion}'});
        }
      });

  Future<String?> startLoading(LoadingTrip trip) => _act(trip.tripId, () async {
        await api.post('/loading/trips/${trip.tripId}/start', headers: {'Idempotency-Key': newOperationId(), 'If-Match': '${trip.planVersion}'});
      });

  Future<void> _ensureStarted(LoadingTrip trip) async {
    if (!trip.started) {
      await api.post('/loading/trips/${trip.tripId}/start', headers: {'Idempotency-Key': newOperationId(), 'If-Match': '${trip.planVersion}'});
    }
  }

  Future<void> _load(LoadingTrip trip, LoadingOrder order, TechCustody? custody) async {
    if (custody != null) {
      final key = newOperationId();
      await api.post('/orders/${Uri.encodeComponent(order.orderId)}/custody', headers: {'Idempotency-Key': key}, body: {
        'stage': 'LOADED',
        'sealId': custody.sealId,
        'serialNumbers': custody.serials,
        'condition': custody.condition,
        if (custody.evidenceRef.isNotEmpty) 'evidenceRef': custody.evidenceRef,
        'idempotencyKey': key,
      });
    }
    await api.put('/loading/trips/${trip.tripId}/orders/${order.orderId}/loaded', headers: {'Idempotency-Key': newOperationId()});
  }

  /// Marks an order loaded. High-value Tech orders also record a custody
  /// event (seal, serials, condition) before the load is confirmed.
  Future<String?> markLoaded(LoadingTrip trip, LoadingOrder order, {TechCustody? custody}) => _act(trip.tripId, () async {
        await _ensureStarted(trip);
        await _load(trip, order, custody);
      });

  /// "Mark Stop N loaded": every order at the stop that is not loaded or reported.
  Future<String?> markStopLoaded(LoadingTrip trip, StopGroup stop, Map<String, TechCustody> custody) => _act(trip.tripId, () async {
        await _ensureStarted(trip);
        for (final o in stop.orders.where((o) => !o.loaded && !o.short)) {
          await _load(trip, o, custody[o.orderId]);
        }
      });

  /// Missing/damaged/wrong-item report, with an optional photo. The order is
  /// never reduced here: the dispatcher decides.
  ///
  /// [attempt] belongs to one open report form. It keeps the idempotency key and, once the
  /// report exists, its ID, so a failed photo upload is retried on its own: pressing Send
  /// again never files the same report twice.
  Future<String?> reportIssue(LoadingTrip trip, LoadingOrder order, {required ReportAttempt attempt, required String type, required int units, String note = '', List<int>? photo, String photoMime = 'image/jpeg'}) => _act(trip.tripId, () async {
        if (attempt.issueId == null) {
          await _ensureStarted(trip);
          // The same key every time this form is sent: if the first answer was lost on the
          // way back, the server returns the report it already filed instead of a second one.
          final created = await api.post('/loading/trips/${trip.tripId}/orders/${order.orderId}/issues', headers: {'Idempotency-Key': attempt.key}, body: {'type': type, 'affectedUnits': units, if (note.isNotEmpty) 'note': note});
          final id = ((created as Map<String, dynamic>)['issue'] as Map<String, dynamic>?)?['id'];
          if (id is! String) throw ApiException(502, '{"detail":"The report was filed but its ID was not returned."}');
          attempt.issueId = id;
        }
        if (photo != null && !attempt.photoSent) {
          await api.upload('/loading/trips/${trip.tripId}/orders/${order.orderId}/issues/${attempt.issueId}/photo', bytes: photo, mime: photoMime);
          attempt.photoSent = true;
        }
      });

  /// Withdraws a report made by mistake (only while the dispatcher has not decided, or on hold).
  Future<String?> clearIssue(LoadingTrip trip, LoadingOrder order, LoadingIssue issue) => _act(trip.tripId, () async {
        await api.delete('/loading/trips/${trip.tripId}/orders/${order.orderId}/issues/${issue.id}');
      });

  Future<String?> confirmReady(LoadingTrip trip) => _act(trip.tripId, () async {
        final c = checkFor(trip.tripId);
        await api.post('/loading/trips/${trip.tripId}/ready', headers: {'Idempotency-Key': newOperationId()}, body: {
          if (trip.refrigerated && c.temperatureC != null) 'chilledTemperatureC': c.temperatureC,
          if (c.seal.isNotEmpty) 'sealNumber': c.seal,
        });
      });

  /// "Tell dispatcher": goods for another vehicle were staged at this one.
  Future<String?> tellDispatcher(LoadingTrip trip, String orderRef, String belongsVehicleId) => _act(trip.tripId, () async {
        await api.post('/loading/trips/${trip.tripId}/alerts', headers: {'Idempotency-Key': newOperationId()}, body: {'type': 'WRONG_VEHICLE', 'orderRef': orderRef, 'belongsVehicleId': belongsVehicleId});
        final a = await api.get('/loading/alerts?date=$date');
        alerts = ((a as Map<String, dynamic>)['items'] as List? ?? const []).whereType<Map<String, dynamic>>().map(DockAlert.new).toList();
      });

  /// Wrong-vehicle check: which trip on today's list an order number belongs to.
  (LoadingTrip, LoadingOrder)? findOrder(String code) {
    final q = code.trim().toLowerCase();
    if (q.isEmpty) return null;
    for (final t in details.values) {
      for (final o in t.orders) {
        if (o.orderRef.toLowerCase() == q || o.orderId.toLowerCase() == q) return (t, o);
      }
    }
    return null;
  }

  /// Notifications for the loader: plan versions, dispatcher decisions,
  /// reports seen, and answered dock alerts, newest first.
  List<LoaderNotice> notices() {
    final out = <LoaderNotice>[];
    for (final t in allTrips) {
      if (t.planVersion > 1 && t.planPublishedAt != null) {
        out.add(LoaderNotice(t.planPublishedAt, 'Plan updated to v${t.planVersion} · ${t.vehicleId}', t.needsAck ? 'Acknowledge the revised load before loading further.' : 'Acknowledged.', t, tone: t.needsAck ? 'amber' : 'muted'));
      }
      for (final o in t.orders) {
        for (final i in o.issues) {
          if (i.decision != null) {
            out.add(LoaderNotice(i.decidedAt, 'Dispatcher decision · ${o.orderRef}', '${i.decisionLabel}${i.decisionNote.isNotEmpty ? ' · ${i.decisionNote}' : ''}', t, tone: i.allowsDeparture ? 'green' : 'red'));
          } else if (i.seenAt != null) {
            out.add(LoaderNotice(i.seenAt, 'Report seen by the dispatcher · ${o.orderRef}', 'Waiting for a decision. Keep loading the other stops.', t));
          }
        }
      }
    }
    for (final a in alerts.where((a) => a.resolvedAt != null)) {
      final trip = allTrips.where((t) => t.tripId == a.tripId).firstOrNull;
      if (trip != null) out.add(LoaderNotice(a.resolvedAt, 'Dispatcher handled ${a.orderRef}', 'Your wrong-vehicle alert was closed.', trip, tone: 'muted'));
    }
    out.sort((a, b) => (b.at ?? DateTime(0)).compareTo(a.at ?? DateTime(0)));
    return out;
  }

  /// Notices that still need the loader: unacknowledged plans and new decisions.
  int get actionCount => allTrips.where((t) => t.needsAck && t.planVersion > 1).length + allTrips.fold(0, (s, t) => s + t.orders.where((o) => o.issues.any((i) => i.decision != null && !t.ready)).length);
}

class TechCustody {
  TechCustody({required this.sealId, required this.serials, required this.condition, this.evidenceRef = ''});
  final String sealId;
  final List<String> serials;
  final String condition;
  final String evidenceRef;
}
