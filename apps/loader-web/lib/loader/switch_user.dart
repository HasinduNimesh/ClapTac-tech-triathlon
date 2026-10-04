import 'package:flutter/material.dart';

import 'loader_controller.dart';

/// "Switch user" (workflow W6): the dock tablet is shared, so this signs the current loader out
/// right away and returns to the sign-in screen for the next person.
///
/// The loader app is online-only and keeps nothing for later, so there is no offline queue to
/// protect. The one thing sign-out could lose is what the loader has started but not finished
/// sending (an open report form, an action still on its way); when there is some, it asks first
/// instead of discarding it silently. Anything already sent is on the server under their name.
Future<void> switchUser(BuildContext context, LoaderController controller, VoidCallback signOut) async {
  if (!controller.hasUnsentWork) {
    signOut();
    return;
  }
  final confirmed = await showDialog<bool>(
    context: context,
    builder: (ctx) => AlertDialog(
      title: const Text('Switch user?'),
      content: const Text(
        'Something you started has not finished sending: a report that is still open, or an action on its way to the server. '
        'The loader app keeps nothing for later, so switching user now discards it. '
        'Everything already sent stays on the server under your name.',
      ),
      actions: [
        OutlinedButton(onPressed: () => Navigator.pop(ctx, false), child: const Text('Stay signed in')),
        FilledButton(onPressed: () => Navigator.pop(ctx, true), child: const Text('Discard and switch user')),
      ],
    ),
  );
  if (confirmed == true) signOut();
}

/// The always-visible Switch user control for the tablet header.
class SwitchUserButton extends StatelessWidget {
  const SwitchUserButton({super.key, required this.controller, required this.onSignOut});

  final LoaderController controller;
  final VoidCallback onSignOut;

  @override
  Widget build(BuildContext context) {
    return Tooltip(
      message: 'Sign out and let the next loader sign in',
      child: OutlinedButton.icon(
        onPressed: () => switchUser(context, controller, onSignOut),
        icon: const Icon(Icons.switch_account, size: 18),
        label: const Text('Switch user'),
      ),
    );
  }
}
