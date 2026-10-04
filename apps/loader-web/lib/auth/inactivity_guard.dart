import 'package:flutter/services.dart';
import 'package:flutter/widgets.dart';

import 'inactivity_monitor.dart';

/// Signs the loader out of a shared tablet after [timeout] without touching it (workflow W6).
///
/// Put it above the app's Navigator (in `MaterialApp.builder`) so that every screen, dialog and
/// sheet is inside it: touching any of them counts as activity. While [active] is false (nobody
/// is signed in) it does nothing. When the time runs out, [onExpired] is called once; the
/// next sign-in starts a new countdown when [active] becomes true again.
///
/// Activity is any pointer event (touch, mouse, wheel) and any hardware key press. Browsers
/// slow a hidden tab's timers, so the deadline is also checked when the tab comes back.
class InactivityGuard extends StatefulWidget {
  const InactivityGuard({
    super.key,
    required this.active,
    required this.onExpired,
    required this.child,
    this.timeout = loaderInactivityTimeout,
    this.now,
  });

  final bool active;
  final VoidCallback onExpired;
  final Widget child;
  final Duration timeout;

  /// A clock for tests; defaults to the real one.
  final DateTime Function()? now;

  @override
  State<InactivityGuard> createState() => _InactivityGuardState();
}

class _InactivityGuardState extends State<InactivityGuard> with WidgetsBindingObserver {
  late final InactivityMonitor _monitor = InactivityMonitor(
    timeout: widget.timeout,
    now: widget.now,
    onExpired: () => widget.onExpired(),
  );

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
    HardwareKeyboard.instance.addHandler(_onKey);
    if (widget.active) _monitor.start();
  }

  @override
  void didUpdateWidget(InactivityGuard old) {
    super.didUpdateWidget(old);
    if (widget.active && !old.active) {
      _monitor.start();
    } else if (!widget.active && old.active) {
      _monitor.stop();
    }
  }

  @override
  void dispose() {
    HardwareKeyboard.instance.removeHandler(_onKey);
    WidgetsBinding.instance.removeObserver(this);
    _monitor.stop();
    super.dispose();
  }

  bool _onKey(KeyEvent event) {
    _monitor.activity();
    return false; // never consume the key
  }

  // The tab was hidden or put to sleep and is back: its timer may not have run, so compare times.
  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    if (state == AppLifecycleState.resumed) _monitor.checkNow();
  }

  void _touch(PointerEvent _) => _monitor.activity();

  @override
  Widget build(BuildContext context) {
    return Listener(
      behavior: HitTestBehavior.translucent,
      onPointerDown: _touch,
      onPointerMove: _touch,
      onPointerUp: _touch,
      onPointerHover: _touch,
      onPointerSignal: _touch,
      child: widget.child,
    );
  }
}
