import 'package:flutter/material.dart';

import '../theme/tokens.dart';

class WpCard extends StatelessWidget {
  const WpCard({super.key, required this.child, this.padding = const EdgeInsets.all(Wp.space16), this.color = Wp.surface, this.borderColor = Wp.border, this.borderWidth = 1, this.onTap});

  final Widget child;
  final EdgeInsetsGeometry padding;
  final Color color;
  final Color borderColor;
  final double borderWidth;
  final VoidCallback? onTap;

  @override
  Widget build(BuildContext context) {
    final card = Container(
      padding: padding,
      decoration: BoxDecoration(color: color, borderRadius: BorderRadius.circular(Wp.radius), border: Border.all(color: borderColor, width: borderWidth)),
      child: child,
    );
    if (onTap == null) return card;
    return Material(color: Colors.transparent, child: InkWell(borderRadius: BorderRadius.circular(Wp.radius), onTap: onTap, child: card));
  }
}

class StatusTag extends StatelessWidget {
  const StatusTag(this.label, {super.key, this.tone = Tone.muted, this.icon});

  final String label;
  final Tone tone;
  final IconData? icon;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 9, vertical: 3),
      decoration: BoxDecoration(color: tone.bg, borderRadius: BorderRadius.circular(999)),
      child: Row(mainAxisSize: MainAxisSize.min, children: [
        if (icon != null) ...[Icon(icon, size: 13, color: tone.fg), const SizedBox(width: 4)],
        Text(label, style: TextStyle(fontSize: 12, fontWeight: FontWeight.w600, color: tone.fg)),
      ]),
    );
  }
}

/// One sentence that says what happened and what to do (UI Kit "Status note").
class StatusNote extends StatelessWidget {
  const StatusNote({super.key, required this.text, this.title, this.tone = Tone.primary, this.icon, this.onDismiss, this.trailing});

  final String text;
  final String? title;
  final Tone tone;
  final IconData? icon;
  final VoidCallback? onDismiss;
  final Widget? trailing;

  @override
  Widget build(BuildContext context) {
    return Semantics(
      liveRegion: tone == Tone.red || tone == Tone.amber,
      child: Container(
        padding: const EdgeInsets.all(Wp.space12),
        decoration: BoxDecoration(color: tone.bg, borderRadius: BorderRadius.circular(Wp.radiusSm), border: Border.all(color: tone.fg.withValues(alpha: 0.25))),
        child: Row(crossAxisAlignment: CrossAxisAlignment.start, children: [
          if (icon != null) ...[Icon(icon, size: 20, color: tone.fg), const SizedBox(width: Wp.space8)],
          Expanded(
            child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
              if (title != null) Text(title!, style: TextStyle(fontWeight: FontWeight.w700, color: tone == Tone.primary ? Wp.ink : tone.fg)),
              Text(text, style: TextStyle(fontSize: 13, color: tone == Tone.primary ? Wp.primary : (title != null ? Wp.ink : tone.fg))),
            ]),
          ),
          if (trailing != null) trailing!,
          if (onDismiss != null) IconButton(onPressed: onDismiss, icon: const Icon(Icons.close, size: 18), tooltip: 'Dismiss', visualDensity: VisualDensity.compact),
        ]),
      ),
    );
  }
}

class WpProgress extends StatelessWidget {
  const WpProgress({super.key, required this.value, this.color = Wp.primary, this.height = 8, required this.label});

  final double value;
  final Color color;
  final double height;
  final String label;

  @override
  Widget build(BuildContext context) {
    return Semantics(
      label: label,
      value: '${(value.clamp(0, 1) * 100).round()}%',
      child: ClipRRect(
        borderRadius: BorderRadius.circular(height),
        child: LinearProgressIndicator(value: value.clamp(0, 1).toDouble(), minHeight: height, color: color, backgroundColor: Wp.canvas),
      ),
    );
  }
}

class Meter extends StatelessWidget {
  const Meter({super.key, required this.label, required this.valueText, required this.fraction, this.color = Wp.primary, this.tag});

  final String label;
  final String valueText;
  final double fraction;
  final Color color;
  final Widget? tag;

  @override
  Widget build(BuildContext context) {
    return Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
      Row(children: [
        Text(label, style: const TextStyle(fontWeight: FontWeight.w500)),
        if (tag != null) ...[const SizedBox(width: 6), tag!],
        const Spacer(),
        Text(valueText, style: TextStyle(fontWeight: FontWeight.w600, color: color == Wp.amber ? Wp.amber : Wp.ink)),
      ]),
      const SizedBox(height: 6),
      WpProgress(value: fraction, color: color, label: label),
    ]);
  }
}

class CheckRow extends StatelessWidget {
  const CheckRow({super.key, required this.label, required this.state, this.detail});

  /// ok, bad, warn, todo, now
  final String state;
  final String label;
  final String? detail;

  @override
  Widget build(BuildContext context) {
    final (Color bg, IconData? icon, Color fg) = switch (state) {
      'ok' => (Wp.green, Icons.check, Colors.white),
      'bad' => (Wp.red, Icons.priority_high, Colors.white),
      'warn' => (const Color(0xFFD9822B), Icons.priority_high, Colors.white),
      'now' => (Wp.primary, Icons.more_horiz, Colors.white),
      _ => (Wp.surface, null, Wp.muted),
    };
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 4),
      child: Row(crossAxisAlignment: CrossAxisAlignment.start, children: [
        Container(
          width: 22,
          height: 22,
          decoration: BoxDecoration(color: bg, shape: BoxShape.circle, border: state == 'todo' ? Border.all(color: Wp.border, width: 1.5) : null),
          child: icon == null ? null : Icon(icon, size: 14, color: fg),
        ),
        const SizedBox(width: 10),
        Expanded(
          child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
            Text(label, style: TextStyle(fontSize: 14, fontWeight: state == 'bad' ? FontWeight.w600 : FontWeight.w400, color: state == 'bad' ? Wp.red : state == 'todo' ? Wp.muted : Wp.ink)),
            if (detail != null) Text(detail!, style: const TextStyle(fontSize: 12, color: Wp.muted)),
          ]),
        ),
      ]),
    );
  }
}

class SectionTitle extends StatelessWidget {
  const SectionTitle(this.text, {super.key, this.trailing});

  final String text;
  final Widget? trailing;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.only(top: Wp.space8, bottom: Wp.space8),
      child: Row(children: [
        Expanded(child: Text(text, style: const TextStyle(fontSize: 16, fontWeight: FontWeight.w700))),
        if (trailing != null) trailing!,
      ]),
    );
  }
}

class KvRow extends StatelessWidget {
  const KvRow(this.label, this.value, {super.key, this.valueColor});

  final String label;
  final String value;
  final Color? valueColor;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 6),
      child: Row(crossAxisAlignment: CrossAxisAlignment.start, children: [
        Expanded(child: Text(label, style: const TextStyle(color: Wp.muted, fontSize: 14))),
        const SizedBox(width: 12),
        Flexible(child: Text(value, textAlign: TextAlign.right, style: TextStyle(fontWeight: FontWeight.w600, fontSize: 14, color: valueColor ?? Wp.ink))),
      ]),
    );
  }
}

/// Radio option card with a consequence underneath (UI Kit "Choices").
class OptionCard extends StatelessWidget {
  const OptionCard({super.key, required this.title, required this.selected, required this.onTap, this.subtitle, this.enabled = true});

  final String title;
  final String? subtitle;
  final bool selected;
  final bool enabled;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return Semantics(
      selected: selected,
      button: true,
      enabled: enabled,
      child: WpCard(
        onTap: enabled ? onTap : null,
        color: selected ? const Color(0xFFF7F8FF) : Wp.surface,
        borderColor: selected ? Wp.primary : Wp.border,
        borderWidth: selected ? 2 : 1,
        padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 12),
        child: Row(children: [
          Icon(selected ? Icons.radio_button_checked : Icons.radio_button_unchecked, color: enabled ? Wp.primary : Wp.border),
          const SizedBox(width: 12),
          Expanded(
            child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
              Text(title, style: TextStyle(fontWeight: FontWeight.w600, color: enabled ? Wp.ink : Wp.muted)),
              if (subtitle != null) Text(subtitle!, style: const TextStyle(fontSize: 12, color: Wp.muted)),
            ]),
          ),
        ]),
      ),
    );
  }
}

/// Bottom sheet frame used on the phone: one decision, one primary action, one way out.
Future<T?> showWpSheet<T>(BuildContext context, {required String title, String? subtitle, required Widget Function(BuildContext) builder}) {
  return showModalBottomSheet<T>(
    context: context,
    isScrollControlled: true,
    showDragHandle: true,
    backgroundColor: Wp.surface,
    shape: const RoundedRectangleBorder(borderRadius: BorderRadius.vertical(top: Radius.circular(20))),
    builder: (ctx) => SafeArea(
      child: Padding(
        padding: EdgeInsets.fromLTRB(20, 0, 20, 20 + MediaQuery.of(ctx).viewInsets.bottom),
        child: SingleChildScrollView(
          child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, mainAxisSize: MainAxisSize.min, children: [
            Text(title, style: const TextStyle(fontSize: 20, fontWeight: FontWeight.w700)),
            if (subtitle != null) Padding(padding: const EdgeInsets.only(top: 4), child: Text(subtitle, style: const TextStyle(color: Wp.muted, fontSize: 13))),
            const SizedBox(height: Wp.space16),
            builder(ctx),
          ]),
        ),
      ),
    ),
  );
}

/// Blue header band with the Waypoint mark, matching the phone frames.
class WpPhoneHeader extends StatelessWidget {
  const WpPhoneHeader({super.key, required this.trailing});

  final String trailing;

  @override
  Widget build(BuildContext context) {
    return Container(
      decoration: const BoxDecoration(gradient: LinearGradient(colors: [Wp.heroStart, Wp.heroEnd], begin: Alignment.topLeft, end: Alignment.bottomRight)),
      padding: EdgeInsets.fromLTRB(20, MediaQuery.of(context).padding.top + 10, 20, 26),
      child: Row(children: [
        Image.asset('assets/waypoint-logo.png', height: 30, color: Colors.white, colorBlendMode: BlendMode.srcIn, semanticLabel: 'Waypoint'),
        const Spacer(),
        Text(trailing, style: const TextStyle(color: Colors.white, fontSize: 20, fontWeight: FontWeight.w600)),
      ]),
    );
  }
}

String hhmm(String? iso) {
  if (iso == null || iso.isEmpty) return '—';
  final parsed = DateTime.tryParse(iso);
  if (parsed == null) return iso.length >= 5 ? iso.substring(0, 5) : iso;
  final local = parsed.toUtc().add(const Duration(hours: 5, minutes: 30));
  return '${local.hour.toString().padLeft(2, '0')}:${local.minute.toString().padLeft(2, '0')}';
}

String todayLabel([DateTime? now]) {
  final d = (now ?? DateTime.now()).toUtc().add(const Duration(hours: 5, minutes: 30));
  const days = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'];
  const months = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];
  return '${days[d.weekday - 1]} ${d.day} ${months[d.month - 1]}';
}

String todayIso([DateTime? now]) {
  final d = (now ?? DateTime.now()).toUtc().add(const Duration(hours: 5, minutes: 30));
  return '${d.year}-${d.month.toString().padLeft(2, '0')}-${d.day.toString().padLeft(2, '0')}';
}
