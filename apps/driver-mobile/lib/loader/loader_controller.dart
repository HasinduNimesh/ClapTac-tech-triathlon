import 'package:flutter/foundation.dart';

import '../api/api_client.dart';
import '../offline/store.dart';
import '../shared/models.dart';
import '../sync/sync.dart';
import '../widgets/common.dart';

class StopGroup {
  StopGroup(this.stopSequence, this.orders);
  final int stopSequence;
  final List<LoadingOrder> orders;
  String get outletId => orders.first.outletId;
  bool get chilled => orders.any((o) => o.chilled);
  bool get loaded => orders.every((o) => o.loaded);
  bool get hasShortfall => orders.any((o) => o.short);
  int get expectedUnits => orders.fold(0, (s, o) => s + o.expectedUnits);
}

/// Loader dock workflow: versioned load list, reverse-stop loading guidance,
/// shortfall reports and ready-to-depart. Writes are queued like the driver's.
class LoaderController extends ChangeNotifier {
  LoaderController({required this.api, required this.store, required this.sync, required this.ownerId});

  final ApiClient api;
  final LocalStore store;
  final SyncEngine sync;
  final String ownerId;

  List<LoadingTrip> trips = [];
  final Map<String, LoadingTrip> details = {};
  String selectedTripId = '';
  bool loading = true;
  bool offline = false;
  String error = '';
  final Map<String, String> waitingForDispatcher = {};
  final Set<String> readyConfirmed = {};

  String get _listKey => 'loader.trips.$ownerId.${todayIso()}';
  LoadingTrip? get selected => details[selectedTripId] ?? trips.where((t) => t.tripId == selectedTripId).firstOrNull;

  Future<void> load() async {
    loading = true;
    error = '';
    notifyListeners();
    try {
      final body = await api.get('/loading/trips?date=${todayIso()}');
      trips = ((body as Map<String, dynamic>)['items'] as List? ?? const []).whereType<Map<String, dynamic>>().map(LoadingTrip.new).toList();
      await store.writeJson(_listKey, {'items': trips.map((t) => t.raw).toList()});
      for (final t in trips) {
        final d = await api.get('/loading/trips/${t.tripId}');
        details[t.tripId] = LoadingTrip(d as Map<String, dynamic>);
        await store.writeJson('loader.trip.$ownerId.${t.tripId}', d);
      }
      offline = false;
      await sync.drain();
    } on OfflineException {
      offline = true;
      final cached = await store.readJson(_listKey);
      trips = ((cached?['items'] as List?) ?? const []).whereType<Map<String, dynamic>>().map(LoadingTrip.new).toList();
      for (final t in trips) {
        final d = await store.readJson('loader.trip.$ownerId.${t.tripId}');
        if (d != null) details[t.tripId] = LoadingTrip(d);
      }
    } on ApiException catch (e) {
      error = e.status == 401 ? 'Your sign-in has expired. Sign in again; saved work stays on this device.' : 'The load list could not be loaded (${e.status}).';
    } finally {
      if (selectedTripId.isEmpty && trips.isNotEmpty) selectedTripId = (trips.where((t) => !t.ready).firstOrNull ?? trips.first).tripId;
      loading = false;
      notifyListeners();
    }
  }

  void select(String tripId) {
    selectedTripId = tripId;
    notifyListeners();
  }

  /// Stops in drop-off order. Loading guidance walks them in reverse: the last
  /// stop goes in first (front of cargo), the first stop goes in by the doors.
  List<StopGroup> stops(LoadingTrip trip) {
    final map = <int, List<LoadingOrder>>{};
    for (final o in trip.orders) {
      map.putIfAbsent(o.stopSequence, () => []).add(o);
    }
    return map.entries.map((e) => StopGroup(e.key, e.value)).toList()..sort((a, b) => a.stopSequence.compareTo(b.stopSequence));
  }

  List<StopGroup> loadOrder(LoadingTrip trip) => stops(trip).reversed.toList();

  Future<void> _refreshTrip(String tripId, Map<String, dynamic> Function(Map<String, dynamic>) change) async {
    final t = details[tripId];
    if (t == null) return;
    final updated = LoadingTrip(change(Map<String, dynamic>.from(t.raw)));
    details[tripId] = updated;
    await store.writeJson('loader.trip.$ownerId.$tripId', updated.raw);
    notifyListeners();
  }

  Future<void> _queue(QueuedOperation op) async {
    await sync.enqueue(op);
    await sync.drain();
    offline = sync.state.phase == SyncPhase.offline;
    notifyListeners();
  }

  Future<void> acknowledgePlan(LoadingTrip trip) async {
    await _refreshTrip(trip.tripId, (raw) => raw..['acknowledgedVersion'] = trip.planVersion);
    await _queue(QueuedOperation(operationId: newOperationId(), type: 'PLAN_ACK', tripId: trip.tripId, payload: {'planId': trip.planId, 'version': trip.planVersion}));
  }

  Future<void> startLoading(LoadingTrip trip) async {
    if (trip.started) return;
    await _refreshTrip(trip.tripId, (raw) => raw..['loadingStatus'] = 'in_progress');
    await _queue(QueuedOperation(operationId: newOperationId(), type: 'LOAD_START', tripId: trip.tripId, payload: {'planVersion': trip.planVersion}));
  }

  Future<void> markLoaded(LoadingTrip trip, LoadingOrder order) async {
    await startLoading(trip);
    await _refreshTrip(trip.tripId, (raw) => raw..['orders'] = [for (final o in trip.orders) o.orderId == order.orderId ? {...o.raw, 'status': 'loaded'} : o.raw]);
    await _queue(QueuedOperation(operationId: newOperationId(), type: 'ORDER_LOADED', tripId: trip.tripId, orderId: order.orderId));
  }

  /// Missing/damaged report. The order is never reduced here: the dispatcher decides.
  Future<void> reportIssue(LoadingTrip trip, LoadingOrder order, {required String type, required int units, String note = ''}) async {
    await startLoading(trip);
    final issue = {'id': 'local-${newOperationId()}', 'type': type, 'affectedUnits': units, 'note': note};
    await _refreshTrip(trip.tripId, (raw) => raw..['orders'] = [for (final o in trip.orders) o.orderId == order.orderId ? {...o.raw, 'status': 'shortfall', 'issues': [...o.issues.map((i) => i.raw), issue]} : o.raw]);
    waitingForDispatcher[trip.tripId] = '${order.orderRef}: $units ${type.toLowerCase()} · reported ${TimeOfDayLabel.now()}';
    await _queue(QueuedOperation(operationId: newOperationId(), type: 'LOAD_ISSUE', tripId: trip.tripId, orderId: order.orderId, payload: {'type': type, 'affectedUnits': units, if (note.isNotEmpty) 'note': note}));
  }

  Future<bool> confirmReady(LoadingTrip trip) async {
    await _queue(QueuedOperation(operationId: newOperationId(), type: 'LOAD_READY', tripId: trip.tripId));
    if (sync.state.phase == SyncPhase.error) return false;
    readyConfirmed.add(trip.tripId);
    await _refreshTrip(trip.tripId, (raw) => raw..['loadingStatus'] = 'ready');
    return true;
  }

  /// Wrong-vehicle check: finds which trip on today's list an order number belongs to.
  (LoadingTrip, LoadingOrder)? findOrder(String orderRef) {
    final q = orderRef.trim().toLowerCase();
    if (q.isEmpty) return null;
    for (final t in details.values) {
      for (final o in t.orders) {
        if (o.orderRef.toLowerCase() == q || o.orderId.toLowerCase() == q) return (t, o);
      }
    }
    return null;
  }
}

class TimeOfDayLabel {
  static String now() {
    final d = DateTime.now().toUtc().add(const Duration(hours: 5, minutes: 30));
    return '${d.hour.toString().padLeft(2, '0')}:${d.minute.toString().padLeft(2, '0')}';
  }
}
