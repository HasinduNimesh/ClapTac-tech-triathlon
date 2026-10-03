import 'package:flutter/material.dart';

import '../../data/driver_models.dart';
import '../../theme/assets.dart';
import '../../theme/tokens.dart';
import '../../widgets/driver_shell.dart';
import '../../widgets/note_banner.dart';
import '../../widgets/svg_icon.dart';
import 'states_assets.dart';
import 'states_widgets.dart';

/// One screen for the four sync states in the Figma file: saved offline,
/// syncing, synced and upload failed.
class SyncStateScreen extends StatelessWidget {
  const SyncStateScreen({
    super.key,
    required this.kind,
    required this.stop,
    this.savedAt = '08:42',
    this.uploadProgress = 0.75,
    this.outcomeText,
    this.hasPhoto = true,
    this.onPrimary,
    this.onSecondary,
    this.onTabSelected,
  });

  final SyncStateKind kind;
  final StopInfo stop;
  final String savedAt;

  /// 0..1, only shown while [kind] is [SyncStateKind.syncing].
  final double uploadProgress;

  /// What was recorded, e.g. "Refused - 12 units taken back". Defaults to the delivered sample.
  final String? outcomeText;

  /// Whether a photo was stored with the outcome. Without one the screen does not mention a photo.
  final bool hasPhoto;
  final VoidCallback? onPrimary;
  final VoidCallback? onSecondary;
  final ValueChanged<DriverTab>? onTabSelected;

  @override
  Widget build(BuildContext context) {
    return DriverShell(
      tab: DriverTab.route,
      onTabSelected: onTabSelected,
      bodyPadding: const EdgeInsets.fromLTRB(AppSpace.s24, 20, AppSpace.s24, AppSpace.s24),
      body: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: _children(),
      ),
    );
  }

  List<Widget> _children() {
    switch (kind) {
      case SyncStateKind.savedOffline:
        return [
          StatusBanner(
            color: tinted(215, 172, 79, 0.4),
            icon: StatesAssets.wifiOff,
            iconSize: 33.25,
            title: 'Saved on this phone',
            subtitle: 'Check Updates for its sync status.',
          ),
          const SizedBox(height: 13),
          const StateIllustration(circle: StatesAssets.circleNeutral, glyphs: [
            GlyphSpec(StatesAssets.savedPhone, 72, 97, 0, -0.5),
            GlyphSpec(StatesAssets.savedCheck, 29, 21.29, -1.5, -2.4),
          ]),
          const SizedBox(height: 10),
          _heading('Saved on this device'),
          _description(hasPhoto
              ? 'Delivery outcome and photo are stored on this phone. Check Updates for their sync status.'
              : 'The delivery outcome is stored on this phone. Check Updates for its sync status.'),
          _SyncCard(
            color: tinted(240, 214, 147, 0.19),
            border: const Color(0xFFFBFCFF),
            stop: stop,
            rows: [
              _CardRow.check(outcomeText ?? 'Delivered - ${stop.unitsText}', top: 52),
              _CardRow.icon(StatesAssets.rowClock, 'Saved at $savedAt', top: 80),
              _CardRow.icon(StatesAssets.rowPhoto, hasPhoto ? 'Pending: Outcome + 1 photo' : 'Pending: Outcome', top: 108),
            ],
          ),
          const SizedBox(height: 46),
          StatesButton(label: 'Back to route', onPressed: onPrimary),
          const SizedBox(height: 47),
          const DismissibleNote(text: 'Follow the planned stop order.'),
        ];
      case SyncStateKind.syncing:
        final percent = (uploadProgress.clamp(0.0, 1.0) * 100).round();
        return [
          StatusBanner(
            color: tinted(0, 38, 255, 0.3),
            icon: StatesAssets.wifiRestored,
            iconSize: 30,
            title: 'Connection restored',
            subtitle: 'You’re back online.',
          ),
          const SizedBox(height: 13),
          const StateIllustration(circle: StatesAssets.circleNeutral, glyphs: [
            GlyphSpec(StatesAssets.cloudUpload, 60, 60, 2, 0),
          ]),
          const SizedBox(height: 10),
          _heading('Syncing'),
          _description('Uploading your saved delivery update', textTop: 7),
          _SyncCard(
            color: tinted(147, 191, 240, 0.19),
            border: const Color(0xFFFCFDFF),
            stop: stop,
            rows: [
              _CardRow.check('Delivery outcome uploaded', top: 53),
              _CardRow.icon(StatesAssets.rowCloudUpload, null, top: 80, iconCenterY: 98),
            ],
            extras: [
              Positioned(
                left: 65,
                top: 80,
                right: 36,
                child: _UploadProgress(percent: percent, progress: uploadProgress),
              ),
            ],
          ),
          const SizedBox(height: 8),
          figmaText('You can return to your route while this uploads.', lineHeight: 19),
          const SizedBox(height: 9),
          NoteBanner(
            text: '1 upload remaining',
            icon: const SvgIcon(StatesAssets.uploadChip, size: 24),
            textStyle: AppText.of(13, FontWeight.w600, color: const Color(0xFF0052FF), height: 19 / 13),
          ),
          const SizedBox(height: 23),
          StatesButton(label: 'Back to route', onPressed: onPrimary),
        ];
      case SyncStateKind.synced:
        return [
          StatusBanner(
            color: tinted(76, 243, 110, 0.31),
            icon: StatesAssets.wifiOnline,
            iconSize: 30,
            title: 'You’re online',
            subtitle: 'All updates synced.',
          ),
          const SizedBox(height: 13),
          const StateIllustration(circle: StatesAssets.circleSuccess, glyphs: [
            GlyphSpec(StatesAssets.syncedDisc, 80, 80, 0, -1),
            GlyphSpec(StatesAssets.syncedCheck, 47, 48, 0.5, -2),
          ]),
          const SizedBox(height: 10),
          _heading('Synced'),
          _description('The dispatcher has received your delivery outcome and photo'),
          _SyncCard(
            color: tinted(166, 238, 157, 0.19),
            border: Colors.white,
            stop: stop,
            rows: [
              _CardRow.check('Delivered - ${stop.unitsText}', top: 52),
              _CardRow.icon(StatesAssets.rowClock, 'Saved at $savedAt', top: 80),
              _CardRow.icon(StatesAssets.rowPhoto, 'Photo uploaded', top: 108),
            ],
          ),
          const SizedBox(height: 40),
          const _NoPendingNote(),
          const SizedBox(height: 18),
          StatesButton(label: 'Continue route', onPressed: onPrimary),
        ];
      case SyncStateKind.uploadFailed:
        return [
          StatusBanner(
            color: tinted(222, 160, 102, 0.39),
            icon: StatesAssets.alertTriangle,
            iconSize: 40,
            title: 'Upload needs attention',
            subtitle: 'Some items couldn’t be uploaded.',
          ),
          const SizedBox(height: 13),
          const StateIllustration(circle: StatesAssets.circleError, glyphs: [
            GlyphSpec(StatesAssets.cloudError, 60, 60, 2, 0),
            GlyphSpec(StatesAssets.errorStem, 4, 23, 1, 6.5),
            GlyphSpec(StatesAssets.errorDot, 4.16, 4, 1, 24),
          ]),
          const SizedBox(height: 10),
          _heading('Photo couldn’t be uploaded'),
          _description(
            'Your delivery outcome was synced, but the photo is still safely saved on this device.'),
          _SyncCard(
            color: tinted(217, 143, 103, 0.25),
            border: const Color(0xFFFEFEFF),
            stop: stop,
            rows: const [
              _CardRow.richCheck(top: 52, centerY: 66),
              _CardRow.failedPhoto(top: 89),
            ],
          ),
          const SizedBox(height: 17),
          StatesButton(label: 'Retry photo upload', onPressed: onPrimary),
          const SizedBox(height: 8),
          StatesButton(label: 'Back to route', onPressed: onSecondary, secondary: true),
          const SizedBox(height: 17),
          const DismissibleNote(
            text: 'Retry when you have a stable connection.\nDo not clear this browser’s data.',
            minHeight: 62,
          ),
        ];
    }
  }

  Widget _heading(String text) => figmaText(text, size: 15, weight: FontWeight.w700, lineHeight: 19, align: TextAlign.center);

  Widget _description(String text, {double textTop = 0}) {
    return SizedBox(
      height: 82,
      child: Padding(
        padding: EdgeInsets.only(top: textTop),
        child: Align(
          alignment: Alignment.topCenter,
          child: SizedBox(width: 233, child: figmaText(text, size: 12, lineHeight: 19, align: TextAlign.center)),
        ),
      ),
    );
  }
}

class _CardRow {
  const _CardRow._(this.kind, this.top, {this.icon, this.text, this.iconCenterY = 0});

  const _CardRow.richCheck({required double top, required double centerY}) : this._(_RowKind.richCheck, top, iconCenterY: centerY);
  const _CardRow.failedPhoto({required double top}) : this._(_RowKind.failedPhoto, top);
  factory _CardRow.check(String text, {required double top}) => _CardRow._(_RowKind.check, top, text: text);
  factory _CardRow.icon(String icon, String? text, {required double top, double? iconCenterY}) =>
      _CardRow._(_RowKind.icon, top, icon: icon, text: text, iconCenterY: iconCenterY ?? top + 7);

  final _RowKind kind;
  final double top;
  final String? icon;
  final String? text;
  final double iconCenterY;
}

enum _RowKind { check, icon, richCheck, failedPhoto }

/// "OUT108 - Dehiwala" card with the per-row status lines.
class _SyncCard extends StatelessWidget {
  const _SyncCard({required this.color, required this.border, required this.stop, required this.rows, this.extras = const []});

  final Color color;
  final Color border;
  final StopInfo stop;
  final List<_CardRow> rows;
  final List<Widget> extras;

  @override
  Widget build(BuildContext context) {
    final children = <Widget>[
      Positioned(left: 9, top: 8, width: 40, height: 38, child: Image.asset(AppAssets.mapPin, fit: BoxFit.cover, excludeFromSemantics: true)),
      Positioned(left: 65, top: 18, child: figmaText(stop.label, weight: FontWeight.w700, lineHeight: 19)),
    ];
    for (final row in rows) {
      children.addAll(_build(row));
    }
    children.addAll(extras);
    return StatesCard(height: 145, color: color, borderColor: border, children: children);
  }

  List<Widget> _build(_CardRow row) {
    switch (row.kind) {
      case _RowKind.check:
        return [
          const Positioned(left: 14 - 1, top: 46 - 1, child: _CheckBadge()),
          Positioned(left: 65, top: row.top - 1, child: figmaText(row.text!, lineHeight: 19)),
        ];
      case _RowKind.icon:
        return [
          Positioned(left: 29 - 1, top: row.iconCenterY - 1, child: FractionalTranslation(translation: const Offset(-0.5, -0.5), child: SvgIcon(row.icon!))),
          if (row.text != null) Positioned(left: 65, top: row.top - 1, child: figmaText(row.text!, lineHeight: 19)),
        ];
      case _RowKind.richCheck:
        return [
          const Positioned(left: 14 - 1, top: 51 - 1, child: _CheckBadge()),
          Positioned(
            left: 65,
            top: row.top - 1,
            child: Text.rich(
              TextSpan(
                style: AppText.of(13, FontWeight.w400, color: Colors.black, height: 19 / 13),
                children: [
                  const TextSpan(text: 'Delivery outcome - '),
                  TextSpan(text: 'Synced', style: AppText.of(13, FontWeight.w600, color: const Color(0xFF07691B), height: 19 / 13)),
                ],
              ),
            ),
          ),
        ];
      case _RowKind.failedPhoto:
        return [
          const Positioned(left: 14 - 1, top: 92 - 1, child: SvgIcon(StatesAssets.rowAlertRed)),
          Positioned(
            left: 65,
            top: row.top - 1,
            child: Text.rich(
              TextSpan(
                style: AppText.of(13, FontWeight.w400, color: Colors.black, height: 19 / 13),
                children: [
                  const TextSpan(text: 'Delivery photo - '),
                  TextSpan(text: 'Upload failed', style: AppText.of(13, FontWeight.w600, color: const Color(0xFFF80909), height: 19 / 13)),
                  const TextSpan(text: '\nConnection dropped during upload.'),
                ],
              ),
            ),
          ),
        ];
    }
  }
}

class _CheckBadge extends StatelessWidget {
  const _CheckBadge();

  @override
  Widget build(BuildContext context) {
    return const SizedBox(
      width: 30,
      height: 30,
      child: Stack(
        clipBehavior: Clip.none,
        children: [
          SvgIcon(StatesAssets.checkCircle),
          Positioned(left: 5, top: 6, child: SvgIcon(StatesAssets.checkGlyph)),
        ],
      ),
    );
  }
}

class _NoPendingNote extends StatelessWidget {
  const _NoPendingNote();

  @override
  Widget build(BuildContext context) {
    return Semantics(
      container: true,
      label: 'No pending updates',
      child: ExcludeSemantics(
        child: Container(
          height: 44,
          padding: const EdgeInsets.only(left: 15),
          decoration: BoxDecoration(color: const Color.fromRGBO(76, 255, 112, 0.21), borderRadius: BorderRadius.circular(AppRadius.r8)),
          child: Row(
            children: [
              const _CheckBadge(),
              const SizedBox(width: 11),
              figmaText('No pending updates', weight: FontWeight.w600, color: const Color(0xFF1D7D46), lineHeight: 19),
            ],
          ),
        ),
      ),
    );
  }
}

class _UploadProgress extends StatelessWidget {
  const _UploadProgress({required this.percent, required this.progress});

  final int percent;
  final double progress;

  @override
  Widget build(BuildContext context) {
    return Semantics(
      container: true,
      label: 'Uploading photo, 1 of 1',
      value: '$percent%',
      child: ExcludeSemantics(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Row(
              mainAxisAlignment: MainAxisAlignment.spaceBetween,
              children: [
                figmaText('Uploading photo - 1 of 1', lineHeight: 18.2),
                figmaText('$percent%', lineHeight: 18.2),
              ],
            ),
            const SizedBox(height: 8),
            ClipRRect(
              borderRadius: BorderRadius.circular(3),
              child: SizedBox(
                height: 6,
                child: Stack(
                  children: [
                    const Positioned.fill(child: ColoredBox(color: AppColors.border)),
                    FractionallySizedBox(
                      widthFactor: progress.clamp(0.0, 1.0),
                      child: const ColoredBox(color: AppColors.primary, child: SizedBox(height: 6)),
                    ),
                  ],
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}
