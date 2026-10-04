import 'dart:async';

import 'package:flutter_test/flutter_test.dart';
import 'package:waypoint_driver/app/driver_session.dart';
import 'package:waypoint_driver/auth/auth_gateway.dart';
import 'package:waypoint_driver/auth/profile_api.dart';
import 'package:waypoint_driver/data/driver_models.dart';
import 'package:waypoint_driver/messages/messages.dart';
import 'package:waypoint_driver/offline/local_database.dart';
import 'package:waypoint_driver/sync/sync.dart';
import 'package:waypoint_driver/trips/route_store.dart';
import 'package:waypoint_driver/trips/trip_source.dart';
import 'package:waypoint_driver/trips/trip_start.dart';

// One phone, two drivers. Driver A's load, message read or trip start is still waiting for Waypoint when A
// signs out and B signs in. Whatever A's request brings back belongs to A and must never reach B's session,
// or be saved under B's account.

const _a = DriverProfile(userId: 'USR-A', subject: 'usr-a', roles: ['DRIVER'], vehicleId: 'VEH001');
const _b = DriverProfile(userId: 'USR-B', subject: 'usr-b', roles: ['DRIVER'], vehicleId: 'VEH002');
const _stopA = StopInfo(stopId: 'stop-a', sequence: 1, outletCode: 'OUTA', name: 'A\'s outlet', windowStart: '', windowEnd: '', units: 10, accessNote: '', contactNote: '', goods: 'G');
const _stopB = StopInfo(stopId: 'stop-b', sequence: 1, outletCode: 'OUTB', name: 'B\'s outlet', windowStart: '', windowEnd: '', units: 5, accessNote: '', contactNote: '', goods: 'G');
const _tripA = TripInfo(tripId: 'trip-a', runId: 'run-a', vehicleCode: 'VEH001', tripRef: 'PLAN-A', depot: 'D', window: '', stops: [_stopA], runStatus: 'in_progress');
const _tripB = TripInfo(tripId: 'trip-b', runId: 'run-b', vehicleCode: 'VEH002', tripRef: 'PLAN-B', depot: 'D', window: '', stops: [_stopB], runStatus: 'in_progress');
// A trip both drivers could be on, to show the checks do not rely on the trips being different.
const _shared = TripInfo(tripId: 'trip-shared', runId: 'run-s', vehicleCode: 'VEH001', tripRef: 'PLAN-S', depot: 'D', window: '', stops: [_stopA], runStatus: 'prepared', planId: 'p', planVersion: 1);

class _Auth implements AuthGateway {
  DriverProfile profile = _a;
  @override
  Future<AuthOutcome> signIn() async => AuthOutcome.signedIn(profile);
  @override
  Future<DriverProfile?> restore() async => profile;
  @override
  Future<void> signOut() async {}
  @override
  Future<String?> accessToken() async => 'tok';
}

/// Answers per call; a call can be held. The answer is for whoever's request it was.
class _Trips implements TripSource {
  _Trips(this.answers);
  final List<TripLoad> answers;
  final holds = <int, Completer<void>>{};
  int calls = 0;
  @override
  Future<TripLoad> loadToday() async {
    final call = calls++;
    await holds[call]?.future;
    return answers[call < answers.length ? call : answers.length - 1];
  }
}

/// A route store whose first read can be held, to model a slow disk.
class _SlowStore extends MemoryRouteStore {
  Completer<void>? holdReads;
  final release = Completer<void>();
  int reads = 0;
  @override
  Future<SavedRoute?> read(String userId) async {
    reads++;
    final held = holdReads;
    final result = saved[userId];
    if (held != null) await release.future;
    return result;
  }
}

class _Messages implements MessageSource {
  final byCall = <int, List<DispatcherMessage>>{};
  final holds = <int, Completer<void>>{};
  int calls = 0;
  @override
  Future<MessageLoad> load(String tripId) async {
    final call = calls++;
    await holds[call]?.future;
    return MessageLoad.loaded(byCall[call] ?? const []);
  }

  @override
  Future<MessageAck> acknowledge(String tripId, String messageId) async => const MessageAck.done();
}

class _Starter implements TripStarter {
  final holds = <int, Completer<void>>{};
  int calls = 0;
  @override
  Future<TripStartResult> start(TripInfo trip, {required String operationId}) async {
    await holds[calls++]?.future;
    return const TripStartResult(TripStartStatus.started);
  }
}

DispatcherMessage _message(String id, String body) =>
    DispatcherMessage(id: id, tripId: 'trip-shared', body: body, sentBy: 'USR_DISPATCHER', createdAt: DateTime.utc(2026, 10, 4, 3));

Future<void> _until(bool Function() condition) async {
  final deadline = DateTime.now().add(const Duration(seconds: 3));
  while (!condition() && DateTime.now().isBefore(deadline)) {
    await Future<void>.delayed(const Duration(milliseconds: 10));
  }
}

void main() {
  late _Auth auth;
  late MemoryRouteStore store;
  setUp(() {
    auth = _Auth();
    store = MemoryRouteStore();
  });

  DriverSession session({TripSource? trips, MessageSource? messages, TripStarter? starter}) {
    final s = DriverSession(
      database: InMemoryLocalDatabase(),
      queue: InMemorySyncQueue(),
      auth: auth,
      trips: trips,
      messageSource: messages,
      starter: starter,
      routeStore: store,
      messagePollInterval: const Duration(hours: 1),
    );
    addTearDown(s.dispose);
    return s;
  }

  group('a route load that is still running when the driver changes', () {
    test('is not given to the next driver, and is not saved under their account', () async {
      final trips = _Trips([const TripLoad.loaded(_tripA), const TripLoad.loaded(_tripB)]);
      trips.holds[0] = Completer<void>(); // A's request is slow
      final s = session(trips: trips);

      unawaited(s.signIn()); // A signs in; the route request starts and waits
      await _until(() => trips.calls == 1);
      await s.finishTrip(); // A signs out while it waits
      expect(s.signedIn, isFalse);

      auth.profile = _b;
      await s.signIn().timeout(const Duration(seconds: 2)); // B signs in
      expect(trips.calls, 2, reason: 'B makes a request of their own; it does not wait for A\'s');
      expect(s.trip.tripId, 'trip-b');

      trips.holds[0]!.complete(); // A's answer finally arrives
      await Future<void>.delayed(const Duration(milliseconds: 100));

      expect(s.trip.tripId, 'trip-b', reason: 'A\'s route did not replace B\'s');
      expect(s.trip.tripRef, 'PLAN-B');
      expect(store.saved['USR-B']?.trip.tripId, 'trip-b', reason: 'what is saved for B is B\'s route');
      expect(store.saved.containsKey('USR-A'), isFalse, reason: 'nothing of A\'s was saved after A signed out');
    });

    test('is thrown away when nobody has signed in since', () async {
      final trips = _Trips([const TripLoad.loaded(_tripA)]);
      trips.holds[0] = Completer<void>();
      final s = session(trips: trips);
      unawaited(s.signIn());
      await _until(() => trips.calls == 1);
      await s.finishTrip();
      trips.holds[0]!.complete();
      await Future<void>.delayed(const Duration(milliseconds: 100));
      expect(s.hasRoute, isFalse);
      expect(store.saved, isEmpty);
    });

    test('is thrown away when the same driver signs out and in again', () async {
      final trips = _Trips([const TripLoad.loaded(_tripA), const TripLoad.loaded(_tripB)]);
      trips.holds[0] = Completer<void>();
      final s = session(trips: trips);
      unawaited(s.signIn());
      await _until(() => trips.calls == 1);
      await s.finishTrip();
      await s.signIn(); // the same driver again: a new session, with its own request
      expect(trips.calls, 2);
      trips.holds[0]!.complete();
      await Future<void>.delayed(const Duration(milliseconds: 100));
      expect(s.trip.tripId, 'trip-b', reason: 'the request from the earlier session did not overwrite the newer one');
    });

    test('a load that finishes while the same driver is still signed in is applied as before', () async {
      final trips = _Trips([const TripLoad.loaded(_tripA)]);
      final s = session(trips: trips);
      await s.signIn();
      expect(s.trip.tripId, 'trip-a');
      expect(store.saved['USR-A']?.trip.tripId, 'trip-a');
    });

    test('a failed answer for the earlier driver does not show B an error either', () async {
      final trips = _Trips([const TripLoad.failed('Could not reach Waypoint. Check your connection and try again.'), const TripLoad.loaded(_tripB)]);
      trips.holds[0] = Completer<void>();
      final s = session(trips: trips);
      unawaited(s.signIn());
      await _until(() => trips.calls == 1);
      await s.finishTrip();
      auth.profile = _b;
      await s.signIn();
      trips.holds[0]!.complete();
      await Future<void>.delayed(const Duration(milliseconds: 100));
      expect(s.hasRoute, isTrue);
      expect(s.tripsError, isNull);
      expect(s.tripsLoading, isFalse);
    });

    test('an expired sign-in ends the session for any request still in flight', () async {
      final trips = _Trips([const TripLoad.loaded(_tripA), const TripLoad.loaded(_tripB)]);
      trips.holds[0] = Completer<void>();
      final s = session(trips: trips);
      unawaited(s.signIn());
      await _until(() => trips.calls == 1);
      // A's session ends because the provider no longer accepts it (the same path a rejected token takes).
      await s.finishTrip(force: true);
      auth.profile = _b;
      await s.signIn();
      trips.holds[0]!.complete();
      await Future<void>.delayed(const Duration(milliseconds: 100));
      expect(s.trip.tripId, 'trip-b');
    });
  });

  group('the saved route read that is still running when the driver changes', () {
    test('is not shown to the next driver', () async {
      // A's route cannot be loaded, so A's saved route is looked for; that read is slow, A signs out and B
      // signs in. When it finally returns, A's saved route must not appear on B's screen.
      final slow = _SlowStore()..saved['USR-A'] = SavedRoute(businessDate: ApiTripSource.dateKey(DateTime.now()), trip: _tripA);
      slow.holdReads = Completer<void>();
      final trips = _Trips([const TripLoad.failed('Could not reach Waypoint. Check your connection and try again.'), const TripLoad.loaded(_tripB)]);
      final s = DriverSession(database: InMemoryLocalDatabase(), queue: InMemorySyncQueue(), auth: auth, trips: trips, routeStore: slow);
      addTearDown(s.dispose);

      unawaited(s.signIn()); // A
      await _until(() => slow.reads == 1);
      await s.finishTrip();
      auth.profile = _b;
      slow.holdReads = null;
      await s.signIn(); // B
      expect(s.trip.tripId, 'trip-b');

      // Let A's read go: it was the first read, and it is the one that was held.
      slow.release.complete();
      await Future<void>.delayed(const Duration(milliseconds: 100));
      expect(s.trip.tripId, 'trip-b', reason: 'A\'s saved route did not replace B\'s route');
      expect(s.showingSavedRoute, isFalse);
    });
  });

  group('a message read that is still running when the driver changes', () {
    test('is not shown to the next driver, even when both are on the same trip', () async {
      final messages = _Messages()
        ..byCall[0] = [_message('a1', 'for driver A only')]
        ..byCall[1] = [_message('b1', 'for driver B')];
      messages.holds[0] = Completer<void>();
      final trips = _Trips([const TripLoad.loaded(_shared)]);
      final s = session(trips: trips, messages: messages);

      unawaited(s.signIn());
      await _until(() => messages.calls == 1); // A's read is waiting
      await s.finishTrip();
      auth.profile = _b;
      await s.signIn();
      await _until(() => messages.calls >= 2);
      messages.holds[0]!.complete();
      await Future<void>.delayed(const Duration(milliseconds: 100));

      expect(s.messages.map((m) => m.id), isNot(contains('a1')), reason: 'A\'s message was never shown to B');
    });
  });

  group('a trip start that is still running when the driver changes', () {
    test('does not mark the next driver\'s trip as started', () async {
      final starter = _Starter()..holds[0] = Completer<void>();
      final trips = _Trips([const TripLoad.loaded(_shared)]);
      final s = session(trips: trips, starter: starter);
      await s.signIn();
      unawaited(s.confirmLoad()); // A confirms the load; the start request is waiting
      await _until(() => starter.calls == 1);
      await s.finishTrip();

      auth.profile = _b;
      await s.signIn();
      expect(s.tripStartState, TripStartState.notStarted);
      starter.holds[0]!.complete();
      await Future<void>.delayed(const Duration(milliseconds: 100));

      expect(s.tripStartState, TripStartState.notStarted, reason: 'A\'s start does not count for B');
      expect(s.trip.started, isFalse);
    });
  });
}
