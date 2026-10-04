import 'package:flutter/material.dart';

import '../theme/assets.dart';
import '../theme/tokens.dart';
import 'svg_icon.dart';

enum HeroStyle { app, signIn }

/// Gradient hero with the three translucent circles from the Figma frames.
class HeroBackground extends StatelessWidget {
  const HeroBackground({super.key, required this.height, this.style = HeroStyle.app, this.child});

  final double height;
  final HeroStyle style;
  final Widget? child;

  @override
  Widget build(BuildContext context) {
    final colors = style == HeroStyle.app
        ? const [AppColors.heroLight, AppColors.heroDeep]
        : const [AppColors.signInHeroTop, AppColors.signInHeroBottom];
    return SizedBox(
      height: height,
      width: double.infinity,
      child: ClipRect(
        child: DecoratedBox(
          decoration: BoxDecoration(
            gradient: LinearGradient(
              begin: const Alignment(-0.45, -1),
              end: const Alignment(0.45, 1),
              colors: colors,
              stops: const [0.14286, 0.85714],
            ),
          ),
          child: Stack(
            clipBehavior: Clip.hardEdge,
            children: [
              const Positioned(left: 90, top: -150, child: SvgIcon(AppAssets.heroCircle1, size: 520)),
              const Positioned(left: 160, top: -80, child: SvgIcon(AppAssets.heroCircle2, size: 380)),
              const Positioned(left: 230, top: -10, child: SvgIcon(AppAssets.heroCircle3, size: 240)),
              if (child != null) Positioned.fill(child: child!),
            ],
          ),
        ),
      ),
    );
  }
}
