import 'package:flutter/material.dart';

import '../theme/tokens.dart';
import '../widgets/common.dart';
import 'auth.dart';

class SignInScreen extends StatefulWidget {
  const SignInScreen({super.key, required this.auth});

  final AuthService auth;

  @override
  State<SignInScreen> createState() => _SignInScreenState();
}

class _SignInScreenState extends State<SignInScreen> {
  final user = TextEditingController();
  final pass = TextEditingController();
  bool hide = true;
  bool busy = false;
  SignInException? error;

  @override
  void dispose() {
    user.dispose();
    pass.dispose();
    super.dispose();
  }

  Future<void> _submit() async {
    if (user.text.trim().isEmpty || pass.text.isEmpty) return;
    setState(() { busy = true; error = null; });
    try {
      await widget.auth.signIn(user.text, pass.text);
    } on SignInException catch (e) {
      if (mounted) setState(() => error = e);
    } finally {
      if (mounted) setState(() => busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final message = switch (error?.kind) {
      SignInError.invalidCredentials => ('Staff ID or password is not right', 'Check both and try again. Ask your supervisor to reset the password if needed.'),
      SignInError.noConnection => ('No connection to Waypoint', 'The loader app needs a network connection. Check the dock Wi-Fi and try again.'),
      SignInError.wrongRole => ('This app is for loaders', 'Dispatchers and store managers use the Waypoint web app; drivers use the driver app.'),
      SignInError.noProfile => ('Your access profile could not be loaded', 'Try again in a moment.'),
      null => null,
    };
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
          const Text('Staff ID or email', style: TextStyle(color: Wp.muted)),
          const SizedBox(height: 6),
          TextField(controller: user, autofillHints: const [AutofillHints.username], textInputAction: TextInputAction.next, decoration: const InputDecoration(prefixIcon: Icon(Icons.person_outline), hintText: 'e.g. LDR-0112')),
          const SizedBox(height: 16),
          const Text('Password', style: TextStyle(color: Wp.muted)),
          const SizedBox(height: 6),
          TextField(
            controller: pass,
            obscureText: hide,
            autofillHints: const [AutofillHints.password],
            onSubmitted: (_) => _submit(),
            decoration: InputDecoration(prefixIcon: const Icon(Icons.lock_outline), hintText: 'Enter your password', suffixIcon: IconButton(onPressed: () => setState(() => hide = !hide), icon: Icon(hide ? Icons.visibility_outlined : Icons.visibility_off_outlined), tooltip: hide ? 'Show password' : 'Hide password')),
          ),
          const SizedBox(height: 20),
          FilledButton(onPressed: busy ? null : _submit, child: Text(busy ? 'Signing in…' : 'Sign in')),
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
        return Column(children: [hero, Expanded(child: Transform.translate(offset: const Offset(0, -20), child: Container(decoration: const BoxDecoration(color: Wp.surface, borderRadius: BorderRadius.vertical(top: Radius.circular(20))), child: form)))]);
      }),
    );
  }
}
