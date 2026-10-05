/// View models for the driver UI. They are separate from the sync-layer
/// records in `shared/models.dart` so screens stay easy to build and test.
enum DeliveryOutcome { delivered, partial, failed, refused }

/// Which kind of proof the driver asked to capture.
enum ProofKind { photo, signature }

/// A photo or signature the driver captured, kept as a file on the phone until it is uploaded.
class CapturedProof {
  const CapturedProof({required this.kind, required this.path, required this.mimeType, required this.capturedAt, this.receiverName = ''});

  final ProofKind kind;

  /// A file inside the app's own storage, so it survives a restart while the phone is offline.
  final String path;

  /// `image/jpeg` or `image/png`: the only types the server accepts.
  final String mimeType;
  final DateTime capturedAt;

  /// Who accepted the delivery, when the driver entered a name.
  final String receiverName;
}

enum SyncStateKind { savedOffline, syncing, synced, uploadFailed }

enum ProblemKind { vehicleBreakdown, roadBlocked, outletClosed, loadIssue, safetyConcern }

class ProblemReport {
  const ProblemReport({required this.kind, this.note = ''});

  final ProblemKind kind;
  final String note;
}

class StopInfo {
  const StopInfo({
    this.stopId = '',
    required this.sequence,
    required this.outletCode,
    required this.name,
    required this.windowStart,
    required this.windowEnd,
    this.units,
    this.unitLabel = 'units',
    required this.accessNote,
    required this.contactNote,
    required this.goods,
    this.orderRef = '',
    this.orderId = '',
    this.outcomeCode = '',
    this.deliveredUnits,
    this.district = '',
    this.latitude,
    this.longitude,
    this.locationApproximate = false,
  });

  /// The server's id for this stop. Delivery operations are keyed by it, not by the outlet code,
  /// because one outlet can be visited more than once.
  final String stopId;
  final int sequence;
  final String outletCode;
  final String name;
  final String windowStart;
  final String windowEnd;
  /// Expected quantity from the server's order load. Null when the server does not know it (older
  /// stops). The server calls these "units" and does not say they are cartons.
  final int? units;
  final String unitLabel;
  final String accessNote;
  final String contactNote;
  final String goods;
  final String orderRef;

  /// The server's id of the order this stop delivers. The truck checkout confirms the load by these ids.
  final String orderId;

  /// What the server has recorded for a stop that is already done: its outcome code (DELIVERED, PARTIAL,
  /// FAILED, REFUSED) and the units handed over. Empty and null while the stop is still to do.
  final String outcomeCode;
  final int? deliveredUnits;
  final String district;

  /// Where the outlet is, when the server knows. [locationApproximate] means only the district centre
  /// is known: it must not be used to navigate to the shop (see `StopDirections`).
  final double? latitude;
  final double? longitude;
  final bool locationApproximate;

  /// Reads a stop saved with [toJson]. Anything of the wrong type throws [FormatException], never a
  /// cast error, so a damaged saved route is recognised and ignored.
  factory StopInfo.fromJson(Map<String, Object?> json) => StopInfo(
        stopId: _text(json, 'stopId'),
        sequence: _whole(json, 'sequence'),
        outletCode: _text(json, 'outletCode'),
        name: _text(json, 'name'),
        windowStart: _text(json, 'windowStart'),
        windowEnd: _text(json, 'windowEnd'),
        units: _wholeOrNull(json, 'units'),
        unitLabel: _text(json, 'unitLabel', 'units'),
        accessNote: _text(json, 'accessNote'),
        contactNote: _text(json, 'contactNote'),
        goods: _text(json, 'goods'),
        orderRef: _text(json, 'orderRef'),
        orderId: _text(json, 'orderId'),
        outcomeCode: _text(json, 'outcomeCode'),
        deliveredUnits: _wholeOrNull(json, 'deliveredUnits'),
        district: _text(json, 'district'),
        latitude: _decimalOrNull(json, 'latitude'),
        longitude: _decimalOrNull(json, 'longitude'),
        locationApproximate: _flag(json, 'locationApproximate'),
      );

  Map<String, Object?> toJson() => {
        'stopId': stopId,
        'sequence': sequence,
        'outletCode': outletCode,
        'name': name,
        'windowStart': windowStart,
        'windowEnd': windowEnd,
        if (units != null) 'units': units,
        'unitLabel': unitLabel,
        'accessNote': accessNote,
        'contactNote': contactNote,
        'goods': goods,
        'orderRef': orderRef,
        'orderId': orderId,
        if (outcomeCode.isNotEmpty) 'outcomeCode': outcomeCode,
        if (deliveredUnits != null) 'deliveredUnits': deliveredUnits,
        'district': district,
        if (latitude != null) 'latitude': latitude,
        if (longitude != null) 'longitude': longitude,
        'locationApproximate': locationApproximate,
      };

  String get window => windowStart.isEmpty && windowEnd.isEmpty ? 'No time window' : '$windowStart - $windowEnd';

  /// The outlet as shown to the driver, without a dangling separator when the server sent no
  /// outlet code or name: "OUT108 - Dehiwala", "OUT108 · Dehiwala", "OUT108 Dehiwala".
  String labelWith(String separator) => [outletCode, name].where((part) => part.isNotEmpty).join(separator);

  String get label => labelWith(' - ');

  /// "Stop 2 · Nugegoda", or just "Stop 2" when the server sent no outlet name and the name is only
  /// that same fallback.
  String get sequenceTitle => name == 'Stop $sequence' ? name : 'Stop $sequence · $name';

  /// "12 units", or an honest "quantity not recorded".
  String get unitsText => units == null ? 'Quantity not recorded' : '$units $unitLabel';
}

class TripInfo {
  const TripInfo({
    this.tripId = '',
    this.runId = '',
    required this.vehicleCode,
    this.plate = '',
    required this.tripRef,
    required this.depot,
    required this.window,
    required this.stops,
    this.completedStops = 0,
    this.completedStopIds = const {},
    this.planId = '',
    this.planVersion = 0,
    this.runStatus = '',
  });

  /// The server's trip and delivery-run ids, used when operations are sent.
  final String tripId;
  final String runId;
  final String vehicleCode;
  /// The server has no plate number for a vehicle yet, so this is usually empty.
  final String plate;
  final String tripRef;
  final String depot;
  final String window;
  final List<StopInfo> stops;
  final int completedStops;
  final Set<String> completedStopIds;

  /// What the server needs before a trip can start: the plan the driver must acknowledge, its
  /// current version, and whether the run is already in progress (`prepared`, `in_progress`).
  final String planId;
  final int planVersion;
  final String runStatus;

  bool get started => runStatus == 'in_progress';

  /// Reads a trip saved with [toJson]. Throws [FormatException] when what is stored is not a trip, so a
  /// damaged saved route is ignored instead of crashing the app.
  factory TripInfo.fromJson(Map<String, Object?> json) {
    final tripId = _text(json, 'tripId');
    final stops = json['stops'];
    if (tripId.isEmpty || stops is! List) throw const FormatException('saved trip');
    final doneIds = json['completedStopIds'];
    if (doneIds != null && doneIds is! List) throw const FormatException('completedStopIds is not a list');
    return TripInfo(
      tripId: tripId,
      runId: _text(json, 'runId'),
      vehicleCode: _text(json, 'vehicleCode'),
      plate: _text(json, 'plate'),
      tripRef: _text(json, 'tripRef'),
      depot: _text(json, 'depot'),
      window: _text(json, 'window'),
      stops: [
        for (final stop in stops)
          if (stop is Map<String, Object?>) StopInfo.fromJson(stop) else throw const FormatException('a stop is not an object'),
      ],
      completedStops: _whole(json, 'completedStops'),
      completedStopIds: {
        for (final id in (doneIds as List<Object?>? ?? const []))
          if (id is String) id else throw const FormatException('a completed stop id is not text'),
      },
      planId: _text(json, 'planId'),
      planVersion: _whole(json, 'planVersion'),
      runStatus: _text(json, 'runStatus'),
    );
  }

  Map<String, Object?> toJson() => {
        'tripId': tripId,
        'runId': runId,
        'vehicleCode': vehicleCode,
        'plate': plate,
        'tripRef': tripRef,
        'depot': depot,
        'window': window,
        'stops': [for (final stop in stops) stop.toJson()],
        'completedStops': completedStops,
        'completedStopIds': completedStopIds.toList()..sort(),
        'planId': planId,
        'planVersion': planVersion,
        'runStatus': runStatus,
      };

  TripInfo withRunStatus(String status) => TripInfo(
        tripId: tripId,
        runId: runId,
        vehicleCode: vehicleCode,
        plate: plate,
        tripRef: tripRef,
        depot: depot,
        window: window,
        stops: stops,
        completedStops: completedStops,
        completedStopIds: completedStopIds,
        planId: planId,
        planVersion: planVersion,
        runStatus: status,
      );

  /// "VEH001", or "VEH001 - WP LB-4521" when a plate is known.
  String get vehicleLabel => plate.isEmpty ? vehicleCode : '$vehicleCode - $plate';

  StopInfo? get nextStop => completedStops < stops.length ? stops[completedStops] : null;
}

class DeliveryDraft {
  const DeliveryDraft({
    required this.outcome,
    this.quantity,
    this.notes = '',
    this.reason,
    this.hasPhoto = false,
    this.hasSignature = false,
    this.proofs = const [],
  });

  final DeliveryOutcome outcome;

  /// The server's reason code for a failed or refused delivery (OUTLET_CLOSED, ACCESS_BLOCKED,
  /// RECEIVER_UNAVAILABLE, GOODS_REJECTED, VEHICLE_ISSUE, OTHER). Null means "use the default".
  final String? reason;
  final int? quantity;
  final String notes;
  final bool hasPhoto;
  final bool hasSignature;

  /// The captured files to upload before the outcome. Empty when nothing was captured.
  final List<CapturedProof> proofs;
}

String _text(Map<String, Object?> json, String key, [String fallback = '']) {
  final value = json[key];
  if (value == null) return fallback;
  if (value is String) return value;
  throw FormatException('$key is not text');
}

int _whole(Map<String, Object?> json, String key) => _wholeOrNull(json, key) ?? 0;

/// A whole number. JSON may write one as `3.0`, which is accepted; text or a fraction is not.
int? _wholeOrNull(Map<String, Object?> json, String key) {
  final value = json[key];
  if (value == null) return null;
  if (value is int) return value;
  if (value is double && value == value.truncateToDouble()) return value.toInt();
  throw FormatException('$key is not a whole number');
}

/// A number with or without a fraction. Text is not one.
double? _decimalOrNull(Map<String, Object?> json, String key) {
  final value = json[key];
  if (value == null) return null;
  if (value is num && value.isFinite) return value.toDouble();
  throw FormatException('$key is not a number');
}

bool _flag(Map<String, Object?> json, String key) {
  final value = json[key];
  if (value == null) return false;
  if (value is bool) return value;
  throw FormatException('$key is not true or false');
}

const _weekdays = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'];
const _months = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];

/// "Wed 30 Sep", as shown in the hero header.
String formatHeroDate(DateTime date) => '${_weekdays[date.weekday - 1]} ${date.day} ${_months[date.month - 1]}';
