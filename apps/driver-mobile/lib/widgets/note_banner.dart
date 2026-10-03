import 'package:flutter/material.dart';

import '../theme/assets.dart';
import '../theme/tokens.dart';
import 'svg_icon.dart';

enum NoteTone { info, danger, amber, success, offline }

/// Compact status note ("Use only when safely parked.", "No connection", ...).
class NoteBanner extends StatelessWidget {
  const NoteBanner({
    super.key,
    required this.text,
    this.title,
    this.icon,
    this.tone = NoteTone.info,
    this.onDismiss,
    this.textStyle,
  });

  final String text;
  final String? title;
  final Widget? icon;
  final NoteTone tone;
  final VoidCallback? onDismiss;
  final TextStyle? textStyle;

  Color get _background {
    switch (tone) {
      case NoteTone.info:
        return const Color(0x0F0400FF);
      case NoteTone.danger:
        return const Color(0x0FFF0000);
      case NoteTone.amber:
        return AppColors.amberBg;
      case NoteTone.success:
        return AppColors.greenBg;
      case NoteTone.offline:
        return const Color(0xFFF1E3C4);
    }
  }

  @override
  Widget build(BuildContext context) {
    final style = textStyle ?? AppText.of(13, FontWeight.w400);
    return Semantics(
      container: true,
      child: Container(
        constraints: const BoxConstraints(minHeight: 44),
        padding: EdgeInsets.symmetric(horizontal: 12, vertical: onDismiss == null ? 10 : 0),
        decoration: BoxDecoration(color: _background, borderRadius: BorderRadius.circular(AppRadius.r8)),
        child: Row(
          crossAxisAlignment: CrossAxisAlignment.center,
          children: [
            if (icon != null) ...[icon!, const SizedBox(width: 10)],
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                mainAxisSize: MainAxisSize.min,
                children: [
                  if (title != null) Text(title!, style: AppText.of(13, FontWeight.w700)),
                  Text(text, style: style),
                ],
              ),
            ),
            if (onDismiss != null)
              IconButton(
                onPressed: onDismiss,
                tooltip: 'Dismiss',
                padding: EdgeInsets.zero,
                constraints: const BoxConstraints(minWidth: 44, minHeight: 44),
                visualDensity: VisualDensity.compact,
                icon: const SvgIcon(AppAssets.close, size: 14),
              ),
          ],
        ),
      ),
    );
  }
}
