import 'package:flutter/material.dart';

import '../../data/driver_models.dart';
import '../../theme/assets.dart';
import '../../theme/tokens.dart';
import '../../widgets/app_buttons.dart';
import '../../widgets/driver_shell.dart';
import '../../widgets/note_banner.dart';
import '../../widgets/svg_icon.dart';
import 'delivery_assets.dart';
import 'delivery_widgets.dart';

/// Stop details with the delivery outcome form (Figma 339:3529 and the
/// offline variant 339:3796).
class StopDetailsScreen extends StatefulWidget {
  const StopDetailsScreen({
    super.key,
    required this.stop,
    required this.onSave,
    this.offline = false,
    this.onReportIssue,
    this.onProofRequested,
    this.onTabSelected,
  });

  final StopInfo stop;
  final ValueChanged<DeliveryDraft> onSave;
  final bool offline;
  final VoidCallback? onReportIssue;

  /// When set, the proof buttons call this instead of toggling a "captured" state. Used while real
  /// photo and signature capture is not available, so nothing pretends to be stored.
  final ValueChanged<ProofKind>? onProofRequested;
  final ValueChanged<DriverTab>? onTabSelected;

  @override
  State<StopDetailsScreen> createState() => _StopDetailsScreenState();
}

class _StopDetailsScreenState extends State<StopDetailsScreen> {
  final _notes = TextEditingController();
  DeliveryOutcome _outcome = DeliveryOutcome.delivered;
  bool _hasPhoto = false;
  bool _hasSignature = false;
  bool _parkedNoteVisible = true;
  bool _uploadNoteVisible = true;

  static const _labels = {
    DeliveryOutcome.delivered: 'Delivered',
    DeliveryOutcome.partial: 'Partial',
    DeliveryOutcome.failed: 'Failed',
    DeliveryOutcome.refused: 'Refused',
  };

  @override
  void dispose() {
    _notes.dispose();
    super.dispose();
  }

  void _save() {
    widget.onSave(
      DeliveryDraft(
        outcome: _outcome,
        quantity: _outcome == DeliveryOutcome.delivered ? widget.stop.units : null,
        notes: _notes.text.trim(),
        hasPhoto: _hasPhoto,
        hasSignature: _hasSignature,
      ),
    );
  }

  Widget _outcomeGrid() {
    Widget option(DeliveryOutcome value) => Expanded(
          child: OutcomeOption(label: _labels[value]!, selected: _outcome == value, onTap: () => setState(() => _outcome = value)),
        );
    return Column(
      children: [
        Row(children: [option(DeliveryOutcome.delivered), const SizedBox(width: 11), option(DeliveryOutcome.partial)]),
        const SizedBox(height: 3),
        Row(children: [option(DeliveryOutcome.failed), const SizedBox(width: 11), option(DeliveryOutcome.refused)]),
      ],
    );
  }

  Widget _proof() {
    final photoButton = ProofButton(
      image: AppAssets.camera,
      imageWidth: 41,
      imageHeight: 35,
      label: 'Take photo',
      capturedLabel: 'Photo saved',
      captured: _hasPhoto,
      onTap: widget.onProofRequested == null ? () => setState(() => _hasPhoto = !_hasPhoto) : () => widget.onProofRequested!(ProofKind.photo),
    );
    final signatureButton = ProofButton(
      image: AppAssets.signature,
      imageWidth: 52,
      imageHeight: 26,
      label: 'Add signature',
      capturedLabel: 'Signature added',
      captured: _hasSignature,
      onTap: widget.onProofRequested == null ? () => setState(() => _hasSignature = !_hasSignature) : () => widget.onProofRequested!(ProofKind.signature),
    );
    if (widget.offline && _hasPhoto) {
      return Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [PhotoSavedRow(onTap: () => setState(() => _hasPhoto = false)), const SizedBox(height: 8), signatureButton],
      );
    }
    return Row(children: [Expanded(child: photoButton), const SizedBox(width: 11), Expanded(child: signatureButton)]);
  }

  @override
  Widget build(BuildContext context) {
    final stop = widget.stop;
    final offline = widget.offline;
    return DriverShell(
      onTabSelected: widget.onTabSelected,
      bodyPadding: const EdgeInsets.fromLTRB(24, 20, 24, 16),
      bottomAction: AppButton(
        label: offline ? 'Save on this device' : 'Save delivery',
        onPressed: _save,
        strong: true,
        radius: AppRadius.r8 + 2,
        weight: FontWeight.w700,
      ),
      body: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          if (offline) ...[
            Semantics(
              liveRegion: true,
              child: Container(
                constraints: const BoxConstraints(minHeight: 47),
                padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
                decoration: BoxDecoration(
                  color: DeliveryColors.offlineBanner,
                  borderRadius: BorderRadius.circular(AppRadius.r8),
                  border: Border.all(color: const Color(0xFFF4FBF5)),
                ),
                child: Row(
                  children: [
                    const SvgIcon(DeliveryAssets.wifiOff, size: 27),
                    const SizedBox(width: 22),
                    Expanded(
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        mainAxisSize: MainAxisSize.min,
                        children: [
                          Text('No connection', style: deliveryText(12, FontWeight.w700)),
                          Text('You can still record this delivery', style: deliveryText(10, FontWeight.w400)),
                        ],
                      ),
                    ),
                  ],
                ),
              ),
            ),
            const SizedBox(height: 12),
          ],
          StopCard(
            fill: DeliveryColors.outletFill,
            border: DeliveryColors.outletBorder,
            minHeight: 91,
            padding: const EdgeInsets.symmetric(horizontal: 9, vertical: 8),
            child: Row(
              children: [
                SizedBox(width: 81, child: Image.asset(AppAssets.mapPin, width: 70, height: 67, fit: BoxFit.contain, excludeFromSemantics: true)),
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      Text('${stop.outletCode} - ${stop.name}', style: deliveryText(13, FontWeight.w700)),
                      const SizedBox(height: 4),
                      Text(stop.window, style: deliveryText(13, FontWeight.w400)),
                      Text(stop.unitsText, style: deliveryText(13, FontWeight.w400)),
                    ],
                  ),
                ),
              ],
            ),
          ),
          const SizedBox(height: 13),
          StopCard(
            padding: const EdgeInsets.fromLTRB(12, 0, 12, 0),
            child: SummaryRow(
              image: AppAssets.outletBuilding,
              imageWidth: 53,
              imageHeight: 53,
              title: stop.accessNote,
              caption: stop.contactNote,
              trailing: Semantics(
                label: 'Phone the store on arrival',
                child: const Padding(padding: EdgeInsets.only(right: 8), child: SvgIcon(AppAssets.phoneCall, size: 24)),
              ),
            ),
          ),
          const SizedBox(height: 13),
          StopCard(
            padding: const EdgeInsets.fromLTRB(12, 0, 12, 0),
            child: SummaryRow(
              image: AppAssets.cartonBox,
              imageWidth: 55,
              imageHeight: 46,
              title: stop.goods,
              caption: 'Expected: ${stop.units ?? 'not recorded'}',
            ),
          ),
          const SizedBox(height: 11),
          const SectionHeading('Delivery outcome'),
          const SizedBox(height: 8),
          _outcomeGrid(),
          const SizedBox(height: 10),
          SectionHeading(offline ? 'Photo proof' : 'Add delivery proof'),
          const SizedBox(height: 8),
          _proof(),
          if (!offline) ...[
            const SizedBox(height: 10),
            const SectionHeading('Report an issue', suffix: '(optional)'),
            const SizedBox(height: 8),
            StopCard(
              fill: DeliveryColors.issueFill,
              border: DeliveryColors.issueBorder,
              minHeight: 44,
              padding: const EdgeInsets.symmetric(horizontal: 14),
              onTap: widget.onReportIssue,
              semanticLabel: 'Report an issue',
              child: Row(
                children: [
                  const SvgIcon(AppAssets.alertTriangleRed, size: 24),
                  const SizedBox(width: 16),
                  Expanded(child: Text('Report an issue', style: deliveryText(12, FontWeight.w600))),
                  const SvgIcon(AppAssets.chevronRightRed, size: 24),
                ],
              ),
            ),
          ],
          const SizedBox(height: 10),
          const SectionHeading('Delivery notes'),
          const SizedBox(height: 8),
          TextField(
            controller: _notes,
            minLines: 3,
            maxLines: 4,
            textInputAction: TextInputAction.newline,
            style: deliveryText(12, FontWeight.w400),
            decoration: InputDecoration(
              hintText: 'Add notes (optional) ...',
              hintStyle: deliveryText(11, FontWeight.w400, color: const Color(0x80000000)),
              filled: true,
              fillColor: DeliveryColors.cardFill,
              contentPadding: const EdgeInsets.fromLTRB(12, 8, 12, 8),
              enabledBorder: OutlineInputBorder(borderRadius: BorderRadius.circular(AppRadius.r8), borderSide: const BorderSide(color: AppColors.separator)),
              focusedBorder: OutlineInputBorder(borderRadius: BorderRadius.circular(AppRadius.r8), borderSide: const BorderSide(color: AppColors.primary, width: 2)),
            ),
          ),
          if (offline && _uploadNoteVisible) ...[
            const SizedBox(height: 12),
            NoteBanner(
              text: 'Updates will be uploaded when you’re online.',
              icon: const SvgIcon(AppAssets.infoCircle, size: 24),
              textStyle: deliveryText(11, FontWeight.w400, color: AppColors.ink),
              onDismiss: () => setState(() => _uploadNoteVisible = false),
            ),
          ],
          if (_parkedNoteVisible) ...[
            const SizedBox(height: 10),
            offline
                ? NoteBanner(
                    text: 'Use only while parked.',
                    tone: NoteTone.amber,
                    icon: const SvgIcon(DeliveryAssets.warningAmber, size: 24),
                    textStyle: deliveryText(11, FontWeight.w400, color: AppColors.ink),
                    onDismiss: () => setState(() => _parkedNoteVisible = false),
                  )
                : NoteBanner(
                    text: 'Complete this form only while parked.',
                    icon: const SvgIcon(AppAssets.infoCircle, size: 24),
                    textStyle: deliveryText(11, FontWeight.w400, color: AppColors.ink),
                    onDismiss: () => setState(() => _parkedNoteVisible = false),
                  ),
          ],
        ],
      ),
    );
  }
}
