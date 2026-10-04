import 'dart:async';

/// How long a loader may leave the shared dock tablet untouched before the app signs them out
/// (workflow W6). The React app uses the same five minutes (`LOADER_INACTIVITY_MS`).
const loaderInactivityTimeout = Duration(minutes: 5);

typedef MonitorTimerFactory = Timer Function(Duration delay, void Function() callback);

/// The inactivity rule for a shared tablet, with no dependency on widgets: a clock to read and
/// a timer to wait with are both injected, so it is tested with a fake clock.
///
/// It remembers when the person last did something ([activity]) and calls [onExpired] once
/// [timeout] has passed since then. A browser may hold a hidden tab's timers back for
/// much longer than they were set for, so the deadline is a comparison of timestamps, not
/// just a timer: [checkNow] (called when the tab comes back) expires immediately when the
/// deadline has already passed, however late or never the timer fired.
///
/// Like the web version, once it has expired it stays expired: further [activity] is ignored
/// until [start] is called again (after the next person has signed in).
class InactivityMonitor {
  InactivityMonitor({
    required this.onExpired,
    this.timeout = loaderInactivityTimeout,
    DateTime Function()? now,
    MonitorTimerFactory? timerFactory,
  })  : _now = now ?? DateTime.now,
        _timerFactory = timerFactory ?? Timer.new;

  final void Function() onExpired;
  final Duration timeout;
  final DateTime Function() _now;
  final MonitorTimerFactory _timerFactory;

  Timer? _timer;
  DateTime? _lastActivity;
  bool _running = false;
  bool _expired = false;

  /// Waiting for the deadline.
  bool get running => _running;

  /// The deadline passed and [onExpired] was called.
  bool get expired => _expired;

  /// When the person will be signed out unless they do something first.
  DateTime? get deadline => _running ? _lastActivity!.add(timeout) : null;

  /// Starts counting from now (a person has just signed in).
  void start() {
    _expired = false;
    _running = true;
    _lastActivity = _now();
    _arm(timeout);
  }

  /// The person touched, typed or scrolled: the countdown starts again. Cheap enough to call
  /// on every pointer move, because it only records the time; the timer notices at its deadline.
  void activity() {
    if (!_running) return;
    _lastActivity = _now();
  }

  /// Expires now if the deadline has already passed. Call it when the tab becomes visible again.
  void checkNow() {
    if (!_running) return;
    if (_remaining() <= Duration.zero) _expire();
  }

  /// Stops counting (signed out some other way). Does not call [onExpired].
  void stop() {
    _timer?.cancel();
    _timer = null;
    _running = false;
  }

  Duration _remaining() {
    final elapsed = _now().difference(_lastActivity!);
    // A clock that moved backwards must not grant more than one full period.
    if (elapsed.isNegative) {
      _lastActivity = _now();
      return timeout;
    }
    return timeout - elapsed;
  }

  void _arm(Duration delay) {
    _timer?.cancel();
    _timer = _timerFactory(delay, _onTimer);
  }

  void _onTimer() {
    _timer = null;
    if (!_running) return;
    final remaining = _remaining();
    if (remaining <= Duration.zero) {
      _expire();
    } else {
      _arm(remaining); // there was activity since the timer was set
    }
  }

  void _expire() {
    if (_expired) return;
    _expired = true;
    stop();
    onExpired();
  }
}
