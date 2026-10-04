import 'dart:async';

import 'package:flutter_test/flutter_test.dart';
import 'package:waypoint_loader/auth/inactivity_monitor.dart';

/// A clock and a timer factory in one: time only moves when the test says so, and timers fire
/// at their moment, in order, as [advance] passes it.
class FakeClock {
  DateTime now = DateTime.utc(2026, 10, 5, 8);
  final List<FakeTimer> timers = [];

  DateTime read() => now;

  Timer timer(Duration delay, void Function() callback) {
    final t = FakeTimer(now.add(delay), callback);
    timers.add(t);
    return t;
  }

  int get activeTimers => timers.where((t) => t.isActive).length;

  /// Time passes and every timer due on the way fires.
  void advance(Duration d) {
    final target = now.add(d);
    while (true) {
      final due = timers.where((t) => t.isActive && !t.at.isAfter(target)).toList()..sort((a, b) => a.at.compareTo(b.at));
      if (due.isEmpty) break;
      final next = due.first;
      if (next.at.isAfter(now)) now = next.at;
      next.fire();
    }
    now = target;
  }

  /// Time passes but no timer runs, as when a browser holds a hidden tab's timers back.
  void jumpWithoutTimers(Duration d) => now = now.add(d);
}

class FakeTimer implements Timer {
  FakeTimer(this.at, this._callback);
  final DateTime at;
  final void Function() _callback;
  bool _active = true;

  void fire() {
    if (!_active) return;
    _active = false;
    _callback();
  }

  @override
  void cancel() => _active = false;
  @override
  bool get isActive => _active;
  @override
  int get tick => _active ? 0 : 1;
}

void main() {
  late FakeClock clock;
  late int expiries;
  late InactivityMonitor monitor;

  InactivityMonitor build() => InactivityMonitor(onExpired: () => expiries++, now: clock.read, timerFactory: clock.timer);

  setUp(() {
    clock = FakeClock();
    expiries = 0;
    monitor = build();
  });

  test('the shared-tablet timeout is five minutes, the same as the web app', () {
    expect(loaderInactivityTimeout, const Duration(minutes: 5));
    expect(monitor.timeout, const Duration(minutes: 5));
  });

  test('signs out at exactly five minutes of no activity, not a moment before', () {
    monitor.start();
    clock.advance(const Duration(minutes: 5) - const Duration(milliseconds: 1));
    expect(expiries, 0);
    expect(monitor.expired, isFalse);
    clock.advance(const Duration(milliseconds: 1));
    expect(expiries, 1);
    expect(monitor.expired, isTrue);
    expect(monitor.running, isFalse);
  });

  test('does nothing until it is started', () {
    clock.advance(const Duration(hours: 1));
    expect(expiries, 0);
    expect(clock.activeTimers, 0);
  });

  test('activity starts the five minutes again', () {
    monitor.start();
    clock.advance(const Duration(minutes: 3));
    monitor.activity();
    clock.advance(const Duration(minutes: 2)); // five minutes after start, two after the touch
    expect(expiries, 0);
    clock.advance(const Duration(minutes: 2, seconds: 59));
    expect(expiries, 0);
    clock.advance(const Duration(seconds: 1)); // exactly five minutes after the touch
    expect(expiries, 1);
  });

  test('constant activity keeps the loader signed in indefinitely', () {
    monitor.start();
    for (var i = 0; i < 60; i++) {
      clock.advance(const Duration(minutes: 4, seconds: 59));
      monitor.activity();
    }
    expect(expiries, 0);
    expect(monitor.running, isTrue);
    expect(monitor.deadline, clock.now.add(const Duration(minutes: 5)));
  });

  test('stop cancels the countdown and nothing fires afterwards', () {
    monitor.start();
    clock.advance(const Duration(minutes: 4));
    monitor.stop();
    expect(clock.activeTimers, 0);
    clock.advance(const Duration(hours: 1));
    monitor.activity();
    monitor.checkNow();
    expect(expiries, 0);
    expect(monitor.running, isFalse);
    expect(monitor.deadline, isNull);
  });

  test('coming back after the deadline signs out immediately, even though the timer never ran', () {
    monitor.start();
    clock.advance(const Duration(minutes: 1));
    monitor.activity();
    clock.jumpWithoutTimers(const Duration(minutes: 5, seconds: 1)); // tab hidden; its timer was held back
    expect(expiries, 0, reason: 'nothing has noticed yet');
    monitor.checkNow();
    expect(expiries, 1);
    expect(clock.activeTimers, 0, reason: 'the held-back timer is cancelled, so it cannot fire a second time');
    clock.advance(const Duration(minutes: 10));
    expect(expiries, 1);
  });

  test('coming back before the deadline keeps the loader signed in', () {
    monitor.start();
    clock.jumpWithoutTimers(const Duration(minutes: 4, seconds: 59));
    monitor.checkNow();
    expect(expiries, 0);
    expect(monitor.running, isTrue);
  });

  test('a late timer that did run with time to spare re-arms for what is left', () {
    monitor.start();
    clock.advance(const Duration(minutes: 4));
    monitor.activity();
    clock.advance(const Duration(minutes: 1)); // the original timer fires now but the person was active
    expect(expiries, 0);
    expect(clock.activeTimers, 1);
    clock.advance(const Duration(minutes: 4));
    expect(expiries, 1);
  });

  test('once expired, activity is ignored until the next sign-in starts it again', () {
    monitor.start();
    clock.advance(const Duration(minutes: 5));
    expect(expiries, 1);
    monitor.activity();
    monitor.checkNow();
    clock.advance(const Duration(minutes: 30));
    expect(expiries, 1);
    expect(monitor.running, isFalse);

    monitor.start(); // the next person signs in
    expect(monitor.expired, isFalse);
    clock.advance(const Duration(minutes: 5));
    expect(expiries, 2);
  });

  test('a clock that moves backwards does not extend the deadline beyond five minutes', () {
    monitor.start();
    clock.jumpWithoutTimers(const Duration(minutes: -30));
    monitor.checkNow();
    expect(expiries, 0);
    clock.jumpWithoutTimers(const Duration(minutes: 5));
    monitor.checkNow();
    expect(expiries, 1);
  });
}
