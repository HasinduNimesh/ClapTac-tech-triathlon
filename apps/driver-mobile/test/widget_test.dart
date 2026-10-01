import 'package:flutter_test/flutter_test.dart';
import 'package:waypoint_driver/main.dart';
import 'package:waypoint_driver/offline/local_database.dart';
import 'package:waypoint_driver/sync/sync.dart';

void main() {
  testWidgets('boots offline shell', (tester) async {
    await tester.pumpWidget(WaypointDriverApp(
      database: InMemoryLocalDatabase(),
      queue: InMemorySyncQueue(),
    ));
    expect(find.textContaining('Offline-first'), findsOneWidget);
  });
}
