// JSON models for the delivery and loading services. Field names match the
// web client (apps/web/src/api/delivery.ts and loading.ts).

String? _s(Map<String, dynamic> j, String k) => j[k] is String ? j[k] as String : null;
int? _i(Map<String, dynamic> j, String k) => j[k] is num ? (j[k] as num).toInt() : null;

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

class DeliveryStop {
  DeliveryStop(this.raw);
  final Map<String, dynamic> raw;

  String get id => _s(raw, 'id') ?? '';
  String get orderId => _s(raw, 'orderId') ?? '';
  String get orderRef => _s(raw, 'orderRef') ?? orderId;
  String get outletId => _s(raw, 'outletId') ?? '';
  String get outletName => _s(raw, 'outletName') ?? outletId;
  String get brand => _s(raw, 'brand') ?? '';
  String get temperature => _s(raw, 'temperatureRequirement') ?? '';
  bool get chilled => RegExp('chill|frozen|refriger', caseSensitive: false).hasMatch(temperature);
  String get district => _s(raw, 'district') ?? '';
  String get dockType => _s(raw, 'dockType') ?? '';
  String get parking => _s(raw, 'parkingConstraint') ?? '';
  String get access => _s(raw, 'accessInstructions') ?? '';
  String? get windowOpen => _s(raw, 'plannedWindowOpen');
  String? get windowClose => _s(raw, 'plannedWindowClose');
  int get sequence => _i(raw, 'stopSequence') ?? 0;
  String get status => _s(raw, 'status') ?? 'pending';
  String? get outcomeCode => _s(raw, 'outcomeCode');
  String? get outcomeReason => _s(raw, 'outcomeReason');
  String? get arrivedAt => _s(raw, 'arrivedAt');
  int? get expectedUnits => _i(raw, 'expectedUnits') ?? _i(raw, 'orderUnits');
  bool get done => outcomeCode != null || status == 'completed';
  bool get delivered => outcomeCode == 'DELIVERED';
  List<Map<String, dynamic>> get shortfalls => ((raw['loadingShortfallSummary'] as List?) ?? const []).whereType<Map<String, dynamic>>().toList();

  DeliveryStop copyWith(Map<String, dynamic> changes) => DeliveryStop({...raw, ...changes});
}

class DeliveryTrip {
  DeliveryTrip(this.raw);
  final Map<String, dynamic> raw;

  Map<String, dynamic> get run => (raw['run'] as Map<String, dynamic>?) ?? const {};
  String get tripId => _s(raw, 'tripId') ?? '';
  String get status => _s(raw, 'status') ?? _s(run, 'status') ?? '';
  String get vehicleId => _s(run, 'vehicleId') ?? '';
  String get planRef => _s(run, 'planRef') ?? '';
  String get planId => _s(run, 'planId') ?? '';
  String get depot => _s(run, 'depot') ?? '';
  int get tripNumber => _i(run, 'tripNumber') ?? 1;
  String get deliveryDate => _s(run, 'deliveryDate') ?? '';
  int get planVersion => _i(raw, 'currentPlanVersion') ?? _i(run, 'planVersion') ?? 1;
  int get acknowledgedVersion => _i(run, 'acknowledgedVersion') ?? 0;
  bool get needsAcknowledgement => planVersion > acknowledgedVersion && acknowledgedVersion > 0;
  bool get started => RegExp('progress|started|complete', caseSensitive: false).hasMatch(status) || stops.any((s) => s.arrivedAt != null || s.done);
  bool get completed => RegExp('complete', caseSensitive: false).hasMatch(status);
  List<DeliveryStop> get stops => (((raw['stops'] as List?) ?? const []).whereType<Map<String, dynamic>>().map(DeliveryStop.new).toList())..sort((a, b) => a.sequence.compareTo(b.sequence));
  DeliveryStop? get nextStop {
    for (final s in stops) {
      if (!s.done) return s;
    }
    return null;
  }

  DeliveryTrip withStop(DeliveryStop stop) => DeliveryTrip({...raw, 'stops': stops.map((s) => s.id == stop.id ? stop.raw : s.raw).toList()});
  DeliveryTrip withStatus(String status) => DeliveryTrip({...raw, 'status': status});
}

class TripMessage {
  TripMessage(this.raw);
  final Map<String, dynamic> raw;
  String get id => _s(raw, 'id') ?? '';
  String get body => _s(raw, 'body') ?? '';
  String get sentBy => _s(raw, 'sentBy') ?? '';
  String? get createdAt => _s(raw, 'createdAt');
  String? get acknowledgedAt => _s(raw, 'acknowledgedAt');
}

class LoadingIssue {
  LoadingIssue(this.raw);
  final Map<String, dynamic> raw;
  String get id => _s(raw, 'id') ?? '';
  String get type => _s(raw, 'type') ?? '';
  int get units => _i(raw, 'affectedUnits') ?? 0;
  String get note => _s(raw, 'note') ?? '';
  /// Dispatcher decision: PARTIAL_LOAD, HOLD or MOVE_TO_NEXT_RUN (null while waiting).
  String? get decision => _s(raw, 'decision');
  String get decisionNote => _s(raw, 'decisionNote') ?? '';
  String? get decidedBy => _s(raw, 'decidedBy');
  bool get allowsDeparture => decision == 'PARTIAL_LOAD' || decision == 'MOVE_TO_NEXT_RUN';
  String get decisionLabel => switch (decision) { 'PARTIAL_LOAD' => 'Partial load accepted', 'HOLD' => 'On hold until stock arrives', 'MOVE_TO_NEXT_RUN' => 'Moved to the next run', _ => 'Waiting for dispatcher' };
}

class LoadingOrder {
  LoadingOrder(this.raw);
  final Map<String, dynamic> raw;
  String get orderId => _s(raw, 'orderId') ?? '';
  String get orderRef => _s(raw, 'orderRef') ?? orderId;
  String get outletId => _s(raw, 'outletId') ?? '';
  String get brand => _s(raw, 'brand') ?? '';
  String get temperature => _s(raw, 'temperatureRequirement') ?? '';
  bool get chilled => RegExp('chill|frozen|refriger', caseSensitive: false).hasMatch(temperature);
  int get expectedUnits => _i(raw, 'expectedUnits') ?? 0;
  int get stopSequence => _i(raw, 'stopSequence') ?? 0;
  int get loadSequence => _i(raw, 'suggestedLoadSequence') ?? 0;
  String get status => _s(raw, 'status') ?? 'pending';
  bool get loaded => status == 'loaded';
  List<LoadingIssue> get issues => ((raw['issues'] as List?) ?? const []).whereType<Map<String, dynamic>>().map(LoadingIssue.new).toList();
  bool get short => issues.isNotEmpty || status == 'shortfall';
  /// A shortfall the dispatcher has not cleared for departure yet.
  bool get unresolved => short && (issues.isEmpty || issues.any((i) => !i.allowsDeparture));
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
  bool get refrigerated => RegExp('chill|frozen|refriger|multi', caseSensitive: false).hasMatch(capability);
  int get stopCount => _i(raw, 'stopCount') ?? orders.map((o) => o.stopSequence).toSet().length;
  String get loadingStatus => _s(raw, 'loadingStatus') ?? _s(raw, 'status') ?? 'pending';
  bool get ready => RegExp('ready', caseSensitive: false).hasMatch(loadingStatus);
  bool get started => RegExp('progress|started|ready|loaded', caseSensitive: false).hasMatch(loadingStatus);
  int get loadedCount => _i(raw, 'loadedCount') ?? orders.where((o) => o.loaded).length;
  int get shortfallCount => _i(raw, 'shortfallCount') ?? orders.where((o) => o.short).length;
  int get pendingCount => _i(raw, 'pendingCount') ?? orders.where((o) => !o.loaded && !o.short).length;
  int get planVersion => _i(raw, 'planVersion') ?? 1;
  int get acknowledgedVersion => _i(raw, 'acknowledgedVersion') ?? 0;
  bool get planChanged => acknowledgedVersion > 0 && planVersion > acknowledgedVersion;
  List<LoadingOrder> get orders => (((raw['orders'] as List?) ?? const []).whereType<Map<String, dynamic>>().map(LoadingOrder.new).toList())..sort((a, b) => a.loadSequence.compareTo(b.loadSequence));
}
