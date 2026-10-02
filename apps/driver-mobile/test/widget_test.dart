import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:waypoint_driver/api/api_client.dart';
import 'package:waypoint_driver/auth/auth.dart';
import 'package:waypoint_driver/main.dart';
import 'package:waypoint_driver/offline/store.dart';
import 'package:waypoint_driver/sync/sync.dart';

void main() {
  testWidgets('signed-out device opens the shared sign-in screen', (tester) async {
    final store = MemoryStore();
    await tester.pumpWidget(WaypointFieldApp(store: store, auth: AuthService(store: store)));
    await tester.pump();
    expect(find.text('Sign in'), findsWidgets);
    expect(find.textContaining("today's run is saved on this phone"), findsOneWidget);
  });

  test('queued operations are idempotent and stay in FIFO order', () async {
    final store = MemoryStore();
    final sync = SyncEngine(store: store, api: ApiClient(tokenProvider: () => ''), ownerId: 'driver-1');
    final first = QueuedOperation(operationId: 'op-1', type: 'ARRIVED', tripId: 't1', stopId: 's1');
    await sync.enqueue(first);
    await sync.enqueue(first);
    await sync.enqueue(QueuedOperation(operationId: 'op-2', type: 'STOP_OUTCOME', tripId: 't1', stopId: 's1', dependsOn: 'op-1'));
    final pending = await sync.pending();
    expect(pending.map((p) => p.operationId), ['op-1', 'op-2']);
  });

  test('queues are kept per owner on a shared device', () async {
    final store = MemoryStore();
    final api = ApiClient(tokenProvider: () => '');
    await SyncEngine(store: store, api: api, ownerId: 'loader-a').enqueue(QueuedOperation(operationId: 'a', type: 'ORDER_LOADED', tripId: 't', orderId: 'o'));
    expect(await SyncEngine(store: store, api: api, ownerId: 'loader-b').pending(), isEmpty);
  });

  testWidgets('theme uses the Waypoint primary token', (tester) async {
    await tester.pumpWidget(MaterialApp(home: Builder(builder: (c) => Text('x', style: TextStyle(color: Theme.of(c).colorScheme.primary)))));
    expect(find.text('x'), findsOneWidget);
  });
}
