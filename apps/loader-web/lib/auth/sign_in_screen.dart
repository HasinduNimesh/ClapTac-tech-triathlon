import 'package:flutter/material.dart';

import '../theme/tokens.dart';
import '../widgets/common.dart';
import 'auth.dart';
import 'inactivity_monitor.dart';

/// The loader's way in. The password is typed on the identity server's own page, never here:
/// this screen only sends the browser there and explains it when something goes wrong.
class SignInScreen extends StatefulWidget {
  const SignInScreen({super.key, required this.auth});

  final AuthService auth;

  @override
  State<SignInScreen> createState() => _SignInScreenState();
}

class _SignInScreenState extends State<SignInScreen> {
  bool busy = false;

  Future<void> _signIn() async {
    setState(() => busy = true);
    await widget.auth.signIn();
    // On success the browser leaves for the identity server; on failure show why.
    if (mounted) setState(() => busy = false);
  }

  (String, String)? _message(SignInException? e) => switch (e?.kind) {
        SignInError.notConfigured => ('Sign-in is not set up', 'This build of the loader app has no identity server configured. Ask IT to build it with OIDC_ISSUER set.'),
        SignInError.denied => ('Sign-in was cancelled or refused', e!.detail.isEmpty ? 'Try again, or ask your supervisor if your account is active.' : e.detail),
        SignInError.notVerified => ('Sign-in could not be verified', "Start again from this page. If it keeps happening, clear this site's data and try again."),
        SignInError.exchangeFailed => ('Sign-in did not complete', 'The identity service rejected the request. Try again in a moment.'),
        SignInError.noConnection => ('No connection to Waypoint', 'The loader app needs a network connection. Check the dock Wi-Fi and try again.'),
        SignInError.wrongRole => ('This app is for loaders', 'Dispatchers and store managers use the Waypoint web app; drivers use the driver app.'),
        SignInError.noProfile => ('Your access profile could not be loaded', 'Your account is not set up for Waypoint yet. Ask your supervisor.'),
        SignInError.wrongAudience => ('Signed in, but not for the Waypoint API', 'The identity server issued a token for "${e!.detail}", which the Waypoint services will not accept. Ask IT to check the loader client\'s API resource setting.'),
        SignInError.sessionExpired => ('Your session ended', 'Sign in again to continue. Loads and reports are saved on the server as you make them.'),
        SignInError.inactivity => (
            'Signed out',
            'You were signed out after ${loaderInactivityTimeout.inMinutes} minutes of inactivity.${e!.detail == 'unsent' ? ' A report that had not been sent was discarded. Open the trip and send it again.' : ''}',
          ),
        null => null,
      };

  @override
  Widget build(BuildContext context) {
    return ListenableBuilder(
      listenable: widget.auth,
      builder: (context, _) {
        final message = _message(widget.auth.error);
        final canSignIn = widget.auth.config.configured;
        return Scaffold(
          backgroundColor: Wp.surface,
          body: LayoutBuilder(builder: (context, box) {
            final wide = box.maxWidth >= Wp.tabletBreakpoint;
            final form = ListView(padding: const EdgeInsets.fromLTRB(24, 32, 24, 24), children: [
              const Text('Sign in', style: TextStyle(fontSize: 28, fontWeight: FontWeight.w700)),
              const SizedBox(height: 4),
              const Text('Loader workspace · dock tablet and phone', style: TextStyle(color: Wp.muted)),
              const SizedBox(height: 20),
              if (message != null) Padding(padding: const EdgeInsets.only(bottom: 16), child: StatusNote(tone: Tone.red, icon: Icons.error_outline, title: message.$1, text: message.$2)),
              const Text('You will sign in with your Waypoint account on the secure sign-in page, then come back here.', style: TextStyle(color: Wp.muted)),
              const SizedBox(height: 20),
              FilledButton(onPressed: busy || !canSignIn ? null : _signIn, child: Text(busy ? 'Opening sign-in…' : 'Sign in')),
              const SizedBox(height: 16),
              const StatusNote(icon: Icons.wifi, text: 'The loader app works online only. Every load, shortfall report and ready confirmation goes straight to the dispatcher.'),
            ]);
            final hero = Container(
              decoration: const BoxDecoration(gradient: LinearGradient(colors: [Wp.heroStart, Wp.heroEnd], begin: Alignment.topLeft, end: Alignment.bottomRight)),
              padding: const EdgeInsets.fromLTRB(28, 32, 28, 40),
              child: Column(crossAxisAlignment: CrossAxisAlignment.start, mainAxisAlignment: wide ? MainAxisAlignment.center : MainAxisAlignment.start, children: [
                Image.asset('assets/waypoint-logo.png', height: 52, color: Colors.white, colorBlendMode: BlendMode.srcIn, semanticLabel: 'Waypoint Group'),
                const SizedBox(height: 16),
                const Text('Loader workspace', style: TextStyle(color: Colors.white, fontSize: 18, fontWeight: FontWeight.w600)),
                const Text('Load in the right order, report shortfalls, confirm ready to depart.', style: TextStyle(color: Colors.white)),
              ]),
            );
            if (wide) return Row(children: [Expanded(child: hero), SizedBox(width: 480, child: form)]);
            return Column(children: [
              hero,
              Expanded(child: Transform.translate(offset: const Offset(0, -20), child: Container(decoration: const BoxDecoration(color: Wp.surface, borderRadius: BorderRadius.vertical(top: Radius.circular(20))), child: form))),
            ]);
          }),
        );
      },
    );
  }
}
