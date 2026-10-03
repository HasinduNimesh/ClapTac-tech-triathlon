import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:waypoint_loader/auth/auth.dart';
import 'package:waypoint_loader/auth/sign_in_screen.dart';
import 'package:waypoint_loader/api/api_client.dart';
import 'package:waypoint_loader/shared/models.dart';
import 'package:waypoint_loader/theme/tokens.dart';

void main() {
  testWidgets('sign-in screen explains the app is online-only', (tester) async {
    await tester.pumpWidget(MaterialApp(theme: waypointTheme(), home: SignInScreen(auth: AuthService())));
    expect(find.text('Sign in'), findsWidgets);
    expect(find.textContaining('works online only'), findsOneWidget);
  });

  test('a shortfall only allows departure after a partial-load or move decision', () {
    LoadingOrder order(String? decision) => LoadingOrder({
          'orderId': 'o1',
          'status': 'shortfall',
          'issues': [
            {'id': 'i1', 'type': 'MISSING', 'affectedUnits': 2, if (decision != null) 'decision': decision},
          ],
        });
    expect(order(null).unresolved, isTrue);
    expect(order('HOLD').unresolved, isTrue);
    expect(order('PARTIAL_LOAD').unresolved, isFalse);
    expect(order('MOVE_TO_NEXT_RUN').unresolved, isFalse);
  });

  test('server problem types become loader-facing messages', () {
    final e = ApiException(409, '{"type":"dispatcher_decision_required","detail":"x"}');
    expect(e.type, 'dispatcher_decision_required');
  });
}
