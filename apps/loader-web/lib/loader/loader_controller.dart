import 'package:flutter/foundation.dart';

import '../api/api_client.dart';
import '../shared/models.dart';
import '../widgets/common.dart';

class StopGroup {
  StopGroup(this.stopSequence, this.orders);
  final int stopSequence;
  final List<LoadingOrder> orders;
  String get outletId => orders.first.outletId;
  bool get chilled => orders.any((o) => o.chilled);
  /// Loaded, or short with a dispatcher decision that lets the trip leave.
  bool get loaded => orders.every((o) => o.loaded || (o.short && !o.unresolved));
  bool get hasShortfall => orders.any((o) => o.short);
  int get expectedUnits => orders.fold(0, (s, o) => s + o.expectedUnits);
}

/// Loader dock workflow against the loading, planning and order services.
/// Always online: every action goes straight to the server and the trip is
/// re-read afterwards, so the screen always shows the server's state.
class LoaderController extends ChangeNotifier {
  LoaderController({required this.api});

  final ApiClient api;

  List<LoadingTrip> trips = [];
  final Map<String, LoadingTrip> details = {};
  String selectedTripId = '';
  String date = todayIso();
  bool loading = true;
  bool busy = false;
  bool connectionLost = false;
  String error = '';
  DateTime? refreshedAt;

  LoadingTrip? get selected => details[selectedTripId] ?? trips.where((t) => t.tripId == selectedTripId).firstOrNull;

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
        return 'This order has a reported shortfall. Clear the report first, or wait for the dispatcher.';
    }
    if (e.status == 401) return 'Your sign-in has expired. Sign in again.';
    if (e.status == 403) return 'Your account is not allowed to do this for this depot.';
    final d = e.detail;
    if (d.contains('plan version changed') || d.contains('acknowledge')) return 'The dispatcher published a new plan version. Acknowledge the revised load, then continue.';
    return d.isEmpty ? 'The request failed (${e.status}).' : d;
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

  Future<String?> acknowledgePlan(LoadingTrip trip) => _act(trip.tripId, () async {
        await api.post('/planning/plans/${trip.planId}/acknowledgements', body: {'version': trip.planVersion});
      });

  Future<String?> startLoading(LoadingTrip trip) => _act(trip.tripId, () async {
        await api.post('/loading/trips/${trip.tripId}/start', headers: {'Idempotency-Key': newOperationId(), 'If-Match': '${trip.planVersion}'});
      });

  Future<void> _ensureStarted(LoadingTrip trip) async {
    if (!trip.started) {
      await api.post('/loading/trips/${trip.tripId}/start', headers: {'Idempotency-Key': newOperationId(), 'If-Match': '${trip.planVersion}'});
    }
  }

  /// Marks an order loaded. High-value Tech orders also record a custody
  /// event (seal, serials, condition) before the load is confirmed.
  Future<String?> markLoaded(LoadingTrip trip, LoadingOrder order, {TechCustody? custody}) => _act(trip.tripId, () async {
        await _ensureStarted(trip);
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
      });

  /// Missing/damaged report. The order is never reduced here: the dispatcher decides.
  Future<String?> reportIssue(LoadingTrip trip, LoadingOrder order, {required String type, required int units, String note = ''}) => _act(trip.tripId, () async {
        await _ensureStarted(trip);
        await api.post('/loading/trips/${trip.tripId}/orders/${order.orderId}/issues', headers: {'Idempotency-Key': newOperationId()}, body: {'type': type, 'affectedUnits': units, if (note.isNotEmpty) 'note': note});
      });

  /// Withdraws a report made by mistake (only while the dispatcher has not decided).
  Future<String?> clearIssue(LoadingTrip trip, LoadingOrder order, LoadingIssue issue) => _act(trip.tripId, () async {
        await api.delete('/loading/trips/${trip.tripId}/orders/${order.orderId}/issues/${issue.id}');
      });

  Future<String?> confirmReady(LoadingTrip trip) => _act(trip.tripId, () async {
        await api.post('/loading/trips/${trip.tripId}/ready', headers: {'Idempotency-Key': newOperationId()});
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
}

class TechCustody {
  TechCustody({required this.sealId, required this.serials, required this.condition, this.evidenceRef = ''});
  final String sealId;
  final List<String> serials;
  final String condition;
  final String evidenceRef;
}
