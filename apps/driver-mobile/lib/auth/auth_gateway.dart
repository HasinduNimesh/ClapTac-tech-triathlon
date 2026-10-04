import 'dart:async';

import 'auth_config.dart';
import 'auth_failure.dart';
import 'auth_store.dart';
import 'oidc_client.dart';
import 'profile_api.dart';
import 'revocation_queue.dart';
import 'token_revoker.dart';

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

/// Gateways that remember refresh tokens still to be revoked at the provider and can try again.
abstract class RevocationRetry {
  /// Tries again to revoke the tokens whose revocation could not be done when the driver signed out.
  /// Never throws: a failure just leaves them for the next try.
  Future<void> retryPendingRevocations();
}

class OidcAuthGateway implements AuthGateway, RevocationRetry {
  OidcAuthGateway({
    required AuthConfig config,
    required OidcClient client,
    required ProfileApi profiles,
    required AuthStore store,
    TokenRevoker? revoker,
    RevocationQueue? pendingRevocations,
    DateTime Function()? clock,
  })  : _config = config,
        _client = client,
        _profiles = profiles,
        _store = store,
        _revoker = revoker,
        _pendingRevocations = pendingRevocations,
        _clock = clock ?? DateTime.now;

  final AuthConfig _config;
  final OidcClient _client;
  final ProfileApi _profiles;
  final AuthStore _store;
  final TokenRevoker? _revoker;
  final RevocationQueue? _pendingRevocations;
  final DateTime Function() _clock;
  bool _retrying = false;

  /// How long before expiry a token is treated as expired, so it is not used mid-request.
  static const expirySkew = Duration(seconds: 60);

  /// The call in progress, shared so simultaneous callers (the sync worker, loading the route) get one
  /// answer and make at most one refresh request. Providers that rotate refresh tokens would reject
  /// the second request.
  Future<String?>? _pending;

  /// Changes whenever the stored session is replaced or removed (sign-in, sign-out, a refused refresh
  /// token). A refresh that was already under way remembers the value it started with and, if it has
  /// changed by the time the provider answers, throws its result away instead of writing an old
  /// session back over a sign-out or a new sign-in.
  int _sessionGeneration = 0;

  @override
  Future<AuthOutcome> signIn() async {
    if (!_config.isConfigured) return const AuthOutcome.failed(AuthFailure(AuthFailureKind.notConfigured));
    try {
      final tokens = await _client.signIn();
      if (tokens == null) return const AuthOutcome.failed(AuthFailure(AuthFailureKind.cancelled));

      // The server decides who this person is and what they may do.
      final profile = await _profiles.fetchMe(tokens.accessToken);
      if (!profile.isDriver) {
        // Not a driver: the tokens it was given are of no use here, so they are discarded and revoked.
        _sessionGeneration++;
        await _discard(tokens.refreshToken);
        await _store.clear();
        return const AuthOutcome.failed(AuthFailure(AuthFailureKind.accessDenied));
      }
      _sessionGeneration++;
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
      _sessionGeneration++;
      await _store.clear();
      return null;
    }
    // An expired access token with a refresh token is still a session: the driver can open the app
    // and work without signal, and the token is refreshed when something needs it.
    if (stored.tokens.isExpired(_clock()) && !stored.tokens.canRefresh) {
      _sessionGeneration++;
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
    final generation = _sessionGeneration;
    try {
      final tokens = await _client.refresh(stored.tokens);
      // The driver signed out, or someone else signed in, while the provider was answering: this
      // session is gone, so its new tokens must not be written back.
      if (generation != _sessionGeneration) return null;
      await _store.write(StoredAuth(tokens: tokens, profile: stored.profile));
      return tokens.accessToken;
    } on AuthFailure catch (failure) {
      if (failure.kind == AuthFailureKind.unauthorized) {
        // The provider no longer accepts this refresh token: the session is over. If it was already
        // replaced meanwhile, the newer session is not touched.
        if (generation == _sessionGeneration) {
          _sessionGeneration++;
          await _store.clear();
        }
        return null;
      }
      rethrow;
    }
  }

  /// Signs out on this phone at once and always, then asks the provider to revoke the refresh token.
  ///
  /// The refresh token is first written to a small pending list, so that if the phone has no signal (or
  /// the app is closed before the provider answers) the revocation can be repeated later. The sign-out
  /// never waits for the provider and never fails because of it.
  @override
  Future<void> signOut() async {
    _sessionGeneration++;
    String? refreshToken;
    try {
      refreshToken = (await _store.read())?.tokens.refreshToken;
    } on Object {
      refreshToken = null;
    }
    await _discard(refreshToken);
    await _store.clear();
  }

  /// Queues [refreshToken] for revocation and starts trying, without waiting. Revocation is best effort.
  Future<void> _discard(String? refreshToken) async {
    final revoker = _revoker;
    final pending = _pendingRevocations;
    if (revoker == null || pending == null || refreshToken == null || refreshToken.isEmpty) return;
    try {
      await pending.add(refreshToken);
    } on Object {
      return;
    }
    unawaited(_revoke(refreshToken));
  }

  Future<void> _revoke(String refreshToken) async {
    final revoker = _revoker;
    final pending = _pendingRevocations;
    if (revoker == null || pending == null) return;
    try {
      final outcome = await revoker.revokeRefreshToken(refreshToken);
      // Done, refused for good, or not offered by this provider: nothing more can be gained by keeping it.
      if (outcome != RevokeOutcome.retryLater) await pending.remove(refreshToken);
    } on Object {
      // Left in the list for the next try.
    }
  }

  @override
  Future<void> retryPendingRevocations() async {
    final pending = _pendingRevocations;
    if (_revoker == null || pending == null || _retrying) return;
    _retrying = true;
    try {
      for (final token in await pending.read()) {
        await _revoke(token);
      }
    } on Object {
      // Nothing to do: the next try picks them up.
    } finally {
      _retrying = false;
    }
  }
}
