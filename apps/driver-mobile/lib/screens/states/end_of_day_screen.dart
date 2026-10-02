import 'package:flutter/material.dart';

import '../../theme/tokens.dart';
import '../../widgets/driver_shell.dart';
import '../../widgets/svg_icon.dart';
import 'states_assets.dart';
import 'states_widgets.dart';

class StopResult {
  const StopResult({required this.name, required this.window, required this.status, this.detail});

  final String name;
  final String window;
  final String status;
  final String? detail;
}

class EndOfDaySummary {
  const EndOfDaySummary({
    required this.totalStops,
    required this.delivered,
    required this.partial,
    required this.failedOrRefused,
    required this.completed,
    required this.unresolved,
    this.pendingUploads = 0,
  });

  final int totalStops;
  final int delivered;
  final int partial;
  final int failedOrRefused;
  final List<StopResult> completed;
  final List<StopResult> unresolved;
  final int pendingUploads;
}

const sampleEndOfDay = EndOfDaySummary(
  totalStops: 3,
  delivered: 2,
  partial: 1,
  failedOrRefused: 0,
  completed: [
    StopResult(name: 'Dehiwala', window: '08:00 - 09:00', status: 'Delivered'),
    StopResult(name: 'Nugegoda', window: '09:00 - 10:00', status: 'Delivered'),
  ],
  unresolved: [
    StopResult(
      name: 'Kirulapone',
      window: '10:00 - 11:00',
      status: 'Partial delivery',
      detail: '2 cartons short - Dispatcher notified',
    ),
  ],
  pendingUploads: 1,
);

/// "Route Complete" summary shown after the last stop.
class EndOfDayScreen extends StatelessWidget {
  const EndOfDayScreen({
    super.key,
    required this.summary,
    required this.onFinishTrip,
    this.onRetryUpload,
    this.onTabSelected,
  });

  final EndOfDaySummary summary;
  final VoidCallback onFinishTrip;
  final VoidCallback? onRetryUpload;
  final ValueChanged<DriverTab>? onTabSelected;

  @override
  Widget build(BuildContext context) {
    final pending = summary.pendingUploads;
    return DriverShell(
      tab: DriverTab.summary,
      onTabSelected: onTabSelected,
      bodyPadding: const EdgeInsets.fromLTRB(AppSpace.s24, 20, AppSpace.s24, AppSpace.s24),
      body: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          const _CompleteBanner(),
          const SizedBox(height: 6),
          Row(
            children: [
              Expanded(
                child: _Tile(
                  color: tinted(108, 222, 137, 0.4),
                  icon: StatesAssets.tileCompleted,
                  label: 'Completed',
                  value: summary.completed.length,
                ),
              ),
              const SizedBox(width: 16),
              Expanded(
                child: _Tile(
                  color: tinted(225, 207, 126, 0.4),
                  icon: StatesAssets.tileAlert,
                  label: 'Unresolved',
                  value: summary.unresolved.length,
                ),
              ),
            ],
          ),
          const SizedBox(height: 11),
          _StopsBox(summary: summary),
          if (summary.completed.isNotEmpty) ...[
            const SizedBox(height: 7),
            _sectionLabel('Completed stops'),
            const SizedBox(height: 7),
            _CompletedCard(results: summary.completed),
          ],
          if (summary.unresolved.isNotEmpty) ...[
            const SizedBox(height: 7),
            _sectionLabel('Unresolved stops'),
            const SizedBox(height: 7),
            _UnresolvedCard(results: summary.unresolved),
          ],
          if (pending > 0) ...[
            const SizedBox(height: 7),
            _sectionLabel('Pending uploads'),
            const SizedBox(height: 7),
            _PendingRow(count: pending, onRetry: onRetryUpload),
            const SizedBox(height: 7),
            DismissibleNote(
              text: pending == 1
                  ? 'Delivery records saved. One proof upload is pending.'
                  : 'Delivery records saved. $pending proof uploads are pending.',
              textStyle: AppText.of(10, FontWeight.w400, height: 19 / 10),
            ),
          ],
          SizedBox(height: pending > 0 ? 12 : 24),
          StatesButton(label: 'Finish trip', onPressed: onFinishTrip),
        ],
      ),
    );
  }

  Widget _sectionLabel(String text) => Semantics(
        header: true,
        child: figmaText(text, weight: FontWeight.w700, lineHeight: 19),
      );
}

class _CompleteBanner extends StatelessWidget {
  const _CompleteBanner();

  @override
  Widget build(BuildContext context) {
    return Semantics(
      container: true,
      label: 'Route Complete. You’ve finished today’s route',
      child: ExcludeSemantics(
        child: StatesCard(
          height: 80,
          color: tinted(108, 222, 137, 0.4),
          borderColor: const Color(0xFFF4FBF5),
          children: [
            const Positioned(left: 36 - 1, top: 6 - 1, child: SvgIcon(StatesAssets.eodDisc)),
            const Positioned(left: 51 - 1, top: 18 - 1, child: SvgIcon(StatesAssets.eodCheck)),
            Positioned(left: 120 - 1, top: 16 - 1, child: figmaText('Route Complete', size: 20, weight: FontWeight.w700, lineHeight: 22)),
            Positioned(left: 120 - 1, top: 45 - 1, child: figmaText('You’ve finished today’s route', lineHeight: 22)),
          ],
        ),
      ),
    );
  }
}

class _Tile extends StatelessWidget {
  const _Tile({required this.color, required this.icon, required this.label, required this.value});

  final Color color;
  final String icon;
  final String label;
  final int value;

  @override
  Widget build(BuildContext context) {
    return StatesCard(
      height: 57,
      color: color,
      borderColor: const Color(0xFFF4FBF5),
      semanticLabel: '$label $value',
      children: [
        Positioned(left: 14 - 1, top: 7 - 1, child: SvgIcon(icon)),
        Positioned(left: 60 - 1, top: 11 - 1, child: figmaText(label, size: 10, weight: FontWeight.w500, lineHeight: 12)),
        Positioned(left: 60 - 1, top: 27 - 1, child: figmaText('$value', size: 16, weight: FontWeight.w700, lineHeight: 19)),
      ],
    );
  }
}

class _StopsBox extends StatelessWidget {
  const _StopsBox({required this.summary});

  final EndOfDaySummary summary;

  @override
  Widget build(BuildContext context) {
    Widget row(String label, int value, {bool bold = false}) => Padding(
          padding: const EdgeInsets.only(bottom: 0),
          child: Row(
            mainAxisAlignment: MainAxisAlignment.spaceBetween,
            children: [
              figmaText(label, weight: bold ? FontWeight.w700 : FontWeight.w400, lineHeight: 19),
              figmaText('$value', weight: bold ? FontWeight.w700 : FontWeight.w400, lineHeight: 19),
            ],
          ),
        );
    return Container(
      height: 95,
      padding: const EdgeInsets.fromLTRB(11, 8, 17, 0),
      decoration: BoxDecoration(
        color: tinted(147, 191, 240, 0.19),
        borderRadius: BorderRadius.circular(AppRadius.r8),
        border: Border.all(color: const Color(0xFF0051FF)),
      ),
      child: Column(
        children: [
          row('Stops:', summary.totalStops, bold: true),
          row('Delivered:', summary.delivered),
          row('Partial:', summary.partial),
          row('Failed / Refused:', summary.failedOrRefused),
        ],
      ),
    );
  }
}

class _CompletedCard extends StatelessWidget {
  const _CompletedCard({required this.results});

  final List<StopResult> results;

  @override
  Widget build(BuildContext context) {
    final height = results.length * 46.0 + 12;
    final children = <Widget>[];
    for (var i = 0; i < results.length; i++) {
      final r = results[i];
      final top = i * 46.0;
      children.addAll([
        Positioned(left: 7 - 1, top: top + 8 - 1, child: const SvgIcon(StatesAssets.stopDisc)),
        Positioned(left: 16 - 1, top: top + 15 - 1, child: const SvgIcon(StatesAssets.stopCheck)),
        Positioned(left: 52 - 1, top: top + 11 - 1, child: figmaText(r.name, weight: FontWeight.w700, lineHeight: 19)),
        Positioned(left: 51 - 1, top: top + 24 - 1, child: figmaText(r.status, size: 10, color: const Color(0xFF143E19), lineHeight: 19)),
        Positioned(right: 28, top: top + 12 - 1, child: figmaText(r.window, lineHeight: 19)),
        if (i < results.length - 1) Positioned(left: 51 - 1, right: 28, top: top + 46, child: const SvgIcon(StatesAssets.line)),
      ]);
    }
    return StatesCard(
      height: height,
      color: tinted(166, 220, 134, 0.19),
      borderColor: AppColors.separator,
      children: children,
    );
  }
}

class _UnresolvedCard extends StatelessWidget {
  const _UnresolvedCard({required this.results});

  final List<StopResult> results;

  @override
  Widget build(BuildContext context) {
    return Column(
      children: [
        for (var i = 0; i < results.length; i++) ...[
          if (i > 0) const SizedBox(height: 7),
          StatesCard(
            height: 71,
            color: tinted(225, 207, 126, 0.31),
            borderColor: AppColors.separator,
            children: [
              const Positioned(left: 7 - 1, top: 5 - 1, child: SvgIcon(StatesAssets.tileAlert)),
              Positioned(left: 52 - 1, top: 11 - 1, child: figmaText(results[i].name, weight: FontWeight.w700, lineHeight: 19)),
              Positioned(left: 51 - 1, top: 24 - 1, child: figmaText(results[i].status, size: 10, color: const Color(0xFF9A6009), lineHeight: 19)),
              Positioned(right: 25, top: 11 - 1, child: figmaText(results[i].window, lineHeight: 19)),
              if (results[i].detail != null)
                Positioned(left: 51 - 1, top: 42 - 1, child: figmaText(results[i].detail!, size: 10, color: const Color(0xFF040200), lineHeight: 19)),
            ],
          ),
        ],
      ],
    );
  }
}

class _PendingRow extends StatelessWidget {
  const _PendingRow({required this.count, required this.onRetry});

  final int count;
  final VoidCallback? onRetry;

  @override
  Widget build(BuildContext context) {
    final text = count == 1 ? '1 photo waiting to be uploaded' : '$count photos waiting to be uploaded';
    return Row(
      children: [
        Expanded(
          child: StatesCard(
            height: 40,
            color: tinted(179, 198, 230, 0.25),
            borderColor: AppColors.separator,
            semanticLabel: text,
            children: [
              const Positioned(left: 14 - 1, top: 5 - 1, child: SvgIcon(StatesAssets.rowCloudUpload)),
              Positioned(left: 52 - 1, top: 11 - 1, right: 4, child: figmaText(text, size: 10, weight: FontWeight.w700, lineHeight: 19)),
            ],
          ),
        ),
        const SizedBox(width: 9),
        SizedBox(
          width: 118,
          height: 44,
          child: Material(
            color: tinted(147, 191, 240, 0.19),
            shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(AppRadius.r8), side: const BorderSide(color: Color(0xFF0051FF))),
            child: InkWell(
              borderRadius: BorderRadius.circular(AppRadius.r8),
              onTap: onRetry,
              child: Center(child: figmaText('Retry Upload', size: 12, weight: FontWeight.w700, color: const Color(0xFF0310FC), lineHeight: 19)),
            ),
          ),
        ),
      ],
    );
  }
}
