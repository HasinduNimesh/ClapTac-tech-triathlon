import 'package:flutter/material.dart';

import '../../theme/tokens.dart';

/// Colors that appear only in the route and sheet frames.
class RouteColors {
  static const sheetInk = Color(0xFF121726);
  static const sheetMuted = Color(0xFF636B7A);
  static const sheetBorder = Color(0xFFDEE3EB);
  static const checkGreen = Color(0xFF12783B);
  static const warnOrange = Color(0xFFA65208);
  static const noteBlueBg = Color(0xFFEDF2FF);
  static const nextStopLabel = Color(0xFF0800FF);
  static const scrim = Color.fromRGBO(13, 18, 31, 0.45);

  // The frames draw these cards at 86% opacity over white.
  static const vehicleCardFill = Color.fromRGBO(49, 17, 29, 0.069);
  static const vehicleCardBorder = Color.fromRGBO(150, 125, 0, 0.86);
  static const nextStopFill = Color.fromRGBO(0, 119, 255, 0.163);
  static const nextStopBorder = Color.fromRGBO(0, 81, 255, 0.86);
  static const plainCardBorder = Color.fromRGBO(198, 198, 200, 0.86);
}

/// Primary / secondary buttons used by the two bottom sheets and safe-stop
/// screen (44px tall, 6px radius, medium weight).
class SheetButton extends StatelessWidget {
  const SheetButton({
    super.key,
    required this.label,
    required this.onPressed,
    this.primary = false,
    this.fontSize = 15,
    this.verticalPadding = 11,
    this.secondaryTextColor = RouteColors.sheetInk,
    this.secondaryBorderColor = RouteColors.sheetBorder,
  });

  final String label;
  final VoidCallback? onPressed;
  final bool primary;
  final double fontSize;
  final double verticalPadding;
  final Color secondaryTextColor;
  final Color secondaryBorderColor;

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      width: double.infinity,
      child: Material(
        color: primary ? AppColors.primary : Colors.white,
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(AppRadius.r6),
          side: primary
              ? BorderSide.none
              : BorderSide(color: secondaryBorderColor),
        ),
        child: InkWell(
          borderRadius: BorderRadius.circular(AppRadius.r6),
          onTap: onPressed,
          child: ConstrainedBox(
            constraints: const BoxConstraints(minHeight: 44),
            child: Padding(
              padding: EdgeInsets.symmetric(
                  horizontal: 16, vertical: verticalPadding),
              child: Center(
                child: Text(
                  label,
                  textAlign: TextAlign.center,
                  style: AppText.of(fontSize, FontWeight.w500,
                      color: primary ? Colors.white : secondaryTextColor),
                ),
              ),
            ),
          ),
        ),
      ),
    );
  }
}

/// Floating white card used for both modal sheets, sitting 16px from the edges.
class FloatingSheetCard extends StatelessWidget {
  const FloatingSheetCard({super.key, required this.child});

  final Widget child;

  @override
  Widget build(BuildContext context) {
    final viewInsets = MediaQuery.viewInsetsOf(context).bottom;
    return SafeArea(
      child: Padding(
        padding: EdgeInsets.fromLTRB(16, 16, 16, 16 + viewInsets),
        child: Align(
          alignment: Alignment.bottomCenter,
          child: SingleChildScrollView(
            child: Container(
              width: double.infinity,
              padding: const EdgeInsets.all(20),
              decoration: BoxDecoration(
                color: Colors.white,
                borderRadius: BorderRadius.circular(16),
                boxShadow: const [
                  BoxShadow(
                      color: Color(0x2E000000),
                      blurRadius: 32,
                      offset: Offset(0, 8))
                ],
              ),
              child: child,
            ),
          ),
        ),
      ),
    );
  }
}
