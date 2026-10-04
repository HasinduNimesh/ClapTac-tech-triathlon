import 'package:flutter/material.dart';

import '../../data/driver_models.dart';
import '../../theme/tokens.dart';
import '../../widgets/driver_shell.dart';
import 'route_widgets.dart';

/// Arrival guidance shown before the driver starts a stop.
class SafeStopScreen extends StatelessWidget {
  const SafeStopScreen({
    super.key,
    required this.trip,
    required this.stop,
    required this.onStoppedSafely,
    this.onOpenMaps,
    this.onTabSelected,
    this.earlyMinutes = 12,
    this.earliestAccess,
    this.planVersion,
  });

  final TripInfo trip;
  final StopInfo stop;
  final VoidCallback onStoppedSafely;
  final VoidCallback? onOpenMaps;
  final ValueChanged<DriverTab>? onTabSelected;

  /// Minutes the driver is ahead of the receiving window; 0 hides the wait guidance.
  final int earlyMinutes;

  /// Earliest time the outlet allows access (e.g. "05:45"), when known.
  final String? earliestAccess;

  /// Plan version shown next to the trip reference (e.g. "v4"), when known.
  final String? planVersion;

  @override
  Widget build(BuildContext context) {
    final subtitle = planVersion == null
        ? 'Trip ${trip.tripRef}'
        : 'Trip ${trip.tripRef} · Plan $planVersion';
    final waiting = earlyMinutes > 0;

    return DriverShell(
      tab: DriverTab.route,
      onTabSelected: onTabSelected,
      bodyPadding: const EdgeInsets.fromLTRB(14, 23, 14, AppSpace.s24),
      body: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Padding(
            padding: const EdgeInsets.symmetric(horizontal: 2),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Semantics(
                    header: true,
                    child: Text('Stop ${stop.sequence} · ${stop.name}',
                        style: AppText.of(23, FontWeight.w700, height: 1.35))),
                Text(subtitle,
                    style: AppText.of(13, FontWeight.w400,
                        color: AppColors.muted, height: 1.35)),
              ],
            ),
          ),
          const SizedBox(height: 8),
          _Card(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(stop.labelWith(' · '),
                    style: AppText.of(16, FontWeight.w700, height: 1.35)),
                const SizedBox(height: 4),
                Text('Requested window ${stop.windowStart}–${stop.windowEnd}',
                    style: AppText.of(13, FontWeight.w400,
                        color: AppColors.muted, height: 1.35)),
                if (waiting) ...[
                  const SizedBox(height: 10),
                  _Pill(label: '$earlyMinutes min early'),
                ],
              ],
            ),
          ),
          const SizedBox(height: 14),
          _Card(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  waiting
                      ? 'Wait for the receiving window'
                      : 'Receiving window is open',
                  style: AppText.of(17, FontWeight.w700, height: 1.35),
                ),
                if (waiting) ...[
                  const SizedBox(height: 6),
                  Text(
                    earliestAccess == null
                        ? 'Estimated wait $earlyMinutes min'
                        : 'Earliest access $earliestAccess · estimated wait $earlyMinutes min',
                    style: AppText.of(13, FontWeight.w500,
                        color: AppColors.amber, height: 1.35),
                  ),
                ],
                const SizedBox(height: 14),
                const Divider(height: 1, thickness: 1, color: AppColors.border),
                const SizedBox(height: 15),
                Text(stop.accessNote,
                    style: AppText.of(14, FontWeight.w500, height: 1.35)),
                const SizedBox(height: 6),
                Text(stop.contactNote,
                    style: AppText.of(12, FontWeight.w400,
                        color: AppColors.muted, height: 1.35)),
              ],
            ),
          ),
          const SizedBox(height: 14),
          Container(
            width: double.infinity,
            padding: const EdgeInsets.fromLTRB(16, 14, 16, 14),
            decoration: BoxDecoration(
              color: AppColors.amberBg,
              border: Border.all(color: AppColors.border),
              borderRadius: BorderRadius.circular(AppRadius.r8),
            ),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text('Before continuing',
                    style: AppText.of(15, FontWeight.w700,
                        color: AppColors.amber, height: 1.35)),
                const SizedBox(height: 6),
                Text(
                  'Confirm you are parked safely. Do not interact while driving.',
                  style: AppText.of(12, FontWeight.w400, height: 1.35),
                ),
              ],
            ),
          ),
          const SizedBox(height: 18),
          SheetButton(
            label: 'I’ve stopped safely',
            primary: true,
            fontSize: 14,
            verticalPadding: 10,
            onPressed: onStoppedSafely,
          ),
          const SizedBox(height: 8),
          SheetButton(
            label: 'Open in maps',
            fontSize: 14,
            verticalPadding: 10,
            secondaryTextColor: AppColors.primary,
            secondaryBorderColor: AppColors.border,
            onPressed: onOpenMaps,
          ),
          const SizedBox(height: 14),
          Padding(
            padding: const EdgeInsets.symmetric(horizontal: 8),
            child: Text(
              'Offline ready · route and stop details saved on this phone',
              style: AppText.of(12, FontWeight.w400,
                  color: AppColors.muted, height: 1.35),
            ),
          ),
        ],
      ),
    );
  }
}

class _Card extends StatelessWidget {
  const _Card({required this.child});

  final Widget child;

  @override
  Widget build(BuildContext context) {
    return Container(
      width: double.infinity,
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: AppColors.surface,
        border: Border.all(color: AppColors.border),
        borderRadius: BorderRadius.circular(AppRadius.r8),
      ),
      child: child,
    );
  }
}

class _Pill extends StatelessWidget {
  const _Pill({required this.label});

  final String label;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 4),
      decoration: BoxDecoration(
          color: AppColors.amberBg, borderRadius: BorderRadius.circular(13)),
      child: Text(label,
          style: AppText.of(12, FontWeight.w500,
              color: AppColors.amber, height: 1.35)),
    );
  }
}
