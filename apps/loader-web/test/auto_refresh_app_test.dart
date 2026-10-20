import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:waypoint_loader/api/api_client.dart';
import 'package:waypoint_loader/auth/auth.dart';
import 'package:waypoint_loader/auth/browser_stub.dart';
import 'package:waypoint_loader/auth/oidc_config.dart';
import 'package:waypoint_loader/main.dart';
import 'package:waypoint_loader/shared/models.dart';

const apiBase = 'http://api.test/api/v1';

void main() {
  testWidgets('the signed-in app re-reads the day by itself', (tester) async {
    tester.view.physicalSize = const Size(1400, 1000);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);

    var tripListCalls = 0;
    final server = MockClient((http.Request req) async {
      final path = req.url.path;
      if (path.endsWith('/loading/trips')) {
        tripListCalls++;
        return http.Response(jsonEncode({'items': <dynamic>[]}), 200);
      }
      if (path.endsWith('/loading/alerts')) return http.Response(jsonEncode({'items': <dynamic>[]}), 200);
      return http.Response('not found', 404);
    });
    final session = jsonEncode(AuthSession(
      accessToken: 'tok',
      refreshToken: 'ref',
      profile: Profile(userId: 'USR004', roles: ['LOADER'], depot: 'DEPOT_NORTH', displayName: 'Nimal Perera'),
      expiresAt: DateTime.now().add(const Duration(hours: 2)),
    ).toJson());
    final browser = MemoryBrowser(location: Uri.parse('https://app.example.test/loader-app/'), origin: 'https://app.example.test')
      ..storage['waypoint.loader.session'] = session;
    final auth = AuthService(config: const OidcConfig(issuer: 'https://id.example.test'), client: server, browser: browser, apiBase: apiBase, autoRenew: false);
    // As in the browser: the app is on screen first, and the saved sign-in is restored afterwards.
    await tester.pumpWidget(WaypointLoaderApp(
      auth: auth,
      navigatorKey: GlobalKey<NavigatorState>(),
      apiClientFor: (a) => ApiClient(tokenProvider: a.validToken, client: server, baseUrl: apiBase),
    ));
    await tester.pump();
    await tester.runAsync(() => auth.start());
    await tester.pump();
    await tester.pump(const Duration(seconds: 1));
    final afterOpen = tripListCalls;
    expect(afterOpen, greaterThanOrEqualTo(1), reason: 'the first load');

    await tester.pump(const Duration(seconds: 46));
    await tester.pump();
    expect(tripListCalls, greaterThan(afterOpen), reason: 'a refresh should have happened after 45 s');
  });
}
