import 'package:flutter/material.dart';

import '../theme/tokens.dart';

/// Full-width action button used across the app.
///
/// [strong] selects the brighter blue used by the route and stop frames;
/// the sign-in and token-based frames use the standard primary blue.
class AppButton extends StatelessWidget {
  const AppButton({
    super.key,
    required this.label,
    required this.onPressed,
    this.outline = false,
    this.strong = false,
    this.radius = AppRadius.r4,
    this.height = 48,
    this.weight = FontWeight.w500,
    this.trailing,
  });

  final String label;
  final VoidCallback? onPressed;
  final bool outline;
  final bool strong;
  final double radius;
  final double height;
  final FontWeight weight;
  final Widget? trailing;

  @override
  Widget build(BuildContext context) {
    final fill = strong ? AppColors.strongBlue : AppColors.primary;
    final enabled = onPressed != null;
    final background = outline ? Colors.white : (enabled ? fill : AppColors.canvas);
    final foreground = outline ? fill : (enabled ? Colors.white : AppColors.fieldLabel);
    return SizedBox(
      width: double.infinity,
      height: height,
      child: Material(
        color: background,
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(radius),
          side: outline ? BorderSide(color: fill) : BorderSide.none,
        ),
        child: InkWell(
          borderRadius: BorderRadius.circular(radius),
          onTap: onPressed,
          child: Center(
            child: Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                Text(label, style: AppText.of(16, weight, color: foreground, height: 1.45)),
                if (trailing != null) ...[const SizedBox(width: 8), trailing!],
              ],
            ),
          ),
        ),
      ),
    );
  }
}
