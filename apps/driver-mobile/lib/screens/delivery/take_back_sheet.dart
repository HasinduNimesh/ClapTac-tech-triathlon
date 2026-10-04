import 'package:flutter/material.dart';

import '../../data/driver_models.dart';
import '../../theme/tokens.dart';
import '../../widgets/svg_icon.dart';
import 'delivery_assets.dart';

class TakeBackItem {
  const TakeBackItem({required this.name, this.quantity, this.unit = 'units', required this.note});

  final String name;

  /// Null when the server does not know the order's quantity.
  final int? quantity;
  final String unit;
  final String note;

  /// What goes back after a failed or rejected delivery: the stop's order, all of it. The server
  /// holds orders by quantity, not by product line, so the order is the item.
  factory TakeBackItem.forStop(StopInfo stop) => TakeBackItem(
        name: stop.orderRef.isNotEmpty ? 'Order ${stop.orderRef}' : 'This order',
        quantity: stop.units,
        unit: stop.unitLabel,
        note: stop.goods.toLowerCase().contains('chill') ? 'Keep in the chilled zone' : 'Goes back on this vehicle',
      );

  String get title => quantity == null ? '$name · taking back all of it' : '$name · taking back $quantity $unit';
}

/// The driver's reason in words, from the server's reason code (the code the app is about to send).
String takeBackReasonText(String? code) => switch (code) {
      'OUTLET_CLOSED' => 'the outlet was closed',
      'ACCESS_BLOCKED' => 'the driver could not get access',
      'RECEIVER_UNAVAILABLE' => 'nobody was available to receive it',
      'GOODS_REJECTED' => 'the goods were rejected',
      'VEHICLE_ISSUE' => 'a vehicle problem',
      _ => 'another reason',
    };

/// "Rejected: record what goes back" (Figma 412:778). [onSave] receives true
/// to re-attempt on tomorrow's first run, false to ask dispatch to defer.
Future<void> showTakeBackSheet(
  BuildContext context, {
  required StopInfo stop,
  required String orderRef,
  required ValueChanged<bool> onSave,
  required String reason,
  List<TakeBackItem>? items,
}) {
  return showModalBottomSheet<void>(
    context: context,
    isScrollControlled: true,
    useSafeArea: true,
    backgroundColor: Colors.transparent,
    barrierColor: const Color(0x730D121F),
    elevation: 0,
    builder: (sheetContext) => _TakeBackSheet(
      stop: stop,
      orderRef: orderRef,
      reason: reason,
      items: items ?? [TakeBackItem.forStop(stop)],
      onSave: (reattempt) {
        Navigator.of(sheetContext).pop();
        onSave(reattempt);
      },
      onBack: () => Navigator.of(sheetContext).pop(),
    ),
  );
}

class _TakeBackSheet extends StatefulWidget {
  const _TakeBackSheet({required this.stop, required this.orderRef, required this.reason, required this.items, required this.onSave, required this.onBack});

  final StopInfo stop;
  final String orderRef;
  final String reason;
  final List<TakeBackItem> items;
  final ValueChanged<bool> onSave;
  final VoidCallback onBack;

  @override
  State<_TakeBackSheet> createState() => _TakeBackSheetState();
}

class _TakeBackSheetState extends State<_TakeBackSheet> {
  bool _reattempt = true;

  TextStyle _t(double size, FontWeight weight, Color color) => AppText.of(size, weight, color: color, height: 1.4);

  @override
  Widget build(BuildContext context) {
    final keyboard = MediaQuery.viewInsetsOf(context).bottom;
    return Padding(
      padding: EdgeInsets.fromLTRB(16, 16, 16, 16 + keyboard),
      child: DecoratedBox(
        decoration: BoxDecoration(
          color: Colors.white,
          borderRadius: BorderRadius.circular(16),
          boxShadow: const [BoxShadow(color: Color(0x2E000000), blurRadius: 32, offset: Offset(0, 8))],
        ),
        child: Material(
          type: MaterialType.transparency,
          child: SingleChildScrollView(
            padding: const EdgeInsets.all(20),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                Semantics(header: true, child: Text('Rejected: record what goes back', style: _t(19, FontWeight.w700, DeliveryColors.sheetInk))),
                const SizedBox(height: 10),
                Text('Stop ${widget.stop.sequence} · ${widget.stop.name} · ${widget.orderRef}', style: _t(13, FontWeight.w400, DeliveryColors.sheetMuted)),
                const SizedBox(height: 10),
                Container(
                  padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
                  decoration: BoxDecoration(color: DeliveryColors.sheetDangerBg, borderRadius: BorderRadius.circular(8)),
                  child: Text('Reason: ${widget.reason}', style: _t(13, FontWeight.w500, DeliveryColors.sheetDanger)),
                ),
                for (final item in widget.items) ...[
                  const SizedBox(height: 10),
                  _CheckLine(title: item.title, caption: item.note),
                ],
                const SizedBox(height: 10),
                Semantics(header: true, child: Text('What happens next', style: _t(15, FontWeight.w600, DeliveryColors.sheetInk))),
                const SizedBox(height: 10),
                _Choice(
                  title: "Re-attempt on tomorrow's first run",
                  caption: 'Dispatcher confirms before the plan locks',
                  selected: _reattempt,
                  onTap: () => setState(() => _reattempt = true),
                ),
                const SizedBox(height: 10),
                _Choice(
                  title: 'Ask dispatcher to defer',
                  caption: 'The store gets the reason and the next run',
                  selected: !_reattempt,
                  onTap: () => setState(() => _reattempt = false),
                ),
                const SizedBox(height: 10),
                const _CheckLine(title: 'Photo of goods on the truck · store staff name'),
                const SizedBox(height: 10),
                _SheetButton(label: 'Save take-back offline', filled: true, onTap: () => widget.onSave(_reattempt)),
                const SizedBox(height: 10),
                _SheetButton(label: 'Back', filled: false, onTap: widget.onBack),
              ],
            ),
          ),
        ),
      ),
    );
  }
}

class _CheckLine extends StatelessWidget {
  const _CheckLine({required this.title, this.caption});

  final String title;
  final String? caption;

  @override
  Widget build(BuildContext context) {
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        ExcludeSemantics(child: Text('✓', style: AppText.of(14, FontWeight.w700, color: DeliveryColors.sheetCheck, height: 1.4))),
        const SizedBox(width: 10),
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(title, style: AppText.of(14, FontWeight.w500, color: DeliveryColors.sheetInk, height: 1.4)),
              if (caption != null) ...[
                const SizedBox(height: 2),
                Text(caption!, style: AppText.of(12, FontWeight.w400, color: DeliveryColors.sheetMuted, height: 1.4)),
              ],
            ],
          ),
        ),
      ],
    );
  }
}

class _Choice extends StatelessWidget {
  const _Choice({required this.title, required this.caption, required this.selected, required this.onTap});

  final String title;
  final String caption;
  final bool selected;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final radius = BorderRadius.circular(10);
    return Semantics(
      inMutuallyExclusiveGroup: true,
      checked: selected,
      button: true,
      label: '$title. $caption',
      excludeSemantics: true,
      onTap: onTap,
      child: Material(
        color: selected ? DeliveryColors.sheetOptionBg : Colors.white,
        shape: RoundedRectangleBorder(
          borderRadius: radius,
          side: BorderSide(color: selected ? AppColors.primary : DeliveryColors.sheetBorder, width: selected ? 1.5 : 1),
        ),
        child: InkWell(
          borderRadius: radius,
          onTap: onTap,
          child: Padding(
            padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 12),
            child: Row(
              children: [
                SvgIcon(selected ? DeliveryAssets.radioSelected : DeliveryAssets.radioEmpty, size: 18),
                const SizedBox(width: 12),
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(title, style: AppText.of(15, FontWeight.w600, color: DeliveryColors.sheetInk, height: 1.4)),
                      const SizedBox(height: 2),
                      Text(caption, style: AppText.of(13, FontWeight.w400, color: DeliveryColors.sheetMuted, height: 1.4)),
                    ],
                  ),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}

class _SheetButton extends StatelessWidget {
  const _SheetButton({required this.label, required this.filled, required this.onTap});

  final String label;
  final bool filled;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return Material(
      color: filled ? AppColors.primary : Colors.white,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(6),
        side: filled ? BorderSide.none : const BorderSide(color: DeliveryColors.sheetBorder),
      ),
      child: InkWell(
        borderRadius: BorderRadius.circular(6),
        onTap: onTap,
        child: ConstrainedBox(
          constraints: const BoxConstraints(minHeight: 48),
          child: Center(child: Text(label, style: AppText.of(15, FontWeight.w500, color: filled ? Colors.white : DeliveryColors.sheetInk, height: 1.4))),
        ),
      ),
    );
  }
}
