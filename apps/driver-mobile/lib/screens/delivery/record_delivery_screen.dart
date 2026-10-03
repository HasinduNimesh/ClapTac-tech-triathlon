import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../../data/driver_models.dart';
import '../../theme/assets.dart';
import '../../theme/tokens.dart';
import '../../widgets/app_buttons.dart';
import '../../widgets/driver_shell.dart';

/// "Record delivery" with outcome cards, quantity and proof (Figma 323:1375).
class RecordDeliveryScreen extends StatefulWidget {
  const RecordDeliveryScreen({
    super.key,
    required this.stop,
    required this.orderRef,
    required this.expectedQuantity,
    this.unit = 'cups',
    required this.onSave,
    this.onRejected,
    this.onTabSelected,
    this.initialOutcome = DeliveryOutcome.delivered,
    this.onProofRequested,
  });

  final StopInfo stop;
  final String orderRef;
  final int expectedQuantity;
  final String unit;
  final ValueChanged<DeliveryDraft> onSave;
  final VoidCallback? onRejected;
  final ValueChanged<DriverTab>? onTabSelected;

  /// Which outcome card starts selected (e.g. partial when arriving from stop details).
  final DeliveryOutcome initialOutcome;

  /// When set, the proof buttons call this instead of toggling a "captured" state.
  final ValueChanged<ProofKind>? onProofRequested;

  @override
  State<RecordDeliveryScreen> createState() => _RecordDeliveryScreenState();
}

enum _Choice { delivered, partial, rejected, unavailable }

class _RecordDeliveryScreenState extends State<RecordDeliveryScreen> {
  late final TextEditingController _quantity = TextEditingController(text: '${widget.expectedQuantity}');
  late _Choice _choice = switch (widget.initialOutcome) {
    DeliveryOutcome.delivered => _Choice.delivered,
    DeliveryOutcome.partial => _Choice.partial,
    DeliveryOutcome.refused => _Choice.rejected,
    DeliveryOutcome.failed => _Choice.unavailable,
  };
  bool _hasPhoto = false;
  bool _hasSignature = false;

  static const _titles = {
    _Choice.delivered: 'Delivered',
    _Choice.partial: 'Partial',
    _Choice.rejected: 'Rejected',
    _Choice.unavailable: 'Unavailable',
  };
  static const _subtitles = {
    _Choice.delivered: 'Full quantity received',
    _Choice.partial: 'Record exact quantity',
    _Choice.rejected: 'Store declined goods',
    _Choice.unavailable: 'Store closed / no access',
  };

  @override
  void dispose() {
    _quantity.dispose();
    super.dispose();
  }

  int? get _enteredQuantity => int.tryParse(_quantity.text);
  bool get _quantityEditable => _choice == _Choice.partial;
  bool get _valid {
    if (!_quantityEditable) return true;
    final q = _enteredQuantity;
    return q != null && q >= 0 && q <= widget.expectedQuantity;
  }

  int get _effectiveQuantity {
    switch (_choice) {
      case _Choice.delivered:
        return widget.expectedQuantity;
      case _Choice.partial:
        return (_enteredQuantity ?? 0).clamp(0, widget.expectedQuantity);
      case _Choice.rejected:
      case _Choice.unavailable:
        return 0;
    }
  }

  void _select(_Choice value) {
    setState(() {
      _choice = value;
      if (value == _Choice.delivered) {
        _quantity.text = '${widget.expectedQuantity}';
      } else if (value != _Choice.partial) {
        _quantity.text = '0';
      }
    });
    if (value == _Choice.rejected) widget.onRejected?.call();
  }

  DeliveryOutcome get _outcome {
    switch (_choice) {
      case _Choice.delivered:
        return DeliveryOutcome.delivered;
      case _Choice.partial:
        return DeliveryOutcome.partial;
      case _Choice.rejected:
        return DeliveryOutcome.refused;
      case _Choice.unavailable:
        return DeliveryOutcome.failed;
    }
  }

  void _save() {
    if (!_valid) return;
    widget.onSave(
      DeliveryDraft(
        outcome: _outcome,
        quantity: _effectiveQuantity,
        reason: switch (_choice) {
          _Choice.rejected => 'GOODS_REJECTED',
          _Choice.unavailable => 'OUTLET_CLOSED',
          _ => null,
        },
        hasPhoto: _hasPhoto,
        hasSignature: _hasSignature,
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final stop = widget.stop;
    final expected = widget.expectedQuantity;
    return DriverShell(
      onTabSelected: widget.onTabSelected,
      bodyPadding: const EdgeInsets.fromLTRB(14, 17, 14, 16),
      bottomAction: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          AppButton(label: 'Save delivery offline', onPressed: _valid ? _save : null, radius: AppRadius.r6, height: 44, weight: FontWeight.w500),
          const SizedBox(height: 6),
          Text('Only complete this form when the vehicle is stopped.', textAlign: TextAlign.center, style: AppText.of(12, FontWeight.w400, color: AppColors.muted)),
        ],
      ),
      body: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Padding(
            padding: const EdgeInsets.symmetric(horizontal: 4),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Semantics(header: true, child: Text('Record delivery', style: AppText.of(23, FontWeight.w700, height: 1.35))),
                Text(
                  '${[stop.outletCode, widget.orderRef].where((part) => part.isNotEmpty).join(' · ')} · $_effectiveQuantity of $expected received',
                  style: AppText.of(13, FontWeight.w400, color: AppColors.muted, height: 1.35),
                ),
              ],
            ),
          ),
          const SizedBox(height: 24),
          Align(
            alignment: Alignment.centerLeft,
            child: Padding(
              padding: const EdgeInsets.only(left: 6),
              child: Container(
                padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 3),
                decoration: BoxDecoration(color: AppColors.coolBg, borderRadius: BorderRadius.circular(13)),
                child: Text('SAVED ON THIS PHONE', style: AppText.of(12, FontWeight.w500, color: AppColors.cool, height: 1.35)),
              ),
            ),
          ),
          const SizedBox(height: 14),
          Container(
            padding: const EdgeInsets.fromLTRB(14, 16, 14, 16),
            decoration: BoxDecoration(
              color: AppColors.surface,
              borderRadius: BorderRadius.circular(AppRadius.r8),
              border: Border.all(color: AppColors.border),
            ),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Semantics(header: true, child: Text('Delivery outcome', style: AppText.of(17, FontWeight.w700, height: 1.35))),
                const SizedBox(height: 14),
                IntrinsicHeight(
                  child: Row(crossAxisAlignment: CrossAxisAlignment.stretch, children: [_option(_Choice.delivered), const SizedBox(width: 10), _option(_Choice.partial)]),
                ),
                const SizedBox(height: 12),
                IntrinsicHeight(
                  child: Row(crossAxisAlignment: CrossAxisAlignment.stretch, children: [_option(_Choice.rejected), const SizedBox(width: 10), _option(_Choice.unavailable)]),
                ),
              ],
            ),
          ),
          const SizedBox(height: 12),
          Padding(
            padding: const EdgeInsets.only(left: 8),
            child: Text('Quantity received', style: AppText.of(13, FontWeight.w500, height: 1.35)),
          ),
          const SizedBox(height: 8),
          Padding(
            padding: const EdgeInsets.symmetric(horizontal: 8),
            child: TextField(
              controller: _quantity,
              enabled: _quantityEditable,
              keyboardType: TextInputType.number,
              inputFormatters: [FilteringTextInputFormatter.digitsOnly, _MaxValueFormatter(expected)],
              onChanged: (_) => setState(() {}),
              style: AppText.of(14, FontWeight.w500, height: 1.35),
              decoration: InputDecoration(
                suffixText: widget.unit,
                suffixStyle: AppText.of(14, FontWeight.w500, height: 1.35),
                isDense: false,
                errorText: _quantityEditable && _enteredQuantity == null ? 'Enter a quantity from 0 to $expected' : null,
                helperText: _quantityEditable ? 'Expected $expected ${widget.unit}' : null,
                contentPadding: const EdgeInsets.symmetric(horizontal: 14, vertical: 12),
                enabledBorder: _border(AppColors.border),
                disabledBorder: _border(AppColors.border),
                focusedBorder: _border(AppColors.primary, width: 2),
                errorBorder: _border(AppColors.red),
                focusedErrorBorder: _border(AppColors.red, width: 2),
              ),
            ),
          ),
          const SizedBox(height: 14),
          Padding(
            padding: const EdgeInsets.only(left: 8),
            child: Semantics(header: true, child: Text('Proof of delivery', style: AppText.of(14, FontWeight.w700, height: 1.35))),
          ),
          const SizedBox(height: 10),
          Padding(
            padding: const EdgeInsets.symmetric(horizontal: 8),
            child: Row(
              children: [
                Expanded(
                  child: _ProofTile(
                    image: AppAssets.camera,
                    imageWidth: 30,
                    imageHeight: 26,
                    label: 'Add photo',
                    doneLabel: 'Photo added',
                    done: _hasPhoto,
                    onTap: widget.onProofRequested == null ? () => setState(() => _hasPhoto = !_hasPhoto) : () => widget.onProofRequested!(ProofKind.photo),
                  ),
                ),
                const SizedBox(width: 14),
                Expanded(
                  child: _ProofTile(
                    image: AppAssets.signature,
                    imageWidth: 36,
                    imageHeight: 18,
                    label: 'Name / signature',
                    doneLabel: 'Signature added',
                    done: _hasSignature,
                    onTap: widget.onProofRequested == null ? () => setState(() => _hasSignature = !_hasSignature) : () => widget.onProofRequested!(ProofKind.signature),
                  ),
                ),
              ],
            ),
          ),
          const SizedBox(height: 16),
          Padding(
            padding: const EdgeInsets.symmetric(horizontal: 8),
            child: Container(
              padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 14),
              decoration: BoxDecoration(color: AppColors.coolBg, borderRadius: BorderRadius.circular(AppRadius.r6)),
              child: Text('Saved locally first · uploads when connected', style: AppText.of(12, FontWeight.w500, color: AppColors.cool, height: 1.35)),
            ),
          ),
        ],
      ),
    );
  }

  OutlineInputBorder _border(Color color, {double width = 1}) =>
      OutlineInputBorder(borderRadius: BorderRadius.circular(AppRadius.r6), borderSide: BorderSide(color: color, width: width));

  Widget _option(_Choice value) {
    final selected = _choice == value;
    return Expanded(
      child: Semantics(
        inMutuallyExclusiveGroup: true,
        checked: selected,
        button: true,
        label: '${_titles[value]}. ${_subtitles[value]}',
        excludeSemantics: true,
        onTap: () => _select(value),
        child: Material(
          color: selected ? AppColors.tint : AppColors.surface,
          shape: RoundedRectangleBorder(
            borderRadius: BorderRadius.circular(7),
            side: BorderSide(color: selected ? AppColors.primary : AppColors.border),
          ),
          child: InkWell(
            borderRadius: BorderRadius.circular(7),
            onTap: () => _select(value),
            child: ConstrainedBox(
              constraints: const BoxConstraints(minHeight: 58),
              child: Padding(
                padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 8),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  mainAxisAlignment: MainAxisAlignment.center,
                  children: [
                    Text(_titles[value]!, style: AppText.of(13, FontWeight.w700, color: selected ? AppColors.primary : AppColors.ink, height: 1.35)),
                    Text(_subtitles[value]!, style: AppText.of(11, FontWeight.w400, color: AppColors.muted, height: 1.35)),
                  ],
                ),
              ),
            ),
          ),
        ),
      ),
    );
  }
}

class _ProofTile extends StatelessWidget {
  const _ProofTile({
    required this.image,
    required this.imageWidth,
    required this.imageHeight,
    required this.label,
    required this.doneLabel,
    required this.done,
    required this.onTap,
  });

  final String image;
  final double imageWidth;
  final double imageHeight;
  final String label;
  final String doneLabel;
  final bool done;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return Semantics(
      button: true,
      toggled: done,
      label: done ? doneLabel : label,
      excludeSemantics: true,
      onTap: onTap,
      child: Material(
        color: done ? AppColors.greenBg : AppColors.surface,
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(7),
          side: BorderSide(color: done ? AppColors.green : AppColors.border),
        ),
        child: InkWell(
          borderRadius: BorderRadius.circular(7),
          onTap: onTap,
          child: ConstrainedBox(
            constraints: const BoxConstraints(minHeight: 56),
            child: Padding(
              padding: const EdgeInsets.symmetric(horizontal: 8),
              child: Row(
                children: [
                  SizedBox(width: 38, child: Center(child: Image.asset(image, width: imageWidth, height: imageHeight, fit: BoxFit.contain, excludeFromSemantics: true))),
                  const SizedBox(width: 4),
                  Expanded(
                    child: Text(
                      done ? doneLabel : label,
                      style: AppText.of(12, FontWeight.w500, color: done ? AppColors.green : AppColors.primary, height: 1.35),
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

/// Clamps typed digits to the expected quantity instead of rejecting them.
class _MaxValueFormatter extends TextInputFormatter {
  _MaxValueFormatter(this.max);

  final int max;

  @override
  TextEditingValue formatEditUpdate(TextEditingValue oldValue, TextEditingValue newValue) {
    final value = int.tryParse(newValue.text);
    if (value == null || value <= max) return newValue;
    final text = '$max';
    return TextEditingValue(text: text, selection: TextSelection.collapsed(offset: text.length));
  }
}
