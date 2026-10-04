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

const _weekdays = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'];
const _months = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];

/// "Wed 30 Sep", as shown in the hero header.
String formatHeroDate(DateTime date) => '${_weekdays[date.weekday - 1]} ${date.day} ${_months[date.month - 1]}';
