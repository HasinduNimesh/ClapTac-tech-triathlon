import 'dart:convert';

import 'package:sqflite/sqflite.dart';

import 'operations.dart';
import 'sync.dart';

/// SQLite keeps an operation ID and its exact request body across app restarts.
/// Entries belong to one Waypoint user, so another sign-in cannot send them.
class SqliteSyncQueue implements SyncQueue {
  SqliteSyncQueue({Future<Database> Function()? open}) : _open = open ?? _openDefault;

  final Future<Database> Function() _open;
  Future<Database>? _database;
  String? _owner;

  static Future<Database> _openDefault() async {
    final root = await getDatabasesPath();
    return openAt('$root/waypoint_driver_sync.db');
  }

  /// Opens the same schema through an injected factory for desktop tests.
  static Future<Database> openAt(String path, {DatabaseFactory? factory}) async {
    return (factory ?? databaseFactory).openDatabase(path, options: OpenDatabaseOptions(version: 1, onCreate: (db, _) async {
      await db.execute('''
        CREATE TABLE operations (
          sequence INTEGER PRIMARY KEY AUTOINCREMENT,
          owner_id TEXT NOT NULL,
          operation_id TEXT NOT NULL,
          event_json TEXT NOT NULL,
          state TEXT NOT NULL DEFAULT 'pending',
          failure TEXT NOT NULL DEFAULT '',
          created_at TEXT NOT NULL,
          UNIQUE(owner_id, operation_id)
        )
      ''');
      await db.execute('CREATE INDEX operations_owner_order ON operations(owner_id, sequence)');
    }));
  }

  Future<Database> get db => _database ??= _open();

  Future<void> useOwner(String? userId) async {
    _owner = userId?.isNotEmpty == true ? userId : null;
  }

  String get owner {
    final value = _owner;
    if (value == null) throw StateError('Sign in before using the delivery queue');
    return value;
  }

  @override
  Future<void> enqueue(SyncEvent event) async {
    final currentOwner = owner;
    final database = await db;
    await database.insert('operations', {
      'owner_id': currentOwner,
      'operation_id': event.idempotencyKey,
      'event_json': jsonEncode({
        'eventId': event.eventId,
        'idempotencyKey': event.idempotencyKey,
        'action': event.action,
        'resourceType': event.resourceType,
        'resourceId': event.resourceId,
        'payload': event.payload,
        'occurredAt': event.occurredAt?.toUtc().toIso8601String(),
      }),
      'created_at': DateTime.now().toUtc().toIso8601String(),
    }, conflictAlgorithm: ConflictAlgorithm.ignore);
  }

  @override
  Future<List<SyncEvent>> pending() async {
    final rows = await (await db).query('operations',
        columns: ['event_json'], where: 'owner_id = ? AND state != ?', whereArgs: [owner, 'synced'], orderBy: 'sequence ASC');
    return [for (final row in rows) _event(row['event_json'] as String)];
  }

  /// The oldest unsynced item. A blocked result stays at the front until the
  /// driver or dispatcher resolves it; later actions must not overtake it.
  Future<QueuedOperation?> first() async {
    final rows = await (await db).query('operations',
        columns: ['event_json', 'state', 'failure'],
        where: 'owner_id = ? AND state != ?',
        whereArgs: [owner, 'synced'],
        orderBy: 'sequence ASC',
        limit: 1);
    if (rows.isEmpty) return null;
    final row = rows.single;
    return QueuedOperation(_event(row['event_json'] as String), row['state'] as String, row['failure'] as String);
  }

  /// DR-6: status first, photos after. The item to send next: a blocked item at the front still stops
  /// everything, but otherwise the oldest status update (arrival, a failed or refused outcome) that is
  /// not waiting on an unsent proof goes ahead of photo and signature uploads, so the dispatcher sees
  /// what happened at a stop before a large photo has finished uploading on a weak signal.
  Future<QueuedOperation?> next() async {
    final rows = await (await db).query('operations',
        columns: ['event_json', 'state', 'failure'],
        where: 'owner_id = ? AND state != ?',
        whereArgs: [owner, 'synced'],
        orderBy: 'sequence ASC');
    if (rows.isEmpty) return null;
    final items = [for (final row in rows) QueuedOperation(_event(row['event_json'] as String), row['state'] as String, row['failure'] as String)];
    return items[statusFirstIndex(items)];
  }

  @override
  Future<void> markSynced(String idempotencyKey) => _setState(idempotencyKey, 'synced', '');

  Future<void> markBlocked(String idempotencyKey, String reason) => _setState(idempotencyKey, 'blocked', reason);

  Future<void> _setState(String operationId, String state, String failure) async {
    await (await db).update('operations', {'state': state, 'failure': failure},
        where: 'owner_id = ? AND operation_id = ?', whereArgs: [owner, operationId]);
  }

  static SyncEvent _event(String encoded) {
    final json = (jsonDecode(encoded) as Map).cast<String, Object?>();
    final occurred = json['occurredAt'] as String?;
    return SyncEvent(
      eventId: json['eventId'] as String,
      idempotencyKey: json['idempotencyKey'] as String,
      action: json['action'] as String,
      resourceType: json['resourceType'] as String,
      resourceId: json['resourceId'] as String,
      payload: (json['payload'] as Map).cast<String, Object?>(),
      occurredAt: occurred == null ? null : DateTime.parse(occurred),
    );
  }
}

/// Which of the unsent items (oldest first) [SqliteSyncQueue.next] sends. Proof uploads give way to a
/// later status update unless that update depends on an unsent proof; finishing the route never
/// overtakes anything; a blocked item at the front is returned so it keeps everything behind it.
int statusFirstIndex(List<QueuedOperation> items) {
  if (items.isEmpty || items.first.state == 'blocked') return 0;
  if (items.first.event.payload['type'] != OperationType.proofUpload) return 0;
  final unsent = {for (final item in items) item.event.idempotencyKey};
  for (var i = 1; i < items.length; i++) {
    final operation = items[i].event.payload;
    final type = operation['type'];
    if (items[i].state == 'blocked') break;
    if (type == OperationType.proofUpload) continue;
    if (type == OperationType.routeCompleted) break;
    final dependsOn = operation['dependsOnOperationId'] as String?;
    if (dependsOn != null && dependsOn.isNotEmpty && unsent.contains(dependsOn)) continue;
    return i;
  }
  return 0;
}

class QueuedOperation {
  const QueuedOperation(this.event, this.state, this.failure);

  final SyncEvent event;
  final String state;
  final String failure;
}
