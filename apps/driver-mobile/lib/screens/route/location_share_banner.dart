import 'package:flutter/material.dart';

import '../../location/location_reporter.dart';
import '../../theme/tokens.dart';
import '../../widgets/app_buttons.dart';
import '../../widgets/note_banner.dart';

/// What the driver sees about sharing the truck's position. Nothing while no trip is running; an explanation and
/// a choice before the phone's own permission prompt; a plain "on" note while sharing; and why it is off when it
/// is. The trip works the same whichever the driver picks.
class LocationShareBanner extends StatelessWidget {
  const LocationShareBanner({super.key, required this.reporter});

  final LocationReporter reporter;

  @override
  Widget build(BuildContext context) {
    return ListenableBuilder(
      listenable: reporter,
      builder: (context, _) {
        switch (reporter.state) {
          case LocationShareState.off:
            return const SizedBox.shrink();
          case LocationShareState.askFirst:
            return Padding(
              padding: const EdgeInsets.only(bottom: 15),
              child: _AskCard(onAllow: reporter.allow, onDecline: reporter.decline),
            );
          case LocationShareState.sharing:
            return const Padding(
              padding: EdgeInsets.only(bottom: 15),
              child: NoteBanner(
                title: 'Sharing your location',
                text: 'Dispatch can see where the truck is until you finish this trip.',
                tone: NoteTone.success,
              ),
            );
          case LocationShareState.declined:
            return const Padding(
              padding: EdgeInsets.only(bottom: 15),
              child: NoteBanner(
                title: 'Location is not shared',
                text: 'Dispatch only sees the stops you complete. Your trip works the same.',
                tone: NoteTone.offline,
              ),
            );
          case LocationShareState.blocked:
            return const Padding(
              padding: EdgeInsets.only(bottom: 15),
              child: NoteBanner(
                title: 'Location is off on this phone',
                text: 'Turn on location for Waypoint Driver in the phone settings to share the truck\'s position. Your trip works the same.',
                tone: NoteTone.offline,
              ),
            );
        }
      },
    );
  }
}

class _AskCard extends StatelessWidget {
  const _AskCard({required this.onAllow, required this.onDecline});

  final Future<void> Function() onAllow;
  final VoidCallback onDecline;

  @override
  Widget build(BuildContext context) {
    final body = AppText.of(13, FontWeight.w400, color: Colors.black, height: 19 / 13);
    return Semantics(
      container: true,
      liveRegion: true,
      child: Container(
        padding: const EdgeInsets.all(14),
        decoration: BoxDecoration(color: const Color(0x0F0400FF), borderRadius: BorderRadius.circular(AppRadius.r8)),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text('Share your location with dispatch?', style: AppText.of(15, FontWeight.w700, color: Colors.black, height: 1.35)),
            const SizedBox(height: 6),
            Text(
              'While this trip is running, your phone sends the truck\'s position to dispatch so they can see where it is and how far it has driven. '
              'It stops when you finish the trip or sign out. Nothing is shared before or after.',
              style: body,
            ),
            const SizedBox(height: 12),
            AppButton(label: 'Share my location', onPressed: () => onAllow(), strong: true, height: 44),
            const SizedBox(height: 8),
            AppButton(label: 'Not now', onPressed: onDecline, outline: true, strong: true, height: 44),
          ],
        ),
      ),
    );
  }
}
