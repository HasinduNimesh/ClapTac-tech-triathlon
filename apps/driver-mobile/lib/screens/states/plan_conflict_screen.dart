import 'package:flutter/material.dart';

import '../../theme/tokens.dart';
import '../../widgets/driver_shell.dart';
import 'states_widgets.dart';

class PlanConflictInfo {
  const PlanConflictInfo({
    required this.savedPlan,
    required this.savedDetail,
    required this.newPlan,
    required this.newDetail,
  });

  final String savedPlan;
  final String savedDetail;
  final String newPlan;
  final String newDetail;
}

const samplePlanConflict = PlanConflictInfo(
  savedPlan: 'PLAN V3',
  savedDetail: 'Stop 3 · 96 of 120 received · 06:32',
  newPlan: 'PLAN V4',
  newDetail: 'Stop sequence changed · shortfall decision updated',
);

/// Shown when a dispatcher plan arrives while a saved offline record exists.
class PlanConflictScreen extends StatelessWidget {
  const PlanConflictScreen({
    super.key,
    required this.info,
    required this.onSendForReview,
    required this.onReturnToRoute,
    this.onTabSelected,
  });

  final PlanConflictInfo info;
  final VoidCallback onSendForReview;
  final VoidCallback onReturnToRoute;
  final ValueChanged<DriverTab>? onTabSelected;

  @override
  Widget build(BuildContext context) {
    return DriverShell(
      tab: DriverTab.route,
      onTabSelected: onTabSelected,
      bodyPadding: const EdgeInsets.fromLTRB(14, 14, 14, AppSpace.s24),
      body: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Padding(
            padding: const EdgeInsets.symmetric(horizontal: 7),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Semantics(
                  header: true,
                  child: figmaText('Plan update needs review', size: 23, weight: FontWeight.w700, color: AppColors.ink, lineHeight: 31),
                ),
                figmaText('Your saved stop was recorded against Plan v3', color: AppColors.muted, lineHeight: 17.5),
              ],
            ),
          ),
          const SizedBox(height: 10),
          Semantics(
            container: true,
            child: Container(
              height: 94,
              decoration: BoxDecoration(
                color: AppColors.amberBg,
                borderRadius: BorderRadius.circular(AppRadius.r8),
                border: Border.all(color: AppColors.border),
              ),
              padding: const EdgeInsets.fromLTRB(15, 15, 15, 0),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  figmaText('Offline record is safe', size: 16, weight: FontWeight.w700, color: AppColors.amber, lineHeight: 21.6),
                  const SizedBox(height: 7),
                  figmaText('Do not overwrite the driver’s saved event.', color: AppColors.ink, lineHeight: 17.5),
                ],
              ),
            ),
          ),
          const SizedBox(height: 18),
          _PlanComparison(info: info),
          const SizedBox(height: 19),
          Container(
            height: 94,
            decoration: BoxDecoration(color: AppColors.tint, borderRadius: BorderRadius.circular(AppRadius.r8)),
            padding: const EdgeInsets.fromLTRB(16, 16, 16, 0),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                figmaText('What happens next', size: 15, weight: FontWeight.w700, color: AppColors.primary, lineHeight: 20),
                const SizedBox(height: 7),
                figmaText(
                  'Keep the saved proof. Send both versions to dispatch so they can reconcile the stop.',
                  color: AppColors.ink,
                  lineHeight: 17.5,
                ),
              ],
            ),
          ),
          const SizedBox(height: 22),
          _PlanButton(label: 'Send for dispatcher review', onPressed: onSendForReview),
          const SizedBox(height: 8),
          _PlanButton(label: 'Return to route', onPressed: onReturnToRoute, secondary: true),
          const SizedBox(height: 1),
          Padding(
            padding: const EdgeInsets.only(left: 21),
            child: figmaText('Local save ≠ server confirmation · pending until accepted', size: 11, color: AppColors.muted, lineHeight: 15),
          ),
        ],
      ),
    );
  }
}

class _PlanComparison extends StatelessWidget {
  const _PlanComparison({required this.info});

  final PlanConflictInfo info;

  @override
  Widget build(BuildContext context) {
    Widget badge(String text, Color bg, Color fg, double top) => Positioned(
          left: 15,
          top: top,
          child: Container(
            height: 26,
            width: 86,
            alignment: Alignment.centerLeft,
            padding: const EdgeInsets.only(left: 10),
            decoration: BoxDecoration(color: bg, borderRadius: BorderRadius.circular(13)),
            child: figmaText(text, size: 12, weight: FontWeight.w500, color: fg, lineHeight: 16),
          ),
        );
    return Semantics(
      container: true,
      child: Container(
        height: 245,
        decoration: BoxDecoration(
          color: AppColors.surface,
          borderRadius: BorderRadius.circular(AppRadius.r8),
          border: Border.all(color: AppColors.border),
        ),
        child: Stack(
          children: [
            Positioned(left: 15, top: 21, child: figmaText('Saved on this phone', size: 16, weight: FontWeight.w700, color: AppColors.ink, lineHeight: 21.6)),
            badge(info.savedPlan, AppColors.coolBg, AppColors.cool, 49),
            Positioned(left: 15, top: 85, child: figmaText(info.savedDetail, weight: FontWeight.w500, color: AppColors.ink, lineHeight: 17.5)),
            Positioned(left: 15, right: 15, top: 117, child: Container(height: 1, color: AppColors.border)),
            Positioned(left: 15, top: 136, child: figmaText('New dispatcher plan', size: 16, weight: FontWeight.w700, color: AppColors.ink, lineHeight: 21.6)),
            badge(info.newPlan, AppColors.amberBg, AppColors.amber, 164),
            Positioned(left: 15, top: 200, child: figmaText(info.newDetail, size: 12, color: AppColors.muted, lineHeight: 16)),
          ],
        ),
      ),
    );
  }
}

/// 40px action button from the Waypoint UI Kit (6px radius, 14px medium).
class _PlanButton extends StatelessWidget {
  const _PlanButton({required this.label, required this.onPressed, this.secondary = false});

  final String label;
  final VoidCallback onPressed;
  final bool secondary;

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      height: 44,
      child: Material(
        color: secondary ? AppColors.surface : AppColors.primary,
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(AppRadius.r6),
          side: secondary ? const BorderSide(color: AppColors.border) : BorderSide.none,
        ),
        child: InkWell(
          borderRadius: BorderRadius.circular(AppRadius.r6),
          onTap: onPressed,
          child: Center(
            child: figmaText(label, size: 14, weight: FontWeight.w500, color: secondary ? AppColors.primary : Colors.white, lineHeight: 17),
          ),
        ),
      ),
    );
  }
}
