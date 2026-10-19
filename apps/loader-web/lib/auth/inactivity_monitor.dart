import 'dart:async';

/// How long a loader may leave the shared dock tablet untouched before the app signs them out (workflow W6).
/// Loading is hands-on work, so this is long enough not to catch someone busy with the truck; the sign-out
/// itself still stops the next person from acting as the last one. [loaderInactivityWarning] before it, the
/// app asks "Still there?".
const loaderInactivityTimeout = Duration(minutes: 15);

/// How long before the sign-out the loader is warned, so a sign-out never comes as a surprise.
const loaderInactivityWarning = Duration(minutes: 1);

typedef MonitorTimerFactory = Timer Function(Duration delay, void Function() callback);

/// The inactivity rule for a shared tablet, with no dependency on widgets: a clock to read and
/// a timer to wait with are both injected, so it is tested with a fake clock.
///
/// It remembers when the person last did something ([activity]), calls [onWarning] once when only
/// [warnBefore] is left, and calls [onExpired] once [timeout] has passed since the last activity. Any
/// activity while the warning is showing clears it ([onWarningCleared]) and starts the full period again.
/// A browser may hold a hidden tab's timers back for much longer than they were set for, so each deadline is
/// a comparison of timestamps, not just a timer: [checkNow] (called when the tab comes back) warns or expires
/// at once when the time has already come, however late or never the timer fired.
///
/// Once it has expired it stays expired: further [activity] is ignored until [start] is called again (after
/// the next person has signed in). A [warnBefore] of zero, or one not shorter than [timeout], means no warning.
class InactivityMonitor {
  InactivityMonitor({
    required this.onExpired,
    this.onWarning,
    this.onWarningCleared,
    this.timeout = loaderInactivityTimeout,
    this.warnBefore = loaderInactivityWarning,
    DateTime Function()? now,
    MonitorTimerFactory? timerFactory,
  })  : _now = now ?? DateTime.now,
        _timerFactory = timerFactory ?? Timer.new;

  final void Function() onExpired;
  final void Function()? onWarning;
  final void Function()? onWarningCleared;
  final Duration timeout;
  final Duration warnBefore;
  final DateTime Function() _now;
  final MonitorTimerFactory _timerFactory;

  Timer? _timer;
  DateTime? _lastActivity;
  bool _running = false;
  bool _expired = false;
  bool _warned = false;

  /// Waiting for the deadline.
  bool get running => _running;

  /// The deadline passed and [onExpired] was called.
  bool get expired => _expired;

  /// The warning is showing: only [warnBefore] is left and nothing has been touched since.
  bool get warning => _running && _warned;

  /// When the person will be signed out unless they do something first.
  DateTime? get deadline => _running ? _lastActivity!.add(timeout) : null;

  bool get _warns => warnBefore > Duration.zero && warnBefore < timeout;

  /// Starts counting from now (a person has just signed in).
  void start() {
    _expired = false;
    _warned = false;
    _running = true;
    _lastActivity = _now();
    _arm(_warns ? timeout - warnBefore : timeout);
  }

  /// The person touched, typed or scrolled: the countdown starts again. Cheap enough to call
  /// on every pointer move, because it only records the time; the timer notices at its deadline.
  void activity() {
    if (!_running) return;
    _lastActivity = _now();
    if (_warned) {
      // The pending timer recomputes what is left when it fires, so it needs no re-arming.
      _warned = false;
      onWarningCleared?.call();
    }
  }

  /// Warns or expires now if that time has already come. Call it when the tab becomes visible again.
  void checkNow() {
    if (!_running) return;
    final remaining = _remaining();
    if (remaining <= Duration.zero) {
      _expire();
    } else if (_warns && !_warned && remaining <= warnBefore) {
      _warn();
      _arm(remaining);
    }
  }

  /// Stops counting (signed out some other way). Does not call [onExpired].
  void stop() {
    _timer?.cancel();
    _timer = null;
    _running = false;
    if (_warned) {
      _warned = false;
      onWarningCleared?.call();
    }
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
    } else if (_warns && remaining <= warnBefore) {
      if (!_warned) _warn();
      _arm(remaining);
    } else {
      // There was activity since the timer was set: wait until the warning is due again.
      _arm(_warns ? remaining - warnBefore : remaining);
    }
  }

  void _warn() {
    _warned = true;
    onWarning?.call();
  }

  void _expire() {
    if (_expired) return;
    _expired = true;
    stop();
    onExpired();
  }
}
