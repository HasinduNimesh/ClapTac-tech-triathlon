import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import 'inactivity_monitor.dart';

/// Signs the loader out of a shared tablet after [timeout] without touching it (workflow W6), asking "Still
/// there?" with a countdown for the last [warnBefore]. Any touch dismisses the question and starts the time again.
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
    this.warnBefore = loaderInactivityWarning,
    this.now,
  });

  final bool active;
  final VoidCallback onExpired;
  final Widget child;
  final Duration timeout;
  final Duration warnBefore;

  /// A clock for tests; defaults to the real one.
  final DateTime Function()? now;

  @override
  State<InactivityGuard> createState() => _InactivityGuardState();
}

class _InactivityGuardState extends State<InactivityGuard> with WidgetsBindingObserver {
  late final InactivityMonitor _monitor = InactivityMonitor(
    timeout: widget.timeout,
    warnBefore: widget.warnBefore,
    now: widget.now,
    onExpired: () {
      _showWarning(false);
      widget.onExpired();
    },
    onWarning: () => _showWarning(true),
    onWarningCleared: () => _showWarning(false),
  );

  bool _warning = false;
  bool _closed = false;
  Timer? _tick;

  void _showWarning(bool show) {
    if (_closed || _warning == show) return;
    _tick?.cancel();
    _tick = show ? Timer.periodic(const Duration(seconds: 1), (_) => mounted ? setState(() {}) : null) : null;
    setState(() => _warning = show);
  }

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
    _closed = true;
    _tick?.cancel();
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
      child: Stack(
        fit: StackFit.passthrough,
        children: [
          widget.child,
          if (_warning) Positioned(left: 16, right: 16, bottom: 24, child: _StillThereCard(remaining: _remaining(), onStay: _monitor.activity)),
        ],
      ),
    );
  }

  Duration _remaining() {
    final deadline = _monitor.deadline;
    if (deadline == null) return Duration.zero;
    final left = deadline.difference((widget.now ?? DateTime.now)());
    return left.isNegative ? Duration.zero : left;
  }
}

/// The "Still there?" question shown for the last minute before the automatic sign-out.
class _StillThereCard extends StatelessWidget {
  const _StillThereCard({required this.remaining, required this.onStay});

  final Duration remaining;
  final VoidCallback onStay;

  @override
  Widget build(BuildContext context) {
    final seconds = remaining.inSeconds + (remaining.inMilliseconds % 1000 > 0 ? 1 : 0);
    final text = seconds >= 60 ? '${seconds ~/ 60}:${(seconds % 60).toString().padLeft(2, '0')}' : '$seconds seconds';
    return Semantics(
      liveRegion: true,
      container: true,
      child: Material(
        elevation: 6,
        borderRadius: BorderRadius.circular(12),
        color: Theme.of(context).colorScheme.surface,
        child: Padding(
          padding: const EdgeInsets.all(16),
          child: Row(
            children: [
              Expanded(
                child: Column(
                  mainAxisSize: MainAxisSize.min,
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    const Text('Still there?', style: TextStyle(fontSize: 16, fontWeight: FontWeight.w700)),
                    const SizedBox(height: 2),
                    Text('You will be signed out in $text because nothing has been touched.'),
                  ],
                ),
              ),
              const SizedBox(width: 12),
              FilledButton(onPressed: onStay, child: const Text('Stay signed in')),
            ],
          ),
        ),
      ),
    );
  }
}
