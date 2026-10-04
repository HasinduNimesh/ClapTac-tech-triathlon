import 'package:flutter/material.dart';

import '../../data/driver_models.dart';
import '../../theme/tokens.dart';
import '../../widgets/svg_icon.dart';
import 'route_assets.dart';
import 'route_widgets.dart';

class _Option {
  const _Option(this.kind, this.title, [this.subtitle]);

  final ProblemKind kind;
  final String title;
  final String? subtitle;
}

const _options = <_Option>[
  _Option(ProblemKind.vehicleBreakdown, 'Vehicle breakdown',
      'Dispatcher sees rescue options straight away'),
  _Option(ProblemKind.roadBlocked, 'Road blocked or flooded'),
  _Option(ProblemKind.outletClosed, 'Outlet closed or no access'),
  _Option(ProblemKind.loadIssue, 'Load wrong, missing or damaged'),
  _Option(ProblemKind.safetyConcern, 'Safety concern'),
];

/// Lets the driver tell dispatch about a problem on the road or at a stop.
Future<void> showReportProblemSheet(
  BuildContext context, {
  required TripInfo trip,
  StopInfo? stop,
  required ValueChanged<ProblemReport> onSend,
}) {
  return showModalBottomSheet<void>(
    context: context,
    isScrollControlled: true,
    backgroundColor: Colors.transparent,
    elevation: 0,
    barrierColor: RouteColors.scrim,
    builder: (sheetContext) =>
        _ReportProblemSheet(trip: trip, stop: stop, onSend: onSend),
  );
}

class _ReportProblemSheet extends StatefulWidget {
  const _ReportProblemSheet(
      {required this.trip, required this.stop, required this.onSend});

  final TripInfo trip;
  final StopInfo? stop;
  final ValueChanged<ProblemReport> onSend;

  @override
  State<_ReportProblemSheet> createState() => _ReportProblemSheetState();
}

class _ReportProblemSheetState extends State<_ReportProblemSheet> {
  ProblemKind _selected = ProblemKind.vehicleBreakdown;
  final _note = TextEditingController();

  @override
  void dispose() {
    _note.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final stop = widget.stop;
    final where = stop == null
        ? widget.trip.vehicleCode
        : 'Stop ${stop.sequence} · ${stop.name} · ${widget.trip.vehicleCode}';

    return FloatingSheetCard(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        mainAxisSize: MainAxisSize.min,
        children: [
          Semantics(
              header: true,
              child: Text('Report a problem',
                  style: AppText.of(19, FontWeight.w700,
                      color: RouteColors.sheetInk))),
          const SizedBox(height: 10),
          Text('$where. Your stop, vehicle and time are attached.',
              style: AppText.of(13, FontWeight.w400,
                  color: RouteColors.sheetMuted)),
          for (final option in _options) ...[
            const SizedBox(height: 10),
            _OptionTile(
              option: option,
              selected: option.kind == _selected,
              onTap: () => setState(() => _selected = option.kind),
            ),
          ],
          const SizedBox(height: 10),
          TextField(
            controller: _note,
            minLines: 1,
            maxLines: 3,
            textInputAction: TextInputAction.newline,
            style: AppText.of(14, FontWeight.w400, color: RouteColors.sheetInk),
            decoration: InputDecoration(
              hintText: 'Add a note or a photo (optional)',
              hintStyle: AppText.of(14, FontWeight.w400,
                  color: RouteColors.sheetMuted),
              isDense: true,
              contentPadding: const EdgeInsets.all(13),
              enabledBorder: OutlineInputBorder(
                borderRadius: BorderRadius.circular(AppRadius.r8),
                borderSide: const BorderSide(color: RouteColors.sheetBorder),
              ),
              focusedBorder: OutlineInputBorder(
                borderRadius: BorderRadius.circular(AppRadius.r8),
                borderSide:
                    const BorderSide(color: AppColors.primary, width: 1.5),
              ),
            ),
          ),
          const SizedBox(height: 10),
          Container(
            width: double.infinity,
            padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
            decoration: BoxDecoration(
                color: RouteColors.noteBlueBg,
                borderRadius: BorderRadius.circular(AppRadius.r8)),
            child: Text(
              'No signal? It is saved on this phone and sent when you reconnect.',
              style: AppText.of(13, FontWeight.w500, color: AppColors.primary),
            ),
          ),
          const SizedBox(height: 10),
          SheetButton(
            label: 'Send to dispatcher',
            primary: true,
            onPressed: () {
              final report =
                  ProblemReport(kind: _selected, note: _note.text.trim());
              Navigator.of(context).pop();
              widget.onSend(report);
            },
          ),
          const SizedBox(height: 10),
          SheetButton(
              label: 'Cancel', onPressed: () => Navigator.of(context).pop()),
        ],
      ),
    );
  }
}

class _OptionTile extends StatelessWidget {
  const _OptionTile(
      {required this.option, required this.selected, required this.onTap});

  final _Option option;
  final bool selected;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return Semantics(
      inMutuallyExclusiveGroup: true,
      checked: selected,
      button: true,
      label: option.subtitle == null
          ? option.title
          : '${option.title}. ${option.subtitle}',
      excludeSemantics: true,
      onTap: onTap,
      child: Material(
        color: selected ? RouteColors.noteBlueBg : Colors.white,
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(10),
          side: BorderSide(
              color: selected ? AppColors.primary : RouteColors.sheetBorder,
              width: selected ? 1.5 : 1),
        ),
        child: InkWell(
          borderRadius: BorderRadius.circular(10),
          onTap: onTap,
          child: ConstrainedBox(
            constraints: const BoxConstraints(minHeight: 44),
            child: Padding(
              padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 13),
              child: Row(
                children: [
                  SvgIcon(selected
                      ? RouteAssets.radioSelected
                      : RouteAssets.radioUnselected),
                  const SizedBox(width: 12),
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text(option.title,
                            style: AppText.of(15, FontWeight.w600,
                                color: RouteColors.sheetInk)),
                        if (option.subtitle != null) ...[
                          const SizedBox(height: 2),
                          Text(option.subtitle!,
                              style: AppText.of(13, FontWeight.w400,
                                  color: RouteColors.sheetMuted)),
                        ],
                      ],
                    ),
                  ),
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }
}
