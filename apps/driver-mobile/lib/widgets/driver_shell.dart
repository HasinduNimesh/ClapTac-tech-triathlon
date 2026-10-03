import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../data/driver_models.dart';
import '../theme/assets.dart';
import '../theme/tokens.dart';
import 'hero_header.dart';
import 'svg_icon.dart';

enum DriverTab { route, updates, summary }

/// Hero + rounded white sheet + bottom navigation used by every in-app screen.
class DriverShell extends StatelessWidget {
  const DriverShell({
    super.key,
    required this.body,
    this.tab = DriverTab.route,
    this.onTabSelected,
    this.bottomAction,
    this.showNav = true,
    this.dateLabel,
    this.heroTrailing,
    this.bodyPadding = const EdgeInsets.fromLTRB(AppSpace.s24, AppSpace.s24, AppSpace.s24, AppSpace.s24),
  });

  final Widget body;
  final DriverTab tab;
  final ValueChanged<DriverTab>? onTabSelected;

  /// Pinned above the navigation bar (e.g. "Save delivery").
  final Widget? bottomAction;
  final bool showNav;
  final String? dateLabel;

  /// Overlays the hero, aligned to the right (e.g. "Report a problem").
  final Widget? heroTrailing;
  final EdgeInsets bodyPadding;

  @override
  Widget build(BuildContext context) {
    final inset = MediaQuery.paddingOf(context).top;
    final sheetTop = (inset + 71).clamp(93.0, 140.0);
    final date = dateLabel ?? formatHeroDate(DateTime.now());

    return AnnotatedRegion<SystemUiOverlayStyle>(
      value: SystemUiOverlayStyle.light,
      child: Scaffold(
        backgroundColor: AppColors.surface,
        body: Stack(
          children: [
            Positioned(
              top: 0,
              left: 0,
              right: 0,
              child: HeroBackground(
                height: 260,
                child: Stack(
                  children: [
                    Positioned(
                      left: 22,
                      top: sheetTop - 61,
                      width: 76,
                      height: 57,
                      child: Image.asset(AppAssets.logoMark, fit: BoxFit.cover, semanticLabel: 'Waypoint'),
                    ),
                    Positioned(
                      right: 20,
                      top: sheetTop - 53,
                      child: Text(date, style: AppText.of(18, FontWeight.w500, color: Colors.white, height: 1.4)),
                    ),
                  ],
                ),
              ),
            ),
            Positioned.fill(
              top: sheetTop,
              child: Column(
                children: [
                  Expanded(
                    child: Container(
                      width: double.infinity,
                      decoration: const BoxDecoration(
                        color: AppColors.surface,
                        borderRadius: BorderRadius.vertical(top: Radius.circular(AppRadius.r24)),
                      ),
                      clipBehavior: Clip.antiAlias,
                      child: Column(
                        children: [
                          Expanded(child: SingleChildScrollView(padding: bodyPadding, child: body)),
                          if (bottomAction != null)
                            Padding(
                              padding: const EdgeInsets.fromLTRB(AppSpace.s24, AppSpace.s8, AppSpace.s24, AppSpace.s12),
                              child: bottomAction,
                            ),
                        ],
                      ),
                    ),
                  ),
                  if (showNav) DriverBottomNav(selected: tab, onSelected: onTabSelected),
                ],
              ),
            ),
            if (heroTrailing != null) Positioned(right: 24, top: sheetTop - 7, child: heroTrailing!),
          ],
        ),
      ),
    );
  }
}

class DriverBottomNav extends StatelessWidget {
  const DriverBottomNav({super.key, required this.selected, this.onSelected});

  final DriverTab selected;
  final ValueChanged<DriverTab>? onSelected;

  @override
  Widget build(BuildContext context) {
    final bottomInset = MediaQuery.paddingOf(context).bottom;
    return Container(
      color: AppColors.navBar,
      padding: EdgeInsets.only(bottom: bottomInset),
      height: 74 + bottomInset,
      child: Row(
        children: [
          _NavItem(asset: AppAssets.navRoute, label: 'Route', tab: DriverTab.route, selected: selected, onSelected: onSelected),
          _NavItem(asset: AppAssets.navUpdates, label: 'Updates', tab: DriverTab.updates, selected: selected, onSelected: onSelected),
          _NavItem(asset: AppAssets.navSummary, label: 'Summary', tab: DriverTab.summary, selected: selected, onSelected: onSelected),
        ],
      ),
    );
  }
}

class _NavItem extends StatelessWidget {
  const _NavItem({required this.asset, required this.label, required this.tab, required this.selected, required this.onSelected});

  final String asset;
  final String label;
  final DriverTab tab;
  final DriverTab selected;
  final ValueChanged<DriverTab>? onSelected;

  @override
  Widget build(BuildContext context) {
    final active = tab == selected;
    final color = active ? AppColors.navActive : Colors.black;
    return Expanded(
      child: Semantics(
        button: true,
        selected: active,
        label: label,
        excludeSemantics: true,
        child: InkWell(
          onTap: onSelected == null ? null : () => onSelected!(tab),
          child: Column(
            mainAxisAlignment: MainAxisAlignment.center,
            children: [
              SvgIcon(asset, size: 24, color: color),
              const SizedBox(height: 4),
              Text(label, style: AppText.of(10, FontWeight.w400, color: color, height: 1.2)),
            ],
          ),
        ),
      ),
    );
  }
}
