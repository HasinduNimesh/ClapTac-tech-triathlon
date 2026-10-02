import 'package:flutter/material.dart';

import '../../theme/assets.dart';
import '../../theme/tokens.dart';
import '../../widgets/svg_icon.dart';
import 'delivery_assets.dart';

/// Text sizes in the Figma stop frames go down to 10px; keep them legible.
double _legible(double size) => size < 11 ? 11 : size;

TextStyle deliveryText(double size, FontWeight weight, {Color color = Colors.black}) =>
    AppText.of(_legible(size), weight, color: color, height: 1.45);

/// Section heading used on the stop frames ("Delivery outcome", ...).
class SectionHeading extends StatelessWidget {
  const SectionHeading(this.text, {super.key, this.suffix});

  final String text;
  final String? suffix;

  @override
  Widget build(BuildContext context) {
    return Semantics(
      header: true,
      child: Text.rich(
        TextSpan(
          text: text,
          style: deliveryText(13, FontWeight.w700),
          children: [if (suffix != null) TextSpan(text: ' $suffix', style: deliveryText(11, FontWeight.w400))],
        ),
      ),
    );
  }
}

/// Bordered card with the neutral stop-frame styling.
class StopCard extends StatelessWidget {
  const StopCard({
    super.key,
    required this.child,
    this.fill = DeliveryColors.cardFill,
    this.border = AppColors.separator,
    this.minHeight = 55,
    this.padding = const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
    this.onTap,
    this.semanticLabel,
  });

  final Widget child;
  final Color fill;
  final Color border;
  final double minHeight;
  final EdgeInsets padding;
  final VoidCallback? onTap;
  final String? semanticLabel;

  @override
  Widget build(BuildContext context) {
    final radius = BorderRadius.circular(AppRadius.r8);
    Widget card = Material(
      color: fill,
      shape: RoundedRectangleBorder(borderRadius: radius, side: BorderSide(color: border)),
      child: InkWell(
        borderRadius: radius,
        onTap: onTap,
        child: ConstrainedBox(constraints: BoxConstraints(minHeight: minHeight), child: Padding(padding: padding, child: child)),
      ),
    );
    if (semanticLabel != null) {
      card = Semantics(button: onTap != null, label: semanticLabel, excludeSemantics: true, onTap: onTap, child: card);
    }
    return card;
  }
}

/// Outlet / access / goods summary row: illustration, title, caption, trailing.
class SummaryRow extends StatelessWidget {
  const SummaryRow({
    super.key,
    required this.image,
    required this.imageWidth,
    required this.imageHeight,
    required this.title,
    required this.caption,
    this.trailing,
  });

  final String image;
  final double imageWidth;
  final double imageHeight;
  final String title;
  final String caption;
  final Widget? trailing;

  @override
  Widget build(BuildContext context) {
    return Row(
      children: [
        SizedBox(
          width: 78,
          child: Align(
            alignment: Alignment.centerLeft,
            child: Image.asset(image, width: imageWidth, height: imageHeight, fit: BoxFit.contain, excludeFromSemantics: true),
          ),
        ),
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            mainAxisSize: MainAxisSize.min,
            children: [
              Text(title, style: deliveryText(11, FontWeight.w600)),
              Text(caption, style: deliveryText(10, FontWeight.w400)),
            ],
          ),
        ),
        if (trailing != null) trailing!,
      ],
    );
  }
}

/// One of the four outcome choices on the stop frames (Delivered / Partial / ...).
class OutcomeOption extends StatelessWidget {
  const OutcomeOption({super.key, required this.label, required this.selected, required this.onTap});

  final String label;
  final bool selected;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final radius = BorderRadius.circular(AppRadius.r8);
    return Semantics(
      inMutuallyExclusiveGroup: true,
      checked: selected,
      button: true,
      label: label,
      excludeSemantics: true,
      onTap: onTap,
      child: Material(
        color: selected ? AppColors.strongBlue : DeliveryColors.cardFill,
        shape: RoundedRectangleBorder(borderRadius: radius, side: BorderSide(color: selected ? AppColors.strongBlue : AppColors.separator)),
        child: InkWell(
          borderRadius: radius,
          onTap: onTap,
          child: ConstrainedBox(
            constraints: const BoxConstraints(minHeight: 44),
            child: Padding(
              padding: const EdgeInsets.only(left: 14, right: 8),
              child: Row(
                children: [
                  SizedBox(
                    width: 24,
                    height: 24,
                    child: Center(
                      child: selected ? const SvgIcon(AppAssets.outcomeCheck, size: 24) : const SvgIcon(AppAssets.radioEmpty, size: 19),
                    ),
                  ),
                  const SizedBox(width: 11),
                  Expanded(child: Text(label, style: deliveryText(12, FontWeight.w600, color: selected ? Colors.white : Colors.black))),
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }
}

/// Illustration + label button ("Take photo", "Add signature").
class ProofButton extends StatelessWidget {
  const ProofButton({
    super.key,
    required this.image,
    required this.imageWidth,
    required this.imageHeight,
    required this.label,
    required this.capturedLabel,
    required this.captured,
    required this.onTap,
  });

  final String image;
  final double imageWidth;
  final double imageHeight;
  final String label;
  final String capturedLabel;
  final bool captured;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return StopCard(
      minHeight: 44,
      padding: const EdgeInsets.symmetric(horizontal: 8),
      fill: captured ? AppColors.greenBg : DeliveryColors.cardFill,
      border: captured ? AppColors.green : AppColors.separator,
      onTap: onTap,
      semanticLabel: captured ? capturedLabel : label,
      child: Row(
        children: [
          SizedBox(width: 56, child: Center(child: Image.asset(image, width: imageWidth, height: imageHeight, fit: BoxFit.contain, excludeFromSemantics: true))),
          const SizedBox(width: 4),
          Expanded(child: Text(captured ? capturedLabel : label, style: deliveryText(12, FontWeight.w600), maxLines: 2)),
          if (captured) const Padding(padding: EdgeInsets.only(right: 4), child: SvgIcon(AppAssets.outcomeCheck, size: 20, color: AppColors.green)),
        ],
      ),
    );
  }
}

/// "Photo saved locally" row shown once a photo is stored on the phone.
class PhotoSavedRow extends StatelessWidget {
  const PhotoSavedRow({super.key, required this.onTap});

  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return StopCard(
      minHeight: 55,
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
      onTap: onTap,
      semanticLabel: 'Photo saved locally. Tap to remove.',
      child: Row(
        children: [
          Container(
            width: 46,
            height: 46,
            decoration: BoxDecoration(color: const Color(0xFFD3D7DE), borderRadius: BorderRadius.circular(AppRadius.r6)),
            child: Image.asset(DeliveryAssets.photoThumb, fit: BoxFit.cover, excludeFromSemantics: true),
          ),
          const SizedBox(width: 14),
          const Stack(
            alignment: Alignment.center,
            children: [
              SvgIcon(DeliveryAssets.checkCircle, size: 30),
              SvgIcon(DeliveryAssets.checkMark, size: 18),
            ],
          ),
          const SizedBox(width: 8),
          Expanded(child: Text('Photo saved locally', style: deliveryText(11, FontWeight.w600))),
        ],
      ),
    );
  }
}
