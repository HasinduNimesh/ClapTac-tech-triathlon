import 'dart:convert';

/// The access token the Waypoint API expects, and when it stops working.
class OidcTokens {
  const OidcTokens({required this.accessToken, required this.expiresAt});

  factory OidcTokens.fromJson(Map<String, Object?> json) => OidcTokens(
        accessToken: json['accessToken']! as String,
        expiresAt: DateTime.fromMillisecondsSinceEpoch(json['expiresAt']! as int, isUtc: true),
      );

  final String accessToken;
  final DateTime expiresAt;

  bool isExpired(DateTime now) => !expiresAt.isAfter(now.toUtc());

  Map<String, Object?> toJson() => {'accessToken': accessToken, 'expiresAt': expiresAt.millisecondsSinceEpoch};

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
