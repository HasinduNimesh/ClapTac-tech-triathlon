import 'package:flutter_test/flutter_test.dart';
import 'package:waypoint_driver/app/driver_session.dart';
import 'package:waypoint_driver/auth/auth_gateway.dart';
import 'package:waypoint_driver/auth/profile_api.dart';
import 'package:waypoint_driver/data/driver_models.dart';
import 'package:waypoint_driver/messages/messages.dart';
import 'package:waypoint_driver/offline/local_database.dart';
import 'package:waypoint_driver/sync/sync.dart';
import 'package:waypoint_driver/trips/trip_source.dart';

// Covers the two features together: finishing one trip opens the next (next-trip) while the driver may have
// dispatch's messages on screen (messages). Each is tested on its own in next_trip_test.dart and
// messages_test.dart; this is the interaction.

const _driver = DriverProfile(userId: 'USR006', subject: 'usr-driver', roles: ['DRIVER'], vehicleId: 'VEH001');
const _stopA = StopInfo(stopId: 'stop-a', sequence: 1, outletCode: 'OUTA', name: 'Dehiwala', windowStart: '', windowEnd: '', units: 10, accessNote: '', contactNote: '', goods: 'G');
const _stopB = StopInfo(stopId: 'stop-b', sequence: 1, outletCode: 'OUTB', name: 'Nugegoda', windowStart: '', windowEnd: '', units: 5, accessNote: '', contactNote: '', goods: 'G');
const _tripA = TripInfo(tripId: 'trip-a', runId: 'run-a', vehicleCode: 'VEH001', tripRef: 'PLAN-A', depot: 'D', window: '', stops: [_stopA], runStatus: 'in_progress');
const _tripB = TripInfo(tripId: 'trip-b', runId: 'run-b', vehicleCode: 'VEH001', tripRef: 'PLAN-B', depot: 'D', window: '', stops: [_stopB], runStatus: 'in_progress');

DispatcherMessage _message(String id, String tripId, {String body = 'Hello'}) =>
    DispatcherMessage(id: id, tripId: tripId, body: body, sentBy: 'USR_DISPATCHER', createdAt: DateTime.utc(2026, 10, 4, 3, 30));

class _Auth implements AuthGateway {
  @override
  Future<AuthOutcome> signIn() async => const AuthOutcome.signedIn(_driver);
  @override
  Future<DriverProfile?> restore() async => null;
  @override
  Future<void> signOut() async {}
  @override
  Future<String?> accessToken() async => 'tok';
}

/// Trip A first; then whatever the next lookup answers.
class _Trips implements TripSource {
  _Trips(this.next);
  final TripLoad next;
  int calls = 0;
  @override
  Future<TripLoad> loadToday() async => calls++ == 0 ? const TripLoad.loaded(_tripA) : next;
}

class _Messages implements MessageSource {
  final byTrip = <String, List<DispatcherMessage>>{};
  @override
  Future<MessageLoad> load(String tripId) async => MessageLoad.loaded(List.of(byTrip[tripId] ?? const []));
  @override
  Future<MessageAck> acknowledge(String tripId, String messageId) async => const MessageAck.done();
}

Future<void> _settle() => Future<void>.delayed(const Duration(milliseconds: 40));

DriverSession _session(_Trips trips, _Messages messages) => DriverSession(
      database: InMemoryLocalDatabase(),
      queue: InMemorySyncQueue(),
      auth: _Auth(),
      trips: trips,
      messageSource: messages,
      messagePollInterval: const Duration(hours: 1),
    );

Future<void> _finishTheOnlyStop(DriverSession s) async {
  await s.markArrived(_stopA);
  await s.recordDelivery(_stopA, const DeliveryDraft(outcome: DeliveryOutcome.failed, reason: 'OUTLET_CLOSED'));
}

void main() {
  test('finishing trip A shows none of its messages on trip B, and trip B\'s waiting messages are not announced', () async {
    final messages = _Messages()
      ..byTrip['trip-a'] = [_message('a1', 'trip-a', body: 'about trip A')]
      ..byTrip['trip-b'] = [_message('b1', 'trip-b', body: 'already waiting on trip B')];
    final session = _session(_Trips(const TripLoad.loaded(_tripB)), messages);
    addTearDown(session.dispose);
    await session.signIn();
    await _settle();
    expect(session.messages.map((m) => m.id), ['a1']);

    // Everything the screen could have shown once the route was on trip B.
    final shownOnB = <List<String>>[];
    session.addListener(() {
      if (session.hasRoute && session.trip.tripId == 'trip-b') shownOnB.add([for (final m in session.messages) m.id]);
    });

    await _finishTheOnlyStop(session);
    final result = await session.completeTrip();
    await _settle();

    expect(result.kind, TripWrapUp.nextTrip);
    expect(session.trip.tripId, 'trip-b');
    expect(session.messages.map((m) => m.id), ['b1']);
    expect(shownOnB, isNotEmpty);
    expect(shownOnB.any((ids) => ids.contains('a1')), isFalse, reason: 'trip A\'s message was never shown against trip B');
    expect(session.newMessageSerial, 0, reason: 'what trip B already had is not announced as new');
    expect(session.unreadMessages, 1);
  });

  test('a message that arrives for trip B after the switch is announced', () async {
    final messages = _Messages()..byTrip['trip-a'] = [_message('a1', 'trip-a')];
    final session = _session(_Trips(const TripLoad.loaded(_tripB)), messages);
    addTearDown(session.dispose);
    await session.signIn();
    await _settle();
    await _finishTheOnlyStop(session);
    await session.completeTrip();
    await _settle();
    expect(session.newMessageSerial, 0);
    messages.byTrip['trip-b'] = [_message('b-new', 'trip-b', body: 'sent after the switch')];
    await session.refreshMessages();
    expect(session.newMessageSerial, 1);
  });

  test('with no next trip the driver is signed out and no messages are left behind', () async {
    final messages = _Messages()..byTrip['trip-a'] = [_message('a1', 'trip-a')];
    final session = _session(_Trips(const TripLoad.none()), messages);
    addTearDown(session.dispose);
    await session.signIn();
    await _settle();
    expect(session.messages, hasLength(1));
    await _finishTheOnlyStop(session);
    final result = await session.completeTrip();
    expect(result.kind, TripWrapUp.signedOut);
    expect(session.signedIn, isFalse);
    expect(session.messages, isEmpty);
  });
}
