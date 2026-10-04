import 'package:flutter/material.dart';

/// Design tokens from the Waypoint UI Kit (Figma "Foundations").
class AppColors {
  static const ink = Color(0xFF232D42);
  static const muted = Color(0xFF6B7893);
  static const canvas = Color(0xFFE9ECEF);
  static const surface = Color(0xFFFFFFFF);
  static const border = Color(0xFFDEE3ED);
  static const primary = Color(0xFF3A57E8);
  static const tint = Color(0xFFEEF1FF);
  static const green = Color(0xFF008B52);
  static const greenBg = Color(0xFFEAF7F0);
  static const amber = Color(0xFFA45C00);
  static const amberBg = Color(0xFFFFF4DE);
  static const red = Color(0xFFC03221);
  static const redBg = Color(0xFFFCEEEB);
  static const cool = Color(0xFF007F91);
  static const coolBg = Color(0xFFE7F6F8);

  // Used by the route / stop frames, which pair the tokens with a stronger blue.
  static const strongBlue = Color(0xFF002BFF);
  static const heroLight = Color(0xFFB4C1FF);
  static const heroDeep = Color(0xFF0035FF);
  static const signInHeroTop = Color(0xFF3A57E8);
  static const signInHeroBottom = Color(0xFF1A3BB8);

  // Sign-in form chrome.
  static const fieldBorder = Color(0xFFDDE1E8);
  static const fieldLabel = Color(0xFF8A92A6);
  static const fieldHint = Color(0xFFADB5BD);

  static const navBar = Color(0xFFECECEC);
  static const navActive = Color(0xFF006AFF);
  static const separator = Color(0xFFC6C6C8);
  static const dangerOutline = Color(0xFFB81C1C);
}

class AppSpace {
  static const double s4 = 4;
  static const double s8 = 8;
  static const double s12 = 12;
  static const double s16 = 16;
  static const double s24 = 24;
  static const double s32 = 32;
}

class AppRadius {
  static const double r4 = 4;
  static const double r6 = 6;
  static const double r8 = 8;
  static const double r12 = 12;
  static const double r24 = 24;
  static const double full = 999;
}

class AppText {
  static const _family = 'Inter';

  static const title = TextStyle(fontFamily: _family, fontSize: 33, fontWeight: FontWeight.w700, height: 1.4, color: AppColors.ink);
  static const subtitle = TextStyle(fontFamily: _family, fontSize: 22, fontWeight: FontWeight.w500, height: 1.4, color: AppColors.ink);
  static const body = TextStyle(fontFamily: _family, fontSize: 16, fontWeight: FontWeight.w400, height: 1.4, color: AppColors.ink);
  static const label = TextStyle(fontFamily: _family, fontSize: 14, fontWeight: FontWeight.w500, height: 1.4, color: AppColors.ink);
  static const caption = TextStyle(fontFamily: _family, fontSize: 13, fontWeight: FontWeight.w400, height: 1.4, color: AppColors.ink);

  static TextStyle of(double size, FontWeight weight, {Color color = AppColors.ink, double height = 1.4}) =>
      TextStyle(fontFamily: _family, fontSize: size, fontWeight: weight, height: height, color: color);
}
