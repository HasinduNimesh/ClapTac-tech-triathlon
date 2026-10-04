enum AuthFailureKind { notConfigured, cancelled, unauthorized, notProvisioned, accessDenied, unavailable }

/// Why a sign-in did not result in a signed-in driver.
class AuthFailure implements Exception {
  const AuthFailure(this.kind);

  final AuthFailureKind kind;

  /// What to tell the driver. Null for a cancelled sign-in, which needs no message.
  String? get message {
    switch (kind) {
      case AuthFailureKind.notConfigured:
        return 'Sign-in is not available yet. Staff accounts are not connected to this build.';
      case AuthFailureKind.cancelled:
        return null;
      case AuthFailureKind.unauthorized:
        return 'Your sign-in was not accepted. Please try again.';
      case AuthFailureKind.notProvisioned:
        return 'Your account is not set up in Waypoint yet. Ask your dispatcher or administrator to add it.';
      case AuthFailureKind.accessDenied:
        return 'This app is for drivers. Your account does not have the driver role.';
      case AuthFailureKind.unavailable:
        return 'Could not reach Waypoint. Check your connection and try again.';
    }
  }

  @override
  String toString() => 'AuthFailure(${kind.name})';
}
