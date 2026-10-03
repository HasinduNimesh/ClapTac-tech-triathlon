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

  /// The access token for API calls. It is refreshed first when it has expired or is about to, if the
  /// identity provider issued a refresh token.
  ///
  /// Null means the driver has to sign in again: there is no session, or the token expired and cannot
  /// be refreshed (no refresh token, or the provider refused it). Throws [AuthFailure] with kind
  /// `unavailable` when a refresh was needed but Waypoint could not be reached: the session is intact
  /// and the caller should treat it like any other connection problem and try again later.
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

  /// How long before expiry a token is treated as expired, so it is not used mid-request.
  static const expirySkew = Duration(seconds: 60);

  /// The call in progress, shared so simultaneous callers (the sync worker, loading the route) get one
  /// answer and make at most one refresh request. Providers that rotate refresh tokens would reject
  /// the second request.
  Future<String?>? _pending;

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
    if (!stored.profile.isDriver) {
      await _store.clear();
      return null;
    }
    // An expired access token with a refresh token is still a session: the driver can open the app
    // and work without signal, and the token is refreshed when something needs it.
    if (stored.tokens.isExpired(_clock()) && !stored.tokens.canRefresh) {
      await _store.clear();
      return null;
    }
    return stored.profile;
  }

  @override
  Future<String?> accessToken() => _pending ??= _currentAccessToken().whenComplete(() => _pending = null);

  Future<String?> _currentAccessToken() async {
    final stored = await _store.read();
    if (stored == null) return null;
    if (!stored.tokens.isExpired(_clock(), skew: expirySkew)) return stored.tokens.accessToken;
    if (!stored.tokens.canRefresh) return null;
    return _refresh(stored);
  }

  Future<String?> _refresh(StoredAuth stored) async {
    try {
      final tokens = await _client.refresh(stored.tokens);
      await _store.write(StoredAuth(tokens: tokens, profile: stored.profile));
      return tokens.accessToken;
    } on AuthFailure catch (failure) {
      if (failure.kind == AuthFailureKind.unauthorized) {
        // The provider no longer accepts this refresh token: the session is over.
        await _store.clear();
        return null;
      }
      rethrow;
    }
  }

  @override
  Future<void> signOut() => _store.clear();
}
