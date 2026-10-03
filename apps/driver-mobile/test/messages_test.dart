import 'dart:convert';
import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:waypoint_driver/app/driver_session.dart';
import 'package:waypoint_driver/auth/auth_failure.dart';
import 'package:waypoint_driver/auth/auth_gateway.dart';
import 'package:waypoint_driver/auth/profile_api.dart';
import 'package:waypoint_driver/data/driver_models.dart';
import 'package:waypoint_driver/main.dart';
import 'package:waypoint_driver/messages/messages.dart';
import 'package:waypoint_driver/offline/local_database.dart';
import 'package:waypoint_driver/sync/sync.dart';
import 'package:waypoint_driver/trips/trip_source.dart';

import 'helpers/render.dart';

const _driver = DriverProfile(userId: 'USR006', subject: 'usr-driver', roles: ['DRIVER'], vehicleId: 'VEH001');
const _stop = StopInfo(stopId: 'stop-1', sequence: 1, outletCode: 'O1', name: 'Dehiwala', windowStart: '', windowEnd: '', units: 10, accessNote: '', contactNote: '', goods: 'G');
const _trip = TripInfo(tripId: 'trip-1', runId: 'run-1', vehicleCode: 'VEH001', tripRef: 'PLAN1', depot: 'D', window: '', stops: [_stop], runStatus: 'in_progress');

Map<String, Object?> _json({String id = 'm1', String body = 'Use the rear gate', String stopId = '', String? acknowledgedBy, String? acknowledgedAt, String createdAt = '2026-10-04T03:30:00Z'}) => {
      'id': id,
      'tripId': 'trip-1',
      'stopId': stopId,
      'body': body,
      'sentBy': 'USR_DISPATCHER',
      'createdAt': createdAt,
      if (acknowledgedBy != null) 'acknowledgedBy': acknowledgedBy,
      if (acknowledgedAt != null) 'acknowledgedAt': acknowledgedAt,
    };

DispatcherMessage _message(String id, {bool acknowledged = false, String body = 'Hello', String stopId = '', DateTime? at}) => DispatcherMessage(
      id: id,
      tripId: 'trip-1',
      stopId: stopId,
      body: body,
      sentBy: 'USR_DISPATCHER',
      createdAt: at ?? DateTime.utc(2026, 10, 4, 3, 30),
      acknowledged: acknowledged,
    );

class _Auth implements AuthGateway {
  _Auth({this.token = 'tok'});
  String? token;
  Object? error;
  int signOuts = 0;
  @override
  Future<AuthOutcome> signIn() async => const AuthOutcome.signedIn(_driver);
  @override
  Future<DriverProfile?> restore() async => null;
  @override
  Future<void> signOut() async => signOuts++;
  @override
  Future<String?> accessToken() async {
    if (error != null) throw error!;
    return token;
  }
}

class _Trips implements TripSource {
  @override
  Future<TripLoad> loadToday() async => const TripLoad.loaded(_trip);
}

class _Source implements MessageSource {
  List<DispatcherMessage> messages = [];
  MessageLoad? failure;
  MessageAck? ackResult;
  int loads = 0;
  final acked = <String>[];

  @override
  Future<MessageLoad> load(String tripId) async {
    loads++;
    return failure ?? MessageLoad.loaded(List.of(messages));
  }

  @override
  Future<MessageAck> acknowledge(String tripId, String messageId) async {
    acked.add(messageId);
    final result = ackResult ?? const MessageAck.done();
    if (result.isDone) messages = [for (final m in messages) m.id == messageId ? m.asAcknowledged() : m];
    return result;
  }
}

DriverSession _session(_Source source, {_Auth? auth, Duration poll = const Duration(hours: 1)}) =>
    DriverSession(database: InMemoryLocalDatabase(), queue: InMemorySyncQueue(), auth: auth ?? _Auth(), trips: _Trips(), messageSource: source, messagePollInterval: poll);

Future<void> _settle() => Future<void>.delayed(const Duration(milliseconds: 40));

void main() {
  setUpAll(loadAppFonts);

  group('DispatcherMessage.fromJson', () {
    test('reads the server shape, and is unread until someone acknowledged it', () {
      final unread = DispatcherMessage.fromJson(_json(stopId: 'stop-1'));
      expect(unread.id, 'm1');
      expect(unread.stopId, 'stop-1');
      expect(unread.body, 'Use the rear gate');
      expect(unread.acknowledged, isFalse);
      expect(unread.createdAt, DateTime.utc(2026, 10, 4, 3, 30));
      expect(DispatcherMessage.fromJson(_json(acknowledgedBy: 'USR006')).acknowledged, isTrue);
      expect(DispatcherMessage.fromJson(_json(acknowledgedBy: '', acknowledgedAt: '2026-10-04T04:00:00Z')).acknowledged, isTrue);
      expect(DispatcherMessage.fromJson(_json(acknowledgedBy: '')).acknowledged, isFalse);
    });

    test('refuses a message without an id or a readable time', () {
      expect(() => DispatcherMessage.fromJson({..._json(), 'id': ''}), throwsFormatException);
      expect(() => DispatcherMessage.fromJson(_json(createdAt: 'yesterday')), throwsFormatException);
    });
  });

  group('MessagesApi', () {
    MessagesApi api(MockClient client) => MessagesApi(client: client, baseUrl: 'http://api/');

    test('lists the trip\'s messages with the bearer token, keeping the server order', () async {
      late http.Request seen;
      final messages = await api(MockClient((request) async {
        seen = request;
        return http.Response(jsonEncode({'items': [_json(id: 'a'), _json(id: 'b', acknowledgedBy: 'USR006')]}), 200);
      })).list('trip-1', 'abc');
      expect(seen.url.toString(), 'http://api/api/v1/delivery/trips/trip-1/messages');
      expect(seen.method, 'GET');
      expect(seen.headers['Authorization'], 'Bearer abc');
      expect([for (final m in messages) m.id], ['a', 'b']);
      expect(messages.last.acknowledged, isTrue);
    });

    test('acknowledges with a POST to the message', () async {
      late http.Request seen;
      await api(MockClient((request) async {
        seen = request;
        return http.Response(jsonEncode({'message': _json()}), 200);
      })).acknowledge('trip-1', 'm1', 'abc');
      expect(seen.method, 'POST');
      expect(seen.url.path, '/api/v1/delivery/trips/trip-1/messages/m1/ack');
      expect(seen.headers['Authorization'], 'Bearer abc');
    });

    test('maps HTTP and network errors to a failure kind', () async {
      Future<MessagesFailureKind> kind(http.Client client) async {
        try {
          await MessagesApi(client: client, baseUrl: 'http://api').list('t', 'x');
        } on MessagesFailure catch (failure) {
          return failure.kind;
        }
        fail('expected a failure');
      }

      expect(await kind(MockClient((_) async => http.Response('', 401))), MessagesFailureKind.unauthorized);
      expect(await kind(MockClient((_) async => http.Response('', 403))), MessagesFailureKind.forbidden);
      expect(await kind(MockClient((_) async => http.Response('', 404))), MessagesFailureKind.notFound);
      expect(await kind(MockClient((_) async => http.Response('', 503))), MessagesFailureKind.unavailable);
      expect(await kind(MockClient((_) async => http.Response('nope', 200))), MessagesFailureKind.unavailable);
      expect(await kind(MockClient((_) async => http.Response(jsonEncode({'items': [{'id': ''}]}), 200))), MessagesFailureKind.unavailable);
      expect(await kind(MockClient((_) async => throw const SocketException('down'))), MessagesFailureKind.unavailable);
    });
  });

  group('ApiMessageSource', () {
    ApiMessageSource source(MockClient client, _Auth auth) => ApiMessageSource(api: MessagesApi(client: client, baseUrl: 'http://api'), auth: auth);

    test('loads and acknowledges with the driver\'s token', () async {
      final s = source(MockClient((request) async => request.method == 'GET' ? http.Response(jsonEncode({'items': [_json()]}), 200) : http.Response('{}', 200)), _Auth());
      expect((await s.load('trip-1')).messages, hasLength(1));
      expect((await s.acknowledge('trip-1', 'm1')).isDone, isTrue);
    });

    test('a sign-in that is over is reported as expired, and a missing connection for a refresh is not', () async {
      final expired = await source(MockClient((_) async => http.Response('{}', 200)), _Auth(token: null)).load('t');
      expect(expired.signInExpired, isTrue);
      final auth = _Auth()..error = const AuthFailure(AuthFailureKind.unavailable);
      final offline = await source(MockClient((_) async => http.Response('{}', 200)), auth).load('t');
      expect(offline.signInExpired, isFalse);
      expect(offline.failure, contains('Could not reach Waypoint'));
      final ack = await source(MockClient((_) async => http.Response('{}', 200)), auth).acknowledge('t', 'm');
      expect(ack.isDone, isFalse);
      expect(ack.signInExpired, isFalse);
    });

    test('a rejected token and a trip with no run are explained', () async {
      final rejected = await source(MockClient((_) async => http.Response('', 401)), _Auth()).load('t');
      expect(rejected.signInExpired, isTrue);
      final missing = await source(MockClient((_) async => http.Response('', 404)), _Auth()).load('t');
      expect(missing.signInExpired, isFalse);
      expect(missing.failure, contains('not been started'));
    });
  });

  group('business clock', () {
    test('shows Colombo time whatever the phone timezone is', () {
      expect(businessClockLabel(DateTime.utc(2026, 10, 4, 3, 30)), '09:00');
      expect(businessClockLabel(DateTime.utc(2026, 10, 4, 18, 29)), '23:59');
      expect(businessClockLabel(DateTime.utc(2026, 10, 4, 18, 30)), '00:00');
    });
  });

  group('DriverSession messages', () {
    test('reads them when the route loads and lists unread ones first, newest first', () async {
      final source = _Source()
        ..messages = [
          _message('old-read', acknowledged: true, at: DateTime.utc(2026, 10, 4, 1)),
          _message('older-unread', at: DateTime.utc(2026, 10, 4, 2)),
          _message('newer-unread', at: DateTime.utc(2026, 10, 4, 3, 30)),
        ];
      final session = _session(source);
      addTearDown(session.dispose);
      await session.signIn();
      await _settle();
      expect(session.unreadMessages, 2);
      final items = session.visibleUpdates.where((item) => item.id?.startsWith(messageUpdatePrefix) ?? false).toList();
      expect([for (final item in items) item.id], ['msg:newer-unread', 'msg:older-unread', 'msg:old-read']);
      expect(items.first.title, contains('please acknowledge'));
      expect(items.last.title, 'Message from dispatch');
      expect(items.first.time, '09:00');
      expect(items.first.detail, 'Hello');
    });

    test('the first read is not news, but a message that arrives afterwards is', () async {
      final source = _Source()..messages = [_message('a')];
      final session = _session(source, poll: const Duration(milliseconds: 30));
      addTearDown(session.dispose);
      await session.signIn();
      await _settle();
      expect(session.newMessageSerial, 0, reason: 'what was already waiting on sign-in');
      source.messages = [_message('a'), _message('b')];
      await Future<void>.delayed(const Duration(milliseconds: 120));
      expect(session.newMessageSerial, 1);
      await Future<void>.delayed(const Duration(milliseconds: 120));
      expect(session.newMessageSerial, 1, reason: 'the same message is not announced twice');
    });

    test('a message that is already acknowledged is not announced', () async {
      final source = _Source();
      final session = _session(source, poll: const Duration(milliseconds: 30));
      addTearDown(session.dispose);
      await session.signIn();
      await _settle();
      source.messages = [_message('done', acknowledged: true)];
      await Future<void>.delayed(const Duration(milliseconds: 120));
      expect(session.newMessageSerial, 0);
      expect(session.messages, hasLength(1));
    });

    test('a failed read keeps what was already shown', () async {
      final source = _Source()..messages = [_message('a')];
      final session = _session(source);
      addTearDown(session.dispose);
      await session.signIn();
      await _settle();
      source.failure = const MessageLoad.failed('Could not reach Waypoint. Check your connection and try again.');
      await session.refreshMessages();
      expect(session.messages.map((m) => m.id), ['a']);
      expect(session.signedIn, isTrue);
    });

    test('a sign-in that is over during a read returns to sign-in', () async {
      final source = _Source();
      final auth = _Auth();
      final session = _session(source, auth: auth);
      addTearDown(session.dispose);
      await session.signIn();
      await _settle();
      source.failure = const MessageLoad.failed('Your sign-in expired. Sign in again.', signInExpired: true);
      await session.refreshMessages();
      expect(session.signedIn, isFalse);
      expect(auth.signOuts, 1);
    });

    test('acknowledging marks it read and tells the server once', () async {
      final source = _Source()..messages = [_message('a')];
      final session = _session(source);
      addTearDown(session.dispose);
      await session.signIn();
      await _settle();
      expect(await session.acknowledgeMessage('a'), isNull);
      expect(session.unreadMessages, 0);
      expect(source.acked, ['a']);
      expect(await session.acknowledgeMessage('a'), isNull);
      expect(source.acked, ['a'], reason: 'an acknowledged message is not sent again');
    });

    test('a failed acknowledgement keeps it unread and says why', () async {
      final source = _Source()
        ..messages = [_message('a')]
        ..ackResult = const MessageAck.failed('Could not reach Waypoint. Check your connection and try again.');
      final session = _session(source);
      addTearDown(session.dispose);
      await session.signIn();
      await _settle();
      expect(await session.acknowledgeMessage('a'), contains('Could not reach Waypoint'));
      expect(session.unreadMessages, 1);
      expect(await session.acknowledgeMessage('missing'), contains('no longer available'));
    });

    test('signing out stops reading and forgets the messages', () async {
      final source = _Source()..messages = [_message('a')];
      final session = _session(source, poll: const Duration(milliseconds: 30));
      addTearDown(session.dispose);
      await session.signIn();
      await _settle();
      expect(session.messages, hasLength(1));
      await session.finishTrip();
      expect(session.messages, isEmpty);
      final loads = source.loads;
      await Future<void>.delayed(const Duration(milliseconds: 150));
      expect(source.loads, loads, reason: 'no more reads after signing out');
    });

    test('a session without a message source has none and does nothing', () async {
      final session = DriverSession(database: InMemoryLocalDatabase(), queue: InMemorySyncQueue(), auth: _Auth(), trips: _Trips());
      addTearDown(session.dispose);
      await session.signIn();
      await session.refreshMessages();
      expect(session.messages, isEmpty);
      expect(await session.acknowledgeMessage('x'), contains('no longer available'));
    });
  });

  group('screens', () {
    Future<_Source> boot(WidgetTester tester, {List<DispatcherMessage> messages = const [], Duration poll = const Duration(hours: 1)}) async {
      final source = _Source()..messages = List.of(messages);
      final auth = _Auth();
      await pumpScreen(
        tester,
        WaypointDriverApp(database: InMemoryLocalDatabase(), queue: InMemorySyncQueue(), auth: auth, trips: _Trips(), messageSource: source),
      );
      await tester.tap(find.text('Continue to sign in'));
      await tester.pumpAndSettle();
      // The fake trip is already started, so there is no load check to confirm.
      return source;
    }

    Future<void> openMessage(WidgetTester tester) async {
      await tester.tap(find.text('Updates'));
      await tester.pumpAndSettle();
      await tester.tap(find.textContaining('Message from dispatch').first);
      await tester.pumpAndSettle();
    }

    testWidgets('an unread message is in Updates, opens in full and is acknowledged from there', (tester) async {
      final source = await boot(tester, messages: [_message('a', body: 'Gate B is closed, use gate A', stopId: 'stop-1')]);
      await openMessage(tester);
      expect(find.text('Gate B is closed, use gate A'), findsWidgets);
      expect(find.textContaining('About stop 1 - Dehiwala'), findsOneWidget);
      expect(find.text('Acknowledge'), findsOneWidget);
      await tester.tap(find.text('Acknowledge'));
      await tester.pumpAndSettle();
      expect(source.acked, ['a']);
      expect(find.text('Acknowledge'), findsNothing);
      expect(find.text('Message from dispatch'), findsOneWidget, reason: 'now listed as read, no longer asking');
      expect(find.textContaining('please acknowledge'), findsNothing);
    });

    testWidgets('a failed acknowledgement is explained and can be tried again', (tester) async {
      final source = await boot(tester, messages: [_message('a')]);
      source.ackResult = const MessageAck.failed('Could not reach Waypoint. Check your connection and try again.');
      await openMessage(tester);
      await tester.tap(find.text('Acknowledge'));
      await tester.pumpAndSettle();
      expect(find.textContaining('Could not reach Waypoint'), findsOneWidget);
      expect(find.text('Acknowledge'), findsOneWidget);
      source.ackResult = null;
      await tester.tap(find.text('Acknowledge'));
      await tester.pumpAndSettle();
      expect(find.text('Acknowledge'), findsNothing);
      expect(source.acked, ['a', 'a']);
    });

    testWidgets('a message about the whole trip says so, and an acknowledged one cannot be acknowledged again', (tester) async {
      await boot(tester, messages: [_message('a', acknowledged: true, body: 'Thanks, noted')]);
      await openMessage(tester);
      expect(find.textContaining('About the whole trip'), findsOneWidget);
      expect(find.text('You acknowledged this message.'), findsOneWidget);
      expect(find.text('Acknowledge'), findsNothing);
    });
  });
}
