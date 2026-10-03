// JSON models for the loading, planning and shared services. Field names match the
// web client (apps/web/src/api/delivery.ts and loading.ts).

String? _s(Map<String, dynamic> j, String k) => j[k] is String && (j[k] as String).isNotEmpty ? j[k] as String : null;
int? _i(Map<String, dynamic> j, String k) => j[k] is num ? (j[k] as num).toInt() : null;
double? _d(Map<String, dynamic> j, String k) => j[k] is num ? (j[k] as num).toDouble() : null;
DateTime? _t(Map<String, dynamic> j, String k) => _s(j, k) == null ? null : DateTime.tryParse(_s(j, k)!)?.toUtc();
List<Map<String, dynamic>> _list(Map<String, dynamic> j, String k) => ((j[k] as List?) ?? const []).whereType<Map<String, dynamic>>().toList();

class Profile {
  Profile({required this.userId, required this.roles, this.depot, this.vehicleId, this.displayName});
  final String userId;
  final List<String> roles;
  final String? depot;
  final String? vehicleId;
  final String? displayName;

  String get role => roles.isEmpty ? '' : roles.first.toUpperCase();

  factory Profile.fromJson(Map<String, dynamic> j) => Profile(
        userId: _s(j, 'userId') ?? '',
        roles: ((j['roles'] as List?) ?? const []).map((e) => '$e').toList(),
        depot: _s(j, 'depot'),
        vehicleId: _s(j, 'vehicleId'),
        displayName: _s(j, 'displayName'),
      );
  Map<String, dynamic> toJson() => {'userId': userId, 'roles': roles, 'depot': depot, 'vehicleId': vehicleId, 'displayName': displayName};
}

const depotLabels = {'DEPOT_NORTH': 'Peliyagoda', 'DEPOT_SOUTH': 'Kandy'};
String depotName(String? code) => depotLabels[code] ?? (code ?? '');

/// "Rear dock", "Curb", "Mall bay" from the outlet's dock type.
String dockLabel(String dockType) => switch (dockType) { 'rear_dock' => 'Rear dock', 'street' => 'Curb', 'mall_bay' => 'Mall bay', _ => '' };

/// HH:MM in Sri Lanka time for an instant, '' when unknown.
String clock(DateTime? t) {
  if (t == null) return '';
  final c = t.toUtc().add(const Duration(hours: 5, minutes: 30)); // Asia/Colombo
  return '${c.hour.toString().padLeft(2, '0')}:${c.minute.toString().padLeft(2, '0')}';
}

/// Minutes until a planned time (negative once it has passed).
int minutesUntil(DateTime t, [DateTime? now]) => t.difference((now ?? DateTime.now()).toUtc()).inMinutes;

/// "05:00" from "05:00:00".
String hm(String v) => v.length >= 5 ? v.substring(0, 5) : v;

String kg(double v) => '${v.round().toString().replaceAllMapped(RegExp(r'(\d)(?=(\d{3})+$)'), (m) => '${m[1]},')} kg';
String m3(double v) => '${v.toStringAsFixed(1)} m³';

class LoadingIssue {
  LoadingIssue(this.raw);
  final Map<String, dynamic> raw;
  String get id => _s(raw, 'id') ?? '';
  String get type => _s(raw, 'type') ?? '';
  String get typeLabel => switch (type) { 'MISSING' => 'missing', 'DAMAGED' => 'damaged', 'WRONG_ITEM' => 'wrong item', _ => type.toLowerCase() };
  int get units => _i(raw, 'affectedUnits') ?? 0;
  String get note => _s(raw, 'note') ?? '';
  String get reportedBy => _s(raw, 'reportedBy') ?? '';
  DateTime? get reportedAt => _t(raw, 'reportedAt');
  bool get hasPhoto => raw['hasPhoto'] == true;
  String? get seenBy => _s(raw, 'seenBy');
  DateTime? get seenAt => _t(raw, 'seenAt');
  /// Dispatcher decision: PARTIAL_LOAD, HOLD or MOVE_TO_NEXT_RUN (null while waiting).
  String? get decision => _s(raw, 'decision');
  String get decisionNote => _s(raw, 'decisionNote') ?? '';
  String? get decidedBy => _s(raw, 'decidedBy');
  DateTime? get decidedAt => _t(raw, 'decidedAt');
  bool get allowsDeparture => decision == 'PARTIAL_LOAD' || decision == 'MOVE_TO_NEXT_RUN';
  String get decisionLabel => switch (decision) { 'PARTIAL_LOAD' => 'Partial load accepted', 'HOLD' => 'On hold until stock arrives', 'MOVE_TO_NEXT_RUN' => 'Moved to the next run', _ => 'Waiting for dispatcher' };
}

class LoadingOrder {
  LoadingOrder(this.raw);
  final Map<String, dynamic> raw;
  String get orderId => _s(raw, 'orderId') ?? '';
  String get orderRef => _s(raw, 'orderRef') ?? orderId;
  String get outletId => _s(raw, 'outletId') ?? '';
  String get outletName => _s(raw, 'outletName') ?? '';
  String get brand => _s(raw, 'brand') ?? '';
  String get temperature => _s(raw, 'temperatureRequirement') ?? '';
  bool get chilled => RegExp('chill|frozen|refriger', caseSensitive: false).hasMatch(temperature);
  int get expectedUnits => _i(raw, 'expectedUnits') ?? 0;
  int get stopSequence => _i(raw, 'stopSequence') ?? 0;
  int get loadSequence => _i(raw, 'suggestedLoadSequence') ?? 0;
  double get weightKg => _d(raw, 'weightKg') ?? 0;
  double get volumeM3 => _d(raw, 'volumeM3') ?? 0;
  String get district => _s(raw, 'district') ?? '';
  String get windowOpen => hm(_s(raw, 'windowOpen') ?? '');
  String get windowClose => hm(_s(raw, 'windowClose') ?? '');
  String get window => windowOpen.isEmpty ? '' : '$windowOpen–$windowClose${raw['mallWindow'] == true ? ' mall' : ''}';
  String get dock => dockLabel(_s(raw, 'dockType') ?? '');
  bool get vanOnly => _s(raw, 'parkingConstraint') == 'van_only';
  int get changedInVersion => _i(raw, 'changedInVersion') ?? 0;
  String get changeNote => _s(raw, 'changeNote') ?? '';
  String get status => _s(raw, 'status') ?? 'pending';
  bool get loaded => status == 'loaded';
  List<LoadingIssue> get issues => _list(raw, 'issues').map(LoadingIssue.new).toList();
  int get affectedUnits => issues.fold(0, (s, i) => s + i.units);
  bool get short => issues.isNotEmpty || status == 'shortfall';
  /// A shortfall the dispatcher has not cleared for departure yet.
  bool get unresolved => short && (issues.isEmpty || issues.any((i) => !i.allowsDeparture));
  /// Units going on the truck: everything, or what is left after a decided shortfall.
  int get loadedUnits => loaded ? expectedUnits : (short && !unresolved ? expectedUnits - affectedUnits : 0);
}

/// One line of "what changed" between the plan version being loaded and the newer one.
class PlanChange {
  PlanChange(this.raw);
  final Map<String, dynamic> raw;
  String get kind => _s(raw, 'kind') ?? '';
  String get orderRef => _s(raw, 'orderRef') ?? '';
  String get outletId => _s(raw, 'outletId') ?? '';
  int get fromStop => _i(raw, 'fromStop') ?? 0;
  int get toStop => _i(raw, 'toStop') ?? 0;
  bool get wasLoaded => raw['loaded'] == true;
  String get text => switch (kind) {
        'MOVED' => 'Stop $fromStop $outletId moved to Stop $toStop${wasLoaded ? ' — already loaded, reposition it' : ''}',
        'REMOVED' => '$orderRef ($outletId, Stop $fromStop) taken off this trip${wasLoaded ? ' — unload it' : ''}',
        'ADDED' => '$orderRef ($outletId) added at Stop $toStop',
        _ => orderRef,
      };
}

class LoadingTrip {
  LoadingTrip(this.raw);
  final Map<String, dynamic> raw;
  String get tripId => _s(raw, 'tripId') ?? '';
  String get planRef => _s(raw, 'planRef') ?? '';
  String get planId => _s(raw, 'planId') ?? '';
  String get vehicleId => _s(raw, 'vehicleId') ?? '';
  int get tripNumber => _i(raw, 'tripNumber') ?? 1;
  String get depot => _s(raw, 'depot') ?? '';
  String get vehicleType => _s(raw, 'vehicleType') ?? '';
  String get capability => _s(raw, 'vehicleTemperatureCapability') ?? '';
  bool get refrigerated => RegExp('chill|frozen|refriger|multi|reefer', caseSensitive: false).hasMatch(capability);
  String get brand => orders.map((o) => o.brand).firstWhere((b) => b.isNotEmpty, orElse: () => '');
  List<String> get areas => ((raw['areas'] as List?) ?? const []).map((e) => '$e').toList();
  int get stopCount => orders.isNotEmpty ? orders.map((o) => o.stopSequence).toSet().length : (_i(raw, 'stopCount') ?? 0);
  String get loadingStatus => _s(raw, 'loadingStatus') ?? _s(raw, 'status') ?? 'pending';
  bool get ready => RegExp('ready', caseSensitive: false).hasMatch(loadingStatus);
  bool get started => RegExp('progress|started|ready|loaded', caseSensitive: false).hasMatch(loadingStatus);
  int get loadedCount => _i(raw, 'loadedCount') ?? orders.where((o) => o.loaded).length;
  int get planVersion => _i(raw, 'planVersion') ?? 1;
  int get preparedPlanVersion => _i(raw, 'preparedPlanVersion') ?? planVersion;
  int get acknowledgedVersion => _i(raw, 'acknowledgedVersion') ?? 0;
  DateTime? get acknowledgedAt => _t(raw, 'acknowledgedAt');
  /// The loader has not acknowledged the current plan version yet.
  bool get needsAck => !ready && acknowledgedVersion < planVersion;
  /// Loading started on an older version: acknowledging moves it onto the new one.
  bool get planChanged => raw['planChanged'] == true;
  List<PlanChange> get changes => _list(raw, 'changes').map(PlanChange.new).toList();
  DateTime? get planPublishedAt => _t(raw, 'planPublishedAt');
  String get planPublishedBy => _s(raw, 'planPublishedBy') ?? '';
  DateTime? get departAt => _t(raw, 'plannedDepartureAt');
  DateTime? get returnAt => _t(raw, 'plannedReturnAt');
  double get weightKg => _d(raw, 'totalWeightKg') ?? orders.fold(0.0, (s, o) => s + o.weightKg);
  double get volumeM3 => _d(raw, 'totalVolumeM3') ?? orders.fold(0.0, (s, o) => s + o.volumeM3);
  double get chilledM3 => _d(raw, 'chilledVolumeM3') ?? 0;
  double get weightCap => _d(raw, 'vehicleWeightCapacityKg') ?? 0;
  double get volumeCap => _d(raw, 'vehicleVolumeCapacityM3') ?? 0;
  int? get tripMinutes => _i(raw, 'tripMinutes');
  int? get freshBudget => _i(raw, 'freshBudgetMinutes');
  String get readyBy => _s(raw, 'readyBy') ?? '';
  DateTime? get readyAt => _t(raw, 'readyAt');
  double? get readyTemperatureC => _d(raw, 'readyTemperatureC');
  String get readySeal => _s(raw, 'readySeal') ?? '';
  bool get vanOnly => orders.any((o) => o.vanOnly);
  List<LoadingOrder> get orders => (_list(raw, 'orders').map(LoadingOrder.new).toList())..sort((a, b) => a.loadSequence.compareTo(b.loadSequence));
}

/// A loader's "tell dispatcher" note about goods at the wrong vehicle.
class DockAlert {
  DockAlert(this.raw);
  final Map<String, dynamic> raw;
  String get id => _s(raw, 'id') ?? '';
  String get tripId => _s(raw, 'tripId') ?? '';
  String get orderRef => _s(raw, 'orderRef') ?? '';
  String get belongsVehicleId => _s(raw, 'belongsVehicleId') ?? '';
  DateTime? get createdAt => _t(raw, 'createdAt');
  DateTime? get resolvedAt => _t(raw, 'resolvedAt');
}
