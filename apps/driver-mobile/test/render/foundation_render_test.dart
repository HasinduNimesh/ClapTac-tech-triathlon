import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:waypoint_driver/widgets/app_buttons.dart';
import 'package:waypoint_driver/widgets/driver_shell.dart';
import 'package:waypoint_driver/widgets/note_banner.dart';

import '../helpers/render.dart';

void main() {
  setUpAll(loadAppFonts);

  testWidgets('render: shell with sample content', (tester) async {
    await pumpScreen(
      tester,
      DriverShell(
        dateLabel: 'Wed 30 Sep',
        onTabSelected: (_) {},
        body: const Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            NoteBanner(text: 'Use only when safely parked.', tone: NoteTone.danger),
            SizedBox(height: 16),
            NoteBanner(text: 'Follow the planned stop order.', onDismiss: _noop),
            SizedBox(height: 16),
            AppButton(label: 'View Stop', onPressed: _noop, strong: true, radius: 10),
          ],
        ),
      ),
    );
    await expectLater(find.byType(MaterialApp), matchesGoldenFile('out/foundation_shell.png'));
  }, skip: !renderScreensEnabled);
}

void _noop() {}
