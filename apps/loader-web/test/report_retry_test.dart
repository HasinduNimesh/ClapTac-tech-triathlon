import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:waypoint_loader/api/api_client.dart';
import 'package:waypoint_loader/loader/loader_controller.dart';
import 'package:waypoint_loader/shared/models.dart';

const base = 'http://api.test/api/v1';

LoadingTrip trip() => LoadingTrip({
      'tripId': 't1',
      'status': 'in_progress',
      'loadingStatus': 'in_progress',
      'planVersion': 1,
      'orders': [
        {'orderId': 'o1', 'orderRef': 'ORD1', 'status': 'pending', 'expectedUnits': 10, 'stopSequence': 1, 'issues': <dynamic>[]},
      ],
    });

/// A loading service that files reports and takes photos, and can be told to fail.
class FakeLoading {
  final List<http.BaseRequest> calls = [];
  final List<String> issueKeys = [];
  int photoFailures = 0; // fail this many photo uploads before accepting one
  int lostResponses = 0; // file the report but lose the answer this many times
  int filed = 0;
  String? idempotencyKeyFiled;

  MockClient get client => MockClient.streaming((request, body) async {
        calls.add(request);
        final path = request.url.path;
        final bytes = await body.toBytes();
        http.StreamedResponse reply(int status, Object json) => http.StreamedResponse(Stream.value(utf8.encode(jsonEncode(json))), status, headers: {'content-type': 'application/json'});
        if (request.method == 'POST' && path.endsWith('/orders/o1/issues')) {
          final key = request.headers['Idempotency-Key']!;
          issueKeys.add(key);
          if (idempotencyKeyFiled != key) {
            filed++;
            idempotencyKeyFiled = key;
          }
          if (lostResponses > 0) {
            lostResponses--;
            throw http.ClientException('connection dropped');
          }
          return reply(201, {'issue': {'id': 'iss-1'}});
        }
        if (request.method == 'POST' && path.endsWith('/issues/iss-1/photo')) {
          if (photoFailures > 0) {
            photoFailures--;
            return reply(500, {'detail': 'storage unavailable'});
          }
          expect(bytes, isNotEmpty);
          return reply(200, {'issue': {'id': 'iss-1', 'hasPhoto': true}});
        }
        if (request.method == 'GET' && path == '/api/v1/loading/trips/t1') return reply(200, {'tripId': 't1', 'status': 'in_progress', 'orders': <dynamic>[]});
        return reply(404, {'detail': 'not found'});
      });

  int count(String suffix) => calls.where((c) => c.url.path.endsWith(suffix)).length;
}

LoaderController controllerFor(FakeLoading server) => LoaderController(api: ApiClient(tokenProvider: () async => 'tok', client: server.client, baseUrl: base));

void main() {
  final photo = [0xff, 0xd8, 0xff, 0xe0, 1, 2, 3];

  test('a failed photo upload is retried on its own: the report is filed once', () async {
    final server = FakeLoading()..photoFailures = 1;
    final c = controllerFor(server);
    final attempt = ReportAttempt();

    final first = await c.reportIssue(trip(), trip().orders.single, attempt: attempt, type: 'DAMAGED', units: 3, photo: photo);
    expect(first, isNotNull, reason: 'the loader is told the upload failed');
    expect(attempt.filed, isTrue, reason: 'the report itself exists');
    expect(attempt.photoSent, isFalse);

    final second = await c.reportIssue(trip(), trip().orders.single, attempt: attempt, type: 'DAMAGED', units: 3, photo: photo);
    expect(second, isNull);
    expect(attempt.photoSent, isTrue);
    expect(server.count('/orders/o1/issues'), 1, reason: 'a retry must not file a second report');
    expect(server.count('/issues/iss-1/photo'), 2);
  });

  test('once the photo is up, sending again does nothing more', () async {
    final server = FakeLoading();
    final c = controllerFor(server);
    final attempt = ReportAttempt();
    expect(await c.reportIssue(trip(), trip().orders.single, attempt: attempt, type: 'MISSING', units: 1, photo: photo), isNull);
    expect(await c.reportIssue(trip(), trip().orders.single, attempt: attempt, type: 'MISSING', units: 1, photo: photo), isNull);
    expect(server.count('/orders/o1/issues'), 1);
    expect(server.count('/issues/iss-1/photo'), 1);
  });

  test('a lost answer to the report is retried with the same key, so the server returns the report it already filed', () async {
    final server = FakeLoading()..lostResponses = 1;
    final c = controllerFor(server);
    final attempt = ReportAttempt();

    expect(await c.reportIssue(trip(), trip().orders.single, attempt: attempt, type: 'MISSING', units: 2), isNotNull);
    expect(attempt.filed, isFalse, reason: 'the loader did not get the answer, so the app cannot know it was filed');
    expect(await c.reportIssue(trip(), trip().orders.single, attempt: attempt, type: 'MISSING', units: 2), isNull);

    expect(server.issueKeys, hasLength(2));
    expect(server.issueKeys.toSet(), hasLength(1), reason: 'both sends carry one idempotency key');
    expect(server.filed, 1, reason: 'the server dedupes on that key');
  });

  test('a report without a photo needs no upload', () async {
    final server = FakeLoading();
    final c = controllerFor(server);
    expect(await c.reportIssue(trip(), trip().orders.single, attempt: ReportAttempt(), type: 'MISSING', units: 1), isNull);
    expect(server.count('/photo'), 0);
  });

  test('each form is its own report: a new form gets a new key', () {
    expect(ReportAttempt().key, isNot(ReportAttempt().key));
  });

  group('a request refused with 401', () {
    MockClient answering(List<String> tokens) => MockClient((req) async {
          tokens.add(req.headers['Authorization']!);
          return tokens.length == 1 ? http.Response('{}', 401) : http.Response('{"ok":true}', 200);
        });

    test('is repeated once with a renewed token', () async {
      final seen = <String>[];
      var token = 'old';
      final api = ApiClient(
        tokenProvider: () async => token,
        onUnauthorized: () async {
          token = 'new';
          return true;
        },
        client: answering(seen),
        baseUrl: base,
      );
      expect(await api.get('/loading/trips'), {'ok': true});
      expect(seen, ['Bearer old', 'Bearer new']);
    });

    test('fails when no new token can be had', () async {
      final api = ApiClient(tokenProvider: () async => 'old', onUnauthorized: () async => false, client: answering([]), baseUrl: base);
      await expectLater(api.get('/loading/trips'), throwsA(isA<ApiException>().having((e) => e.status, 'status', 401)));
    });
  });
}
