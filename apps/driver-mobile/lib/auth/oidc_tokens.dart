import 'dart:convert';

/// The access token the Waypoint API expects, when it stops working, and, when the identity provider
/// issued one, the refresh token that gets a new access token without asking for the password again.
class OidcTokens {
  const OidcTokens({required this.accessToken, required this.expiresAt, this.refreshToken});

  /// Older stored sessions have no `refreshToken`; they read back as having none.
  factory OidcTokens.fromJson(Map<String, Object?> json) => OidcTokens(
        accessToken: json['accessToken']! as String,
        expiresAt: DateTime.fromMillisecondsSinceEpoch(json['expiresAt']! as int, isUtc: true),
        refreshToken: json['refreshToken'] as String?,
      );

  final String accessToken;
  final DateTime expiresAt;
  final String? refreshToken;

  bool get canRefresh => refreshToken != null && refreshToken!.isNotEmpty;

  /// Whether the access token is, or within [skew] of being, no longer accepted. The skew keeps a
  /// token that would run out mid-request from being used.
  bool isExpired(DateTime now, {Duration skew = Duration.zero}) => !expiresAt.isAfter(now.toUtc().add(skew));

  Map<String, Object?> toJson() => {
        'accessToken': accessToken,
        'expiresAt': expiresAt.millisecondsSinceEpoch,
        if (refreshToken != null) 'refreshToken': refreshToken,
      };

  /// Reads the `sub` claim for bookkeeping only. The server verifies the token; the app
  /// never trusts these claims for authorization.
  static String? unverifiedSubject(String jwt) {
    final parts = jwt.split('.');
    if (parts.length != 3) return null;
    try {
      final payload = utf8.decode(base64Url.decode(base64Url.normalize(parts[1])));
      final claims = jsonDecode(payload);
      return claims is Map<String, Object?> ? claims['sub'] as String? : null;
    } on FormatException {
      return null;
    }
  }
}
