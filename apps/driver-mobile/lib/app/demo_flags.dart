import 'package:flutter/foundation.dart';

/// Switches that make the app act on sample data or skip real sign-in. They exist so the screens
/// can be shown and tested, and they are always off in a release build, whatever was passed to
/// `--dart-define`, so a distributable app can never enter the workflow without real sign-in.
class DemoFlags {
  const DemoFlags({this.auth = false, this.route = false, this.updates = false});

  /// Let any credentials in when no identity provider is configured (`DEMO_AUTH`).
  final bool auth;

  /// Show the sample route to a signed-in driver (`DEMO_ROUTE`).
  final bool route;

  /// Add a sample plan update so the plan review screen can be opened (`DEMO_UPDATES`).
  final bool updates;

  factory DemoFlags.forBuild({required bool releaseMode, bool auth = false, bool route = false, bool updates = false}) =>
      releaseMode ? const DemoFlags() : DemoFlags(auth: auth, route: route, updates: updates);

  factory DemoFlags.fromEnvironment() => DemoFlags.forBuild(
        releaseMode: kReleaseMode,
        auth: const bool.fromEnvironment('DEMO_AUTH'),
        route: const bool.fromEnvironment('DEMO_ROUTE'),
        updates: const bool.fromEnvironment('DEMO_UPDATES'),
      );
}
