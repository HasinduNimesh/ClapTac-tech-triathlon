import 'package:flutter/material.dart';

import '../../theme/tokens.dart';
import '../../widgets/driver_shell.dart';
import '../../widgets/note_banner.dart';

/// One entry in the Updates tab (sync results, dispatcher notices, plan changes).
class UpdateItem {
  const UpdateItem({required this.title, required this.detail, required this.time, this.tone = NoteTone.info, this.id});

  final String? id;
  final String title;
  final String detail;
  final String time;
  final NoteTone tone;
}

/// The Updates tab has no dedicated Figma frame; it reuses the note banners and
/// typography of the other screens.
class UpdatesScreen extends StatelessWidget {
  const UpdatesScreen({super.key, required this.items, this.onOpen, this.onTabSelected});

  final List<UpdateItem> items;
  final ValueChanged<UpdateItem>? onOpen;
  final ValueChanged<DriverTab>? onTabSelected;

  @override
  Widget build(BuildContext context) {
    return DriverShell(
      tab: DriverTab.updates,
      onTabSelected: onTabSelected,
      body: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Semantics(header: true, child: Text('Updates', style: AppText.of(22, FontWeight.w700))),
          const SizedBox(height: 4),
          Text('Saved deliveries, uploads and dispatcher notices.', style: AppText.of(13, FontWeight.w400, color: AppColors.muted)),
          const SizedBox(height: 16),
          if (items.isEmpty)
            const NoteBanner(text: 'No updates yet. Saved deliveries and dispatcher messages appear here.')
          else
            for (final item in items) ...[
              _UpdateCard(item: item, onOpen: onOpen == null ? null : () => onOpen!(item)),
              const SizedBox(height: 12),
            ],
        ],
      ),
    );
  }
}

class _UpdateCard extends StatelessWidget {
  const _UpdateCard({required this.item, this.onOpen});

  final UpdateItem item;
  final VoidCallback? onOpen;

  Color get _accent {
    switch (item.tone) {
      case NoteTone.danger:
        return AppColors.red;
      case NoteTone.amber:
      case NoteTone.offline:
        return AppColors.amber;
      case NoteTone.success:
        return AppColors.green;
      case NoteTone.info:
        return AppColors.primary;
    }
  }

  Color get _background {
    switch (item.tone) {
      case NoteTone.danger:
        return AppColors.redBg;
      case NoteTone.amber:
      case NoteTone.offline:
        return AppColors.amberBg;
      case NoteTone.success:
        return AppColors.greenBg;
      case NoteTone.info:
        return AppColors.tint;
    }
  }

  @override
  Widget build(BuildContext context) {
    return Semantics(
      button: onOpen != null,
      label: '${item.title}. ${item.detail}. ${item.time}',
      excludeSemantics: true,
      child: Material(
        color: _background,
        borderRadius: BorderRadius.circular(AppRadius.r8),
        child: InkWell(
          borderRadius: BorderRadius.circular(AppRadius.r8),
          onTap: onOpen,
          child: Padding(
            padding: const EdgeInsets.all(14),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Row(
                  children: [
                    Expanded(child: Text(item.title, style: AppText.of(14, FontWeight.w700, color: _accent))),
                    Text(item.time, style: AppText.of(12, FontWeight.w400, color: AppColors.muted)),
                  ],
                ),
                const SizedBox(height: 4),
                Text(item.detail, style: AppText.of(13, FontWeight.w400)),
              ],
            ),
          ),
        ),
      ),
    );
  }
}
