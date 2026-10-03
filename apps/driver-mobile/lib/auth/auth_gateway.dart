import 'auth_config.dart';
import 'auth_failure.dart';
import 'auth_store.dart';
import 'oidc_client.dart';
import 'profile_api.dart';

class AuthOutcome {
  const AuthOutcome.signedIn(DriverProfile this.profile) : failure = null;
  const AuthOutcome.failed(AuthFailure this.failure) : profile = null;

  final DriverProfile? profile;
  final AuthFailure? failure;
}

/// What the app needs from authentication, so screens and the session do not depend on
/// the OIDC library.
abstract class AuthGateway {
  Future<AuthOutcome> signIn();

  /// A previous sign-in that is still valid, found without the network.
  Future<DriverProfile?> restore();

  Future<void> signOut();

  /// The access token for API calls, or null when there is none or it has expired.
  Future<String?> accessToken();
}

class OidcAuthGateway implements AuthGateway {
  OidcAuthGateway({
    required AuthConfig config,
    required OidcClient client,
    required ProfileApi profiles,
    required AuthStore store,
    DateTime Function()? clock,
  })  : _config = config,
        _client = client,
        _profiles = profiles,
        _store = store,
        _clock = clock ?? DateTime.now;

  final AuthConfig _config;
  final OidcClient _client;
  final ProfileApi _profiles;
  final AuthStore _store;
  final DateTime Function() _clock;

  @override
  Future<AuthOutcome> signIn() async {
    if (!_config.isConfigured) return const AuthOutcome.failed(AuthFailure(AuthFailureKind.notConfigured));
    try {
      final tokens = await _client.signIn();
      if (tokens == null) return const AuthOutcome.failed(AuthFailure(AuthFailureKind.cancelled));

      // The server decides who this person is and what they may do.
      final profile = await _profiles.fetchMe(tokens.accessToken);
      if (!profile.isDriver) {
        await _store.clear();
        return const AuthOutcome.failed(AuthFailure(AuthFailureKind.accessDenied));
      }
      await _store.write(StoredAuth(tokens: tokens, profile: profile));
      return AuthOutcome.signedIn(profile);
    } on AuthFailure catch (failure) {
      return AuthOutcome.failed(failure);
    }
  }

  @override
  Future<DriverProfile?> restore() async {
    final stored = await _store.read();
    if (stored == null) return null;
    if (stored.tokens.isExpired(_clock()) || !stored.profile.isDriver) {
      await _store.clear();
      return null;
    }
    return stored.profile;
  }

  @override
  Future<String?> accessToken() async {
    final stored = await _store.read();
    if (stored == null || stored.tokens.isExpired(_clock())) return null;
    return stored.tokens.accessToken;
  }

  @override
  Future<void> signOut() => _store.clear();
}
