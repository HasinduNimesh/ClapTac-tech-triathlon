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
    this.units,
    this.unitLabel = 'units',
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
  /// Expected quantity from the server's order load. Null when the server does not know it (older
  /// stops). The server calls these "units" and does not say they are cartons.
  final int? units;
  final String unitLabel;
  final String accessNote;
  final String contactNote;
  final String goods;
  final String orderRef;

  String get window => windowStart.isEmpty && windowEnd.isEmpty ? 'No time window' : '$windowStart - $windowEnd';

  /// The outlet as shown to the driver, without a dangling separator when the server sent no
  /// outlet code or name: "OUT108 - Dehiwala", "OUT108 · Dehiwala", "OUT108 Dehiwala".
  String labelWith(String separator) => [outletCode, name].where((part) => part.isNotEmpty).join(separator);

  String get label => labelWith(' - ');

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
