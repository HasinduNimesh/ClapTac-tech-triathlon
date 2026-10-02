import 'package:flutter/material.dart';

/// Assets and colours that only the delivery-recording screens use.
class DeliveryAssets {
  static const wifiOff = 'assets/icons/delivery_wifi_off.svg';
  static const photoThumb = 'assets/images/delivery_photo_thumb.png';
  static const checkCircle = 'assets/icons/delivery_check_circle_bg.svg';
  static const checkMark = 'assets/icons/delivery_check_mark.svg';
  static const warningAmber = 'assets/icons/delivery_warning_amber.svg';
  static const radioSelected = 'assets/icons/delivery_radio_selected.svg';
  static const radioEmpty = 'assets/icons/delivery_radio_empty.svg';
}

class DeliveryColors {
  /// Outlet summary card (blue tint with a blue outline).
  static const outletFill = Color(0x300077FF);
  static const outletBorder = Color(0xFF0051FF);

  /// Neutral list cards on the stop frames.
  static const cardFill = Color(0x30FFFFFF);

  static const issueFill = Color(0x30FFACAC);
  static const issueBorder = Color(0xFFFF7678);

  static const offlineBanner = Color(0x66D7AC4F);
  static const parkedNote = Color(0x4FF3B750);

  static const sheetInk = Color(0xFF121726);
  static const sheetMuted = Color(0xFF636B7A);
  static const sheetBorder = Color(0xFFDEE3EB);
  static const sheetDangerBg = Color(0xFFFEF0F0);
  static const sheetDanger = Color(0xFFB81C1C);
  static const sheetOptionBg = Color(0xFFEDF2FF);
  static const sheetCheck = Color(0xFF12783B);
}
