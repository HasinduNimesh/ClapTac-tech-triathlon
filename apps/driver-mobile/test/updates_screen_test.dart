import 'package:flutter_test/flutter_test.dart';
import 'package:waypoint_driver/screens/updates/updates_screen.dart';
import 'package:waypoint_driver/widgets/driver_shell.dart';

import 'helpers/render.dart';

void main() {
  setUpAll(loadAppFonts);

  testWidgets('lists updates and reports taps', (tester) async {
    UpdateItem? opened;
    const item = UpdateItem(id: 'a', title: 'Delivery saved on this phone', detail: 'OUT108 Dehiwala · waiting to upload', time: '08:42');
    await pumpScreen(tester, UpdatesScreen(items: const [item], onOpen: (i) => opened = i));
    expect(find.text('Updates'), findsWidgets);
    expect(find.text('Delivery saved on this phone'), findsOneWidget);
    await tester.tap(find.text('Delivery saved on this phone'));
    expect(opened?.id, 'a');
  });

  testWidgets('shows an empty state', (tester) async {
    await pumpScreen(tester, const UpdatesScreen(items: []));
    expect(find.textContaining('No updates yet'), findsOneWidget);
  });

  testWidgets('selecting another tab is reported', (tester) async {
    DriverTab? tab;
    await pumpScreen(tester, UpdatesScreen(items: const [], onTabSelected: (t) => tab = t));
    await tester.tap(find.text('Summary'));
    expect(tab, DriverTab.summary);
  });
}
