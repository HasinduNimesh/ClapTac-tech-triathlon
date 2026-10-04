import 'package:flutter/material.dart';

import 'tokens.dart';

ThemeData buildAppTheme() {
  final scheme = ColorScheme.fromSeed(seedColor: AppColors.primary).copyWith(
    primary: AppColors.primary,
    onPrimary: Colors.white,
    surface: AppColors.surface,
    onSurface: AppColors.ink,
    error: AppColors.red,
  );
  return ThemeData(
    useMaterial3: true,
    fontFamily: 'Inter',
    colorScheme: scheme,
    scaffoldBackgroundColor: AppColors.surface,
    textTheme: Typography.blackMountainView.apply(fontFamily: 'Inter', bodyColor: AppColors.ink, displayColor: AppColors.ink),
    splashFactory: InkSparkle.splashFactory,
  );
}
