import 'package:flutter/material.dart';

import '../../data/driver_models.dart';
import '../../theme/tokens.dart';
import 'route_widgets.dart';

/// One row in the "Check the load before you leave" list.
class LoadCheckLine {
  const LoadCheckLine({required this.title, this.detail, this.warning = false});

  final String title;
  final String? detail;

  /// Shows an amber "!" instead of a green tick.
  final bool warning;
}

/// Pre-departure load check. [lines] defaults to one line per stop built from
/// [trip]; pass explicit lines to include shortfalls or cold-chain checks.
Future<void> showTruckCheckoutSheet(
  BuildContext context, {
  required TripInfo trip,
  required VoidCallback onConfirm,
  required VoidCallback onMissingItem,
  List<LoadCheckLine>? lines,
  String? planVersion,
  String? sealNumber,
}) {
  final checks = lines ??
      [
        for (final stop in trip.stops)
          LoadCheckLine(
              title:
                  '${stop.sequenceTitle} · ${stop.unitsText} on board'),
      ];
  final details = [
    trip.vehicleCode,
    if (trip.plate.isNotEmpty) trip.plate,
    trip.tripRef,
    if (planVersion != null) 'Plan $planVersion',
    if (sealNumber != null) 'Seal $sealNumber',
  ].join(' · ');

  return showModalBottomSheet<void>(
    context: context,
    isScrollControlled: true,
    backgroundColor: Colors.transparent,
    elevation: 0,
    barrierColor: RouteColors.scrim,
    builder: (sheetContext) => FloatingSheetCard(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        mainAxisSize: MainAxisSize.min,
        children: [
          Semantics(
            header: true,
            child: Text('Check the load before you leave',
                style: AppText.of(19, FontWeight.w700,
                    color: RouteColors.sheetInk)),
          ),
          const SizedBox(height: 12),
          Text(details,
              style: AppText.of(13, FontWeight.w400,
                  color: RouteColors.sheetMuted)),
          for (final line in checks) ...[
            const SizedBox(height: 12),
            _CheckRow(line: line),
          ],
          const SizedBox(height: 12),
          SheetButton(
            label: 'Confirm load on board',
            primary: true,
            onPressed: () {
              Navigator.of(sheetContext).pop();
              onConfirm();
            },
          ),
          const SizedBox(height: 12),
          SheetButton(
            label: 'Something isn\'t on my list',
            onPressed: () {
              Navigator.of(sheetContext).pop();
              onMissingItem();
            },
          ),
          const SizedBox(height: 12),
          Text(
            'When you confirm, the load check goes to Waypoint so the loader and dispatcher can see it. With no signal it waits on this phone and is sent when you are back online.',
            style:
                AppText.of(12, FontWeight.w400, color: RouteColors.sheetMuted),
          ),
        ],
      ),
    ),
  );
}

class _CheckRow extends StatelessWidget {
  const _CheckRow({required this.line});

  final LoadCheckLine line;

  @override
  Widget build(BuildContext context) {
    final color =
        line.warning ? RouteColors.warnOrange : RouteColors.checkGreen;
    return Semantics(
      container: true,
      label:
          '${line.warning ? 'Needs attention' : 'Checked'}: ${line.title}${line.detail == null ? '' : '. ${line.detail}'}',
      child: ExcludeSemantics(
        child: Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(line.warning ? '!' : '✓',
                style: AppText.of(14, FontWeight.w700, color: color)),
            const SizedBox(width: 10),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(line.title,
                      style: AppText.of(14, FontWeight.w500,
                          color: RouteColors.sheetInk)),
                  if (line.detail != null) ...[
                    const SizedBox(height: 2),
                    Text(line.detail!,
                        style: AppText.of(12, FontWeight.w400,
                            color: line.warning
                                ? RouteColors.warnOrange
                                : RouteColors.sheetMuted)),
                  ],
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}
