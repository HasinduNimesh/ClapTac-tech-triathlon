import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../../theme/assets.dart';
import '../../theme/tokens.dart';
import '../../widgets/app_buttons.dart';
import '../../widgets/hero_header.dart';
import '../../widgets/svg_icon.dart';
import 'sign_in_assets.dart';

/// A driver who already signed in on this phone today.
///
/// [signedInAt] is just the time ("05:12"); [runSummary] is the part of the
/// sentence that follows "Your run for", e.g. "Sat 26 Sep (VEH014, 9 stops)".
class SavedSession {
  const SavedSession({
    required this.name,
    required this.role,
    required this.signedInAt,
    required this.runSummary,
  });

  final String name;
  final String role;
  final String signedInAt;
  final String runSummary;
}

/// Sign-in (Figma 84:5938) and its no-signal variant (84:5990).
class SignInScreen extends StatefulWidget {
  const SignInScreen({
    super.key,
    required this.onSignIn,
    this.onContinueOffline,
    this.noSignal = false,
    this.savedSession,
    this.onForgotPassword,
  });

  final void Function(String staffId, String password) onSignIn;
  final VoidCallback? onContinueOffline;
  final bool noSignal;
  final SavedSession? savedSession;
  final VoidCallback? onForgotPassword;

  @override
  State<SignInScreen> createState() => _SignInScreenState();
}

class _SignInScreenState extends State<SignInScreen> {
  final _staffId = TextEditingController();
  final _password = TextEditingController();
  final _passwordFocus = FocusNode();
  bool _obscure = true;
  bool _keepSignedIn = true;

  bool get _canSubmit => !widget.noSignal && _staffId.text.trim().isNotEmpty && _password.text.isNotEmpty;

  @override
  void initState() {
    super.initState();
    _staffId.addListener(_changed);
    _password.addListener(_changed);
  }

  @override
  void dispose() {
    _staffId.dispose();
    _password.dispose();
    _passwordFocus.dispose();
    super.dispose();
  }

  void _changed() => setState(() {});

  void _submit() {
    if (!_canSubmit) return;
    widget.onSignIn(_staffId.text.trim(), _password.text);
  }

  @override
  Widget build(BuildContext context) {
    final noSignal = widget.noSignal;
    return AnnotatedRegion<SystemUiOverlayStyle>(
      value: SystemUiOverlayStyle.light,
      child: Scaffold(
        backgroundColor: AppColors.signInHeroBottom,
        body: Stack(
          children: [
            Positioned(
              top: 0,
              left: 0,
              right: 0,
              child: HeroBackground(
                height: 260,
                style: HeroStyle.signIn,
                child: Align(
                  alignment: Alignment.topLeft,
                  child: Padding(
                    padding: const EdgeInsets.only(left: 24, top: 64),
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      mainAxisSize: MainAxisSize.min,
                      children: [
                        SizedBox(
                          width: noSignal ? 234 : 216,
                          height: noSignal ? 78 : 72,
                          child: Image.asset(AppAssets.logoWordmark, fit: BoxFit.cover, semanticLabel: 'Waypoint Group'),
                        ),
                        const SizedBox(height: 12),
                        Text(
                          noSignal ? 'Drivers, Loaders and Store Managers' : 'Drivers, Loaders and Store Manager',
                          style: AppText.of(15, FontWeight.w400, color: const Color(0xD9FFFFFF), height: 22 / 15),
                        ),
                      ],
                    ),
                  ),
                ),
              ),
            ),
            Positioned.fill(
              top: 200,
              child: Container(
                decoration: const BoxDecoration(
                  color: AppColors.surface,
                  borderRadius: BorderRadius.vertical(top: Radius.circular(AppRadius.r24)),
                ),
                clipBehavior: Clip.antiAlias,
                child: SingleChildScrollView(
                  keyboardDismissBehavior: ScrollViewKeyboardDismissBehavior.onDrag,
                  padding: const EdgeInsets.fromLTRB(24, 28, 24, 32),
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.stretch,
                    children: [
                      Semantics(
                        header: true,
                        child: Text('Sign in', style: AppText.of(28, FontWeight.w700, height: 41 / 28)),
                      ),
                      if (noSignal) ...[const SizedBox(height: 20), const _NoSignalAlert()],
                      const SizedBox(height: 20),
                      _Field(
                        label: 'Staff ID or email',
                        icon: const SvgIcon(AppAssets.user, size: 18),
                        controller: _staffId,
                        hint: 'e.g. DRV-0318',
                        filled: noSignal,
                        keyboardType: TextInputType.emailAddress,
                        textInputAction: TextInputAction.next,
                        onSubmitted: (_) => _passwordFocus.requestFocus(),
                        fieldKey: const Key('sign_in_staff_id'),
                        autofillHints: const [AutofillHints.username],
                      ),
                      const SizedBox(height: 20),
                      _Field(
                        label: 'Password',
                        icon: const SvgIcon(AppAssets.lock, size: 18),
                        controller: _password,
                        hint: 'Enter your password',
                        filled: noSignal,
                        obscure: _obscure,
                        focusNode: _passwordFocus,
                        textInputAction: TextInputAction.done,
                        onSubmitted: (_) => _submit(),
                        fieldKey: const Key('sign_in_password'),
                        autofillHints: const [AutofillHints.password],
                        trailing: noSignal ? null : _EyeToggle(obscured: _obscure, onPressed: () => setState(() => _obscure = !_obscure)),
                      ),
                      if (!noSignal) ...[
                        const SizedBox(height: 8),
                        _OptionsRow(
                          keepSignedIn: _keepSignedIn,
                          onChanged: (value) => setState(() => _keepSignedIn = value),
                          onForgotPassword: widget.onForgotPassword,
                        ),
                        const SizedBox(height: 8),
                        AppButton(label: 'Sign in', onPressed: _canSubmit ? _submit : null),
                        const SizedBox(height: 20),
                        const _OfflineNote(),
                      ] else ...[
                        const SizedBox(height: 20),
                        const AppButton(label: 'Waiting for signal…', onPressed: null, weight: FontWeight.w400),
                        if (widget.savedSession != null) ...[
                          const SizedBox(height: 20),
                          _SavedSessionCard(session: widget.savedSession!, onContinueOffline: widget.onContinueOffline),
                        ],
                      ],
                    ],
                  ),
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _Field extends StatelessWidget {
  const _Field({
    required this.label,
    required this.icon,
    required this.controller,
    required this.hint,
    required this.fieldKey,
    this.filled = false,
    this.obscure = false,
    this.trailing,
    this.focusNode,
    this.keyboardType,
    this.textInputAction,
    this.onSubmitted,
    this.autofillHints,
  });

  final String label;
  final Widget icon;
  final TextEditingController controller;
  final String hint;
  final Key fieldKey;
  final bool filled;
  final bool obscure;
  final Widget? trailing;
  final FocusNode? focusNode;
  final TextInputType? keyboardType;
  final TextInputAction? textInputAction;
  final ValueChanged<String>? onSubmitted;
  final Iterable<String>? autofillHints;

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        ExcludeSemantics(
          child: Text(label, style: AppText.of(16, FontWeight.w400, color: AppColors.fieldLabel, height: 23 / 16)),
        ),
        const SizedBox(height: 8),
        Container(
          height: 48,
          padding: EdgeInsets.only(left: 16, right: trailing == null ? 16 : 4),
          decoration: BoxDecoration(
            color: filled ? const Color(0xFFF5F6FA) : Colors.white,
            border: Border.all(color: AppColors.fieldBorder),
            borderRadius: BorderRadius.circular(AppRadius.r4),
          ),
          child: Row(
            children: [
              icon,
              const SizedBox(width: 8),
              Expanded(
                child: Semantics(
                  label: label,
                  textField: true,
                  child: TextField(
                    key: fieldKey,
                    controller: controller,
                    focusNode: focusNode,
                    obscureText: obscure,
                    keyboardType: keyboardType,
                    textInputAction: textInputAction,
                    onSubmitted: onSubmitted,
                    autofillHints: autofillHints,
                    autocorrect: false,
                    enableSuggestions: !obscure,
                    cursorColor: AppColors.primary,
                    style: AppText.of(16, FontWeight.w400, height: 23 / 16),
                    decoration: InputDecoration(
                      isCollapsed: true,
                      border: InputBorder.none,
                      hintText: hint,
                      hintStyle: AppText.of(16, FontWeight.w400, color: AppColors.fieldHint, height: 23 / 16),
                    ),
                  ),
                ),
              ),
              if (trailing != null) trailing!,
            ],
          ),
        ),
      ],
    );
  }
}

class _EyeToggle extends StatelessWidget {
  const _EyeToggle({required this.obscured, required this.onPressed});

  final bool obscured;
  final VoidCallback onPressed;

  @override
  Widget build(BuildContext context) {
    return Semantics(
      button: true,
      label: obscured ? 'Show password' : 'Hide password',
      excludeSemantics: true,
      child: InkResponse(
        key: const Key('sign_in_toggle_password'),
        onTap: onPressed,
        radius: 22,
        child: SizedBox(
          width: 44,
          height: 44,
          child: Center(
            child: Opacity(opacity: obscured ? 1 : 0.6, child: const SvgIcon(AppAssets.eye, size: 20)),
          ),
        ),
      ),
    );
  }
}

class _OptionsRow extends StatelessWidget {
  const _OptionsRow({required this.keepSignedIn, required this.onChanged, required this.onForgotPassword});

  final bool keepSignedIn;
  final ValueChanged<bool> onChanged;
  final VoidCallback? onForgotPassword;

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      height: 44,
      child: Row(
        mainAxisAlignment: MainAxisAlignment.spaceBetween,
        children: [
          Semantics(
            checked: keepSignedIn,
            label: 'Keep me signed in',
            excludeSemantics: true,
            child: InkWell(
              key: const Key('sign_in_keep_signed_in'),
              borderRadius: BorderRadius.circular(AppRadius.r4),
              onTap: () => onChanged(!keepSignedIn),
              child: SizedBox(
                height: 44,
                child: Row(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    Container(
                      width: 18,
                      height: 18,
                      decoration: BoxDecoration(
                        color: keepSignedIn ? AppColors.primary : Colors.white,
                        border: Border.all(color: keepSignedIn ? AppColors.primary : AppColors.fieldBorder),
                        borderRadius: BorderRadius.circular(3),
                      ),
                      child: keepSignedIn ? const CustomPaint(key: Key('sign_in_checkmark'), painter: _CheckPainter()) : null,
                    ),
                    const SizedBox(width: 8),
                    Text('Keep me signed in', style: AppText.of(14, FontWeight.w400, height: 20 / 14)),
                  ],
                ),
              ),
            ),
          ),
          Semantics(
            button: true,
            label: 'Forgot password?',
            excludeSemantics: true,
            child: InkWell(
              key: const Key('sign_in_forgot_password'),
              borderRadius: BorderRadius.circular(AppRadius.r4),
              onTap: onForgotPassword,
              child: Container(
                height: 44,
                alignment: Alignment.centerRight,
                child: Text('Forgot password?', style: AppText.of(14, FontWeight.w400, color: AppColors.primary, height: 20 / 14)),
              ),
            ),
          ),
        ],
      ),
    );
  }
}

class _OfflineNote extends StatelessWidget {
  const _OfflineNote();

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(color: const Color(0x0F3A57E8), borderRadius: BorderRadius.circular(AppRadius.r8)),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const SvgIcon(AppAssets.shieldBlue, size: 20),
          const SizedBox(width: 10),
          Expanded(
            child: Text(
              'After you sign in, today’s run is saved on this phone. It keeps working without signal and syncs when you are back online.',
              style: AppText.of(13, FontWeight.w400, height: 19 / 13),
            ),
          ),
        ],
      ),
    );
  }
}

class _NoSignalAlert extends StatelessWidget {
  const _NoSignalAlert();

  @override
  Widget build(BuildContext context) {
    return Semantics(
      container: true,
      liveRegion: true,
      child: Container(
        padding: const EdgeInsets.all(14),
        decoration: BoxDecoration(
          color: const Color(0x14F16A1B),
          border: Border.all(color: const Color(0x59F16A1B)),
          borderRadius: BorderRadius.circular(AppRadius.r8),
        ),
        child: Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            const SvgIcon(SignInAssets.wifiOff, size: 22),
            const SizedBox(width: 12),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text('No signal, so we can’t check your password', style: AppText.of(15, FontWeight.w600, height: 22 / 15)),
                  const SizedBox(height: 4),
                  Text(
                    'Move to an area with coverage and try again. If you signed in on this phone before, you can keep working offline.',
                    style: AppText.of(13, FontWeight.w400, height: 19 / 13),
                  ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _SavedSessionCard extends StatelessWidget {
  const _SavedSessionCard({required this.session, required this.onContinueOffline});

  final SavedSession session;
  final VoidCallback? onContinueOffline;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: Colors.white,
        border: Border.all(color: AppColors.fieldBorder),
        borderRadius: BorderRadius.circular(AppRadius.r8),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Row(
            children: [
              Container(
                padding: const EdgeInsets.all(8),
                decoration: BoxDecoration(color: const Color(0x1A3A57E8), borderRadius: BorderRadius.circular(20)),
                child: const SvgIcon(SignInAssets.userBlue, size: 20),
              ),
              const SizedBox(width: 12),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(session.name, style: AppText.of(16, FontWeight.w600, height: 23 / 16)),
                    const SizedBox(height: 2),
                    Text(
                      '${session.role} · signed in on this phone today at ${session.signedInAt}',
                      style: AppText.of(13, FontWeight.w400, color: AppColors.fieldLabel, height: 19 / 13),
                    ),
                  ],
                ),
              ),
            ],
          ),
          const SizedBox(height: 12),
          AppButton(label: 'Continue offline', outline: true, weight: FontWeight.w400, onPressed: onContinueOffline),
          const SizedBox(height: 12),
          Text(
            'Your run for ${session.runSummary} is saved here. Deliveries you record will sync when signal returns.',
            style: AppText.of(13, FontWeight.w400, color: AppColors.fieldLabel, height: 19 / 13),
          ),
        ],
      ),
    );
  }
}

class _CheckPainter extends CustomPainter {
  const _CheckPainter();

  @override
  void paint(Canvas canvas, Size size) {
    final paint = Paint()
      ..color = Colors.white
      ..style = PaintingStyle.stroke
      ..strokeWidth = 2
      ..strokeCap = StrokeCap.round
      ..strokeJoin = StrokeJoin.round;
    final path = Path()
      ..moveTo(size.width * 0.24, size.height * 0.52)
      ..lineTo(size.width * 0.43, size.height * 0.70)
      ..lineTo(size.width * 0.76, size.height * 0.32);
    canvas.drawPath(path, paint);
  }

  @override
  bool shouldRepaint(covariant CustomPainter oldDelegate) => false;
}
