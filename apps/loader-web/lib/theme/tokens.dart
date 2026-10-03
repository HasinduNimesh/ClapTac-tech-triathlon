import 'package:flutter/material.dart';

/// Waypoint UI Kit tokens (Figma "Waypoint UI Kit / Foundations"), shared
/// with the web dispatcher and store-manager workspaces.
class Wp {
  static const canvas = Color(0xFFE9ECEF);
  static const surface = Color(0xFFFFFFFF);
  static const ink = Color(0xFF232D42);
  static const muted = Color(0xFF6B7893);
  static const primary = Color(0xFF3A57E8);
  static const tint = Color(0xFFEEF1FF);
  static const border = Color(0xFFDEE3ED);
  static const green = Color(0xFF008B52);
  static const greenBg = Color(0xFFEAF7F0);
  static const amber = Color(0xFFA45C00);
  static const amberBg = Color(0xFFFFF4DE);
  static const red = Color(0xFFC03221);
  static const redBg = Color(0xFFFCEEEB);
  static const cool = Color(0xFF007F91);
  static const coolBg = Color(0xFFE7F6F8);
  static const heroStart = Color(0xFF3A57E8);
  static const heroEnd = Color(0xFF1E3BB3);

  static const space4 = 4.0;
  static const space8 = 8.0;
  static const space12 = 12.0;
  static const space16 = 16.0;
  static const space24 = 24.0;
  static const radius = 12.0;
  static const radiusSm = 8.0;

  /// Tablet layout from this width (the dock tablet is 1194 × 834 in Figma).
  static const tabletBreakpoint = 900.0;
}

enum Tone { primary, green, amber, red, cool, muted }

extension ToneColors on Tone {
  Color get fg => switch (this) {
        Tone.primary => Wp.primary,
        Tone.green => Wp.green,
        Tone.amber => Wp.amber,
        Tone.red => Wp.red,
        Tone.cool => Wp.cool,
        Tone.muted => Wp.muted,
      };
  Color get bg => switch (this) {
        Tone.primary => Wp.tint,
        Tone.green => Wp.greenBg,
        Tone.amber => Wp.amberBg,
        Tone.red => Wp.redBg,
        Tone.cool => Wp.coolBg,
        Tone.muted => Wp.canvas,
      };
}

ThemeData waypointTheme() {
  final base = ThemeData(
    useMaterial3: true,
    colorScheme: ColorScheme.fromSeed(seedColor: Wp.primary, primary: Wp.primary, surface: Wp.surface),
    scaffoldBackgroundColor: Wp.canvas,
    fontFamily: 'Inter',
  );
  return base.copyWith(
    textTheme: base.textTheme.apply(bodyColor: Wp.ink, displayColor: Wp.ink),
    inputDecorationTheme: InputDecorationTheme(
      filled: true,
      fillColor: Wp.surface,
      contentPadding: const EdgeInsets.symmetric(horizontal: 14, vertical: 14),
      border: OutlineInputBorder(borderRadius: BorderRadius.circular(Wp.radiusSm), borderSide: const BorderSide(color: Wp.border)),
      enabledBorder: OutlineInputBorder(borderRadius: BorderRadius.circular(Wp.radiusSm), borderSide: const BorderSide(color: Wp.border)),
      focusedBorder: OutlineInputBorder(borderRadius: BorderRadius.circular(Wp.radiusSm), borderSide: const BorderSide(color: Wp.primary, width: 2)),
      hintStyle: const TextStyle(color: Wp.muted),
    ),
    filledButtonTheme: FilledButtonThemeData(
      style: FilledButton.styleFrom(
        backgroundColor: Wp.primary,
        minimumSize: const Size(64, 48),
        textStyle: const TextStyle(fontSize: 16, fontWeight: FontWeight.w600),
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(Wp.radiusSm)),
      ),
    ),
    outlinedButtonTheme: OutlinedButtonThemeData(
      style: OutlinedButton.styleFrom(
        foregroundColor: Wp.primary,
        minimumSize: const Size(64, 48),
        side: const BorderSide(color: Wp.border),
        textStyle: const TextStyle(fontSize: 16, fontWeight: FontWeight.w600),
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(Wp.radiusSm)),
      ),
    ),
    navigationBarTheme: const NavigationBarThemeData(backgroundColor: Wp.surface, indicatorColor: Wp.tint),
  );
}
