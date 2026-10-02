import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:waypoint_driver/theme/app_theme.dart';

const _pngAssets = [
  'assets/images/logo_mark.png',
  'assets/images/logo_wordmark.png',
  'assets/images/truck.png',
  'assets/images/map_pin.png',
  'assets/images/outlet_building.png',
  'assets/images/carton_box.png',
  'assets/images/camera.png',
  'assets/images/signature.png',
];

/// Widget tests use a placeholder font unless the real one is loaded.
Future<void> loadAppFonts() async {
  final loader = FontLoader('Inter');
  for (final weight in ['Regular', 'Medium', 'SemiBold', 'Bold']) {
    loader.addFont(rootBundle.load('assets/fonts/Inter-$weight.ttf'));
  }
  await loader.load();
}

/// Pumps [screen] in a phone-sized viewport with real fonts and images.
Future<void> pumpScreen(WidgetTester tester, Widget screen, {Size size = const Size(390, 844)}) async {
  tester.view.devicePixelRatio = 1.0;
  tester.view.physicalSize = size;
  addTearDown(tester.view.reset);

  await tester.pumpWidget(MaterialApp(debugShowCheckedModeBanner: false, theme: buildAppTheme(), home: screen));
  await tester.runAsync(() async {
    final context = tester.element(find.byType(MaterialApp).first);
    for (final path in _pngAssets) {
      await precacheImage(AssetImage(path), context);
    }
  });
  await tester.pumpAndSettle();
}

/// Set RENDER_SCREENS=1 to write PNGs of the screens (used for visual review).
bool get renderScreensEnabled => const bool.fromEnvironment('RENDER_SCREENS') || _envFlag;
bool get _envFlag => const String.fromEnvironment('RENDER_SCREENS').isNotEmpty;
