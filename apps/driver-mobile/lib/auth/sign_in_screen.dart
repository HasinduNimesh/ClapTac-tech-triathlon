import 'package:flutter/material.dart';

import '../theme/tokens.dart';
import '../widgets/common.dart';
import 'auth.dart';

class SignInScreen extends StatefulWidget {
  const SignInScreen({super.key, required this.auth, this.switchingFrom});

  final AuthService auth;
  /// Shared dock tablet: who handed the device over (Figma "Switch dock user").
  final String? switchingFrom;

  @override
  State<SignInScreen> createState() => _SignInScreenState();
}

class _SignInScreenState extends State<SignInScreen> {
  final user = TextEditingController();
  final pass = TextEditingController();
  bool keep = true;
  bool hide = true;
  bool busy = false;
  SignInError? error;
  AuthSession? last;

  @override
  void initState() {
    super.initState();
    widget.auth.lastSession().then((s) => mounted ? setState(() => last = s) : null);
  }

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
      await widget.auth.signIn(user.text, pass.text, keepSignedIn: keep);
    } on SignInException catch (e) {
      if (mounted) setState(() => error = e.kind);
    } finally {
      if (mounted) setState(() => busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final noSignal = error == SignInError.noSignal;
    return Scaffold(
      backgroundColor: Wp.surface,
      body: LayoutBuilder(builder: (context, box) {
        final wide = box.maxWidth >= Wp.tabletBreakpoint;
        final form = ListView(padding: const EdgeInsets.fromLTRB(20, 24, 20, 24), children: [
          Text(widget.switchingFrom != null ? 'Switch dock user' : 'Sign in', style: const TextStyle(fontSize: 28, fontWeight: FontWeight.w700)),
          if (widget.switchingFrom != null) Padding(padding: const EdgeInsets.only(top: 4), child: Text('${widget.switchingFrom} was signed out. Their saved, unsent records stay protected on this device.', style: const TextStyle(color: Wp.muted))),
          const SizedBox(height: 16),
          if (noSignal) const Padding(padding: EdgeInsets.only(bottom: 16), child: StatusNote(tone: Tone.amber, icon: Icons.wifi_off, title: "No signal, so we can't check your password", text: 'Move to an area with coverage and try again. If you signed in on this phone before, you can keep working offline.')),
          if (error == SignInError.invalidCredentials) const Padding(padding: EdgeInsets.only(bottom: 16), child: StatusNote(tone: Tone.red, icon: Icons.error_outline, title: 'Staff ID or password is not right', text: 'Check both and try again. Ask your supervisor to reset the password if needed.')),
          if (error == SignInError.wrongRole) const Padding(padding: EdgeInsets.only(bottom: 16), child: StatusNote(tone: Tone.red, text: 'This app is for drivers and loaders. Store managers and dispatchers use the Waypoint web app.')),
          if (error == SignInError.noProfile) const Padding(padding: EdgeInsets.only(bottom: 16), child: StatusNote(tone: Tone.red, text: 'Your access profile could not be loaded. Try again.')),
          const Text('Staff ID or email', style: TextStyle(color: Wp.muted)),
          const SizedBox(height: 6),
          TextField(controller: user, autofillHints: const [AutofillHints.username], textInputAction: TextInputAction.next, decoration: const InputDecoration(prefixIcon: Icon(Icons.person_outline), hintText: 'e.g. DRV-0318')),
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
          const SizedBox(height: 12),
          Row(children: [
            Checkbox(value: keep, onChanged: (v) => setState(() => keep = v ?? true)),
            const Expanded(child: Text('Keep me signed in')),
            TextButton(onPressed: () => ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('Ask your supervisor or dispatcher to reset your password.'))), child: const Text('Forgot password?')),
          ]),
          const SizedBox(height: 12),
          FilledButton(onPressed: busy ? null : _submit, child: Text(busy ? (noSignal ? 'Waiting for signal…' : 'Signing in…') : 'Sign in')),
          const SizedBox(height: 16),
          if (noSignal && last != null)
            WpCard(
              child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
                Row(children: [
                  const CircleAvatar(backgroundColor: Wp.tint, child: Icon(Icons.person_outline, color: Wp.primary)),
                  const SizedBox(width: 12),
                  Expanded(child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                    Text(last!.profile.displayName ?? last!.profile.userId, style: const TextStyle(fontWeight: FontWeight.w600)),
                    Text('${last!.profile.role == 'LOADER' ? 'Loader' : 'Driver'} · signed in on this device ${hhmm(last!.signedInAt.toUtc().toIso8601String())}', style: const TextStyle(fontSize: 12, color: Wp.muted)),
                  ])),
                ]),
                const SizedBox(height: 12),
                OutlinedButton(onPressed: () => widget.auth.continueOffline(last!), child: const Text('Continue offline')),
                const SizedBox(height: 8),
                const Text('Your saved run is on this device. Records you make will sync when signal returns.', style: TextStyle(fontSize: 12, color: Wp.muted)),
              ]),
            )
          else
            const StatusNote(icon: Icons.verified_user_outlined, text: "After you sign in, today's run is saved on this phone. It keeps working without signal and syncs when you are back online."),
        ]);
        final hero = Container(
          decoration: const BoxDecoration(gradient: LinearGradient(colors: [Wp.heroStart, Wp.heroEnd], begin: Alignment.topLeft, end: Alignment.bottomRight)),
          padding: EdgeInsets.fromLTRB(24, MediaQuery.of(context).padding.top + 24, 24, 40),
          child: Column(crossAxisAlignment: CrossAxisAlignment.start, mainAxisAlignment: wide ? MainAxisAlignment.center : MainAxisAlignment.start, children: [
            Image.asset('assets/waypoint-logo.png', height: 52, color: Colors.white, colorBlendMode: BlendMode.srcIn, semanticLabel: 'Waypoint Group'),
            const SizedBox(height: 16),
            const Text('Drivers, Loaders and Store Manager', style: TextStyle(color: Colors.white, fontSize: 15)),
          ]),
        );
        if (wide) return Row(children: [Expanded(child: hero), SizedBox(width: 480, child: SafeArea(child: form))]);
        return Column(children: [hero, Expanded(child: Transform.translate(offset: const Offset(0, -20), child: Container(decoration: const BoxDecoration(color: Wp.surface, borderRadius: BorderRadius.vertical(top: Radius.circular(20))), child: form)))]);
      }),
    );
  }
}
