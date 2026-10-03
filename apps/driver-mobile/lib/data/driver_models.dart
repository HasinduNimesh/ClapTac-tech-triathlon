/// View models for the driver UI. They are separate from the sync-layer
/// records in `shared/models.dart` so screens stay easy to build and test.
enum DeliveryOutcome { delivered, partial, failed, refused }

/// Which kind of proof the driver asked to capture.
enum ProofKind { photo, signature }

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
    required this.cartons,
    required this.accessNote,
    required this.contactNote,
    required this.goods,
    this.orderRef = '',
  });

  /// The server's id for this stop. Delivery operations are keyed by it, not by the outlet code,
  /// because one outlet can be visited more than once.
  final String stopId;
  final int sequence;
  final String outletCode;
  final String name;
  final String windowStart;
  final String windowEnd;
  final int cartons;
  final String accessNote;
  final String contactNote;
  final String goods;
  final String orderRef;

  String get window => '$windowStart - $windowEnd';
}

class TripInfo {
  const TripInfo({
    this.tripId = '',
    this.runId = '',
    required this.vehicleCode,
    required this.plate,
    required this.tripRef,
    required this.depot,
    required this.window,
    required this.stops,
    this.completedStops = 0,
  });

  /// The server's trip and delivery-run ids, used when operations are sent.
  final String tripId;
  final String runId;
  final String vehicleCode;
  final String plate;
  final String tripRef;
  final String depot;
  final String window;
  final List<StopInfo> stops;
  final int completedStops;

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
  });

  final DeliveryOutcome outcome;

  /// The server's reason code for a failed or refused delivery (OUTLET_CLOSED, ACCESS_BLOCKED,
  /// RECEIVER_UNAVAILABLE, GOODS_REJECTED, VEHICLE_ISSUE, OTHER). Null means "use the default".
  final String? reason;
  final int? quantity;
  final String notes;
  final bool hasPhoto;
  final bool hasSignature;
}

const _weekdays = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'];
const _months = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];

/// "Wed 30 Sep", as shown in the hero header.
String formatHeroDate(DateTime date) => '${_weekdays[date.weekday - 1]} ${date.day} ${_months[date.month - 1]}';
