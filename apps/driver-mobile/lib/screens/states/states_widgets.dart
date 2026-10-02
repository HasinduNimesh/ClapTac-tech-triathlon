import 'package:flutter/material.dart';

import '../../theme/assets.dart';
import '../../theme/tokens.dart';
import '../../widgets/svg_icon.dart';

/// The Figma cards sit at 86% opacity over white; this bakes that into the tint
/// so text on top stays fully opaque.
Color tinted(int r, int g, int b, double alpha) => Color.fromRGBO(r, g, b, alpha * 0.86);

/// Text positioned by Figma line box (px) rather than a unitless multiplier.
Text figmaText(
  String text, {
  double size = 13,
  FontWeight weight = FontWeight.w400,
  Color color = Colors.black,
  double? lineHeight,
  TextAlign? align,
  TextOverflow? overflow,
  int? maxLines,
}) {
  return Text(
    text,
    textAlign: align,
    overflow: overflow,
    maxLines: maxLines,
    style: AppText.of(size, weight, color: color, height: (lineHeight ?? size * 1.4) / size),
  );
}

/// Bordered, tinted container whose children are placed with [Positioned].
class StatesCard extends StatelessWidget {
  const StatesCard({
    super.key,
    required this.height,
    required this.color,
    required this.borderColor,
    required this.children,
    this.semanticLabel,
  });

  final double height;
  final Color color;
  final Color borderColor;
  final List<Widget> children;
  final String? semanticLabel;

  @override
  Widget build(BuildContext context) {
    final card = Container(
      height: height,
      decoration: BoxDecoration(
        color: color,
        borderRadius: BorderRadius.circular(AppRadius.r8),
        border: Border.all(color: borderColor),
      ),
      clipBehavior: Clip.hardEdge,
      child: Stack(clipBehavior: Clip.none, children: children),
    );
    if (semanticLabel == null) return card;
    return Semantics(container: true, label: semanticLabel, child: ExcludeSemantics(child: card));
  }
}

/// Connectivity banner at the top of the sync screens. Announced to screen
/// readers when its text changes.
class StatusBanner extends StatelessWidget {
  const StatusBanner({
    super.key,
    required this.color,
    required this.icon,
    required this.iconSize,
    required this.title,
    required this.subtitle,
  });

  final Color color;
  final String icon;
  final double iconSize;
  final String title;
  final String subtitle;

  @override
  Widget build(BuildContext context) {
    return Semantics(
      liveRegion: true,
      container: true,
      label: '$title. $subtitle',
      child: ExcludeSemantics(
        child: Container(
          height: 55,
          decoration: BoxDecoration(
            color: color,
            borderRadius: BorderRadius.circular(AppRadius.r8),
            border: Border.all(color: const Color(0xFFF4FBF5)),
          ),
          child: Row(
            children: [
              SizedBox(
                width: 90,
                child: Align(alignment: const Alignment(0.0, 0), child: SvgIcon(icon, size: iconSize)),
              ),
              Expanded(
                child: Column(
                  mainAxisAlignment: MainAxisAlignment.center,
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    figmaText(title, size: 12, weight: FontWeight.w700, lineHeight: 15),
                    const SizedBox(height: 2),
                    figmaText(subtitle, size: 10, lineHeight: 13),
                  ],
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

class GlyphSpec {
  const GlyphSpec(this.asset, this.width, this.height, this.dx, this.dy);

  final String asset;
  final double width;
  final double height;

  /// Offset of the glyph centre from the circle centre.
  final double dx;
  final double dy;
}

/// Large circle with a glyph on top, centred in the sheet.
class StateIllustration extends StatelessWidget {
  const StateIllustration({super.key, required this.circle, required this.glyphs});

  static const double width = 119.25;
  static const double height = 115.25;

  final String circle;
  final List<GlyphSpec> glyphs;

  @override
  Widget build(BuildContext context) {
    return ExcludeSemantics(
      child: Center(
        child: SizedBox(
          width: width,
          height: height,
          child: Stack(
            clipBehavior: Clip.none,
            children: [
              SvgIcon(circle),
              for (final g in glyphs)
                Positioned(
                  left: width / 2 + g.dx - g.width / 2,
                  top: height / 2 + g.dy - g.height / 2,
                  child: SvgIcon(g.asset),
                ),
            ],
          ),
        ),
      ),
    );
  }
}

/// The "Follow the planned stop order." style note that the driver can dismiss.
/// Laid out like the Figma frame: icon at 10px, text at 42px, 14px close icon
/// at the right edge with a 44px tap target.
class DismissibleNote extends StatefulWidget {
  const DismissibleNote({super.key, required this.text, this.minHeight = 44, this.textStyle});

  final String text;
  final double minHeight;
  final TextStyle? textStyle;

  @override
  State<DismissibleNote> createState() => _DismissibleNoteState();
}

class _DismissibleNoteState extends State<DismissibleNote> {
  bool _visible = true;

  @override
  Widget build(BuildContext context) {
    if (!_visible) return const SizedBox.shrink();
    final style = widget.textStyle ?? AppText.of(13, FontWeight.w400, height: 19 / 13);
    return Semantics(
      container: true,
      child: Stack(
        clipBehavior: Clip.none,
        children: [
          Container(
            constraints: BoxConstraints(minHeight: widget.minHeight),
            decoration: BoxDecoration(color: const Color(0x0F0400FF), borderRadius: BorderRadius.circular(AppRadius.r8)),
            padding: const EdgeInsets.only(left: 10, right: 30),
            child: Row(
              children: [
                const SvgIcon(AppAssets.infoCircle, size: 24),
                const SizedBox(width: 8),
                Expanded(
                  child: Padding(
                    padding: const EdgeInsets.symmetric(vertical: 12),
                    child: Text(widget.text, style: style),
                  ),
                ),
              ],
            ),
          ),
          Positioned(
            right: -3,
            top: 0,
            bottom: 0,
            width: 44,
            child: Center(
              child: IconButton(
                onPressed: () => setState(() => _visible = false),
                tooltip: 'Dismiss',
                padding: EdgeInsets.zero,
                constraints: const BoxConstraints(minWidth: 44, minHeight: 44),
                icon: const SvgIcon(AppAssets.close, size: 14),
              ),
            ),
          ),
        ],
      ),
    );
  }
}

/// Large action button used on the 339 frames (bold 16px, 10px radius).
class StatesButton extends StatelessWidget {
  const StatesButton({super.key, required this.label, required this.onPressed, this.secondary = false});

  final String label;
  final VoidCallback? onPressed;
  final bool secondary;

  @override
  Widget build(BuildContext context) {
    final fill = secondary ? const Color(0xFFF7F7F8) : AppColors.strongBlue;
    final text = secondary ? const Color(0xFF001AFF) : Colors.white;
    return SizedBox(
      height: 48,
      child: Material(
        color: fill,
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(10),
          side: secondary ? const BorderSide(color: Color(0xFF0E0779)) : BorderSide.none,
        ),
        child: InkWell(
          borderRadius: BorderRadius.circular(10),
          onTap: onPressed,
          child: Center(child: figmaText(label, size: 16, weight: FontWeight.w700, color: text, lineHeight: 23)),
        ),
      ),
    );
  }
}
