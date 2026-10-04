import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:http/http.dart' as http;

import 'auth_failure.dart';

/// The application profile Waypoint holds for a signed-in person
/// (`GET /api/v1/shared/profiles/me`). Role and vehicle are decided by the server.
class DriverProfile {
  const DriverProfile({
    required this.userId,
    required this.subject,
    required this.roles,
    this.vehicleId,
    this.depot,
    this.outletIds = const [],
  });

  factory DriverProfile.fromJson(Map<String, Object?> json) => DriverProfile(
        userId: json['userId'] as String? ?? '',
        subject: json['subject'] as String? ?? '',
        roles: [for (final role in (json['roles'] as List<Object?>? ?? const [])) role.toString()],
        vehicleId: json['vehicleId'] as String?,
        depot: json['depot'] as String?,
        outletIds: [for (final id in (json['outletIds'] as List<Object?>? ?? const [])) id.toString()],
      );

  final String userId;
  final String subject;
  final List<String> roles;
  final String? vehicleId;
  final String? depot;
  final List<String> outletIds;

  bool get isDriver => roles.any((role) => role.toUpperCase() == 'DRIVER');

  Map<String, Object?> toJson() => {
        'userId': userId,
        'subject': subject,
        'roles': roles,
        if (vehicleId != null) 'vehicleId': vehicleId,
        if (depot != null) 'depot': depot,
        'outletIds': outletIds,
      };
}

class ProfileApi {
  ProfileApi({required http.Client client, required String baseUrl, this.timeout = const Duration(seconds: 15)})
      : _client = client,
        _baseUrl = baseUrl.replaceAll(RegExp(r'/+$'), '');

  final http.Client _client;
  final String _baseUrl;
  final Duration timeout;

  /// Throws [AuthFailure]: unauthorized (401), notProvisioned (404), unavailable (anything else).
  Future<DriverProfile> fetchMe(String accessToken) async {
    final http.Response response;
    try {
      response = await _client.get(
        Uri.parse('$_baseUrl/api/v1/shared/profiles/me'),
        headers: {'Authorization': 'Bearer $accessToken', 'Accept': 'application/json'},
      ).timeout(timeout);
    } on SocketException {
      throw const AuthFailure(AuthFailureKind.unavailable);
    } on TimeoutException {
      throw const AuthFailure(AuthFailureKind.unavailable);
    } on http.ClientException {
      throw const AuthFailure(AuthFailureKind.unavailable);
    }
    switch (response.statusCode) {
      case 200:
        try {
          final body = jsonDecode(response.body);
          // The API answers {"profile": {...}}.
          final profile = body is Map<String, Object?> ? body['profile'] : null;
          if (profile is Map<String, Object?>) return DriverProfile.fromJson(profile);
        } on FormatException {
          // Fall through to unavailable.
        }
        throw const AuthFailure(AuthFailureKind.unavailable);
      case 401:
        throw const AuthFailure(AuthFailureKind.unauthorized);
      case 404:
        throw const AuthFailure(AuthFailureKind.notProvisioned);
      default:
        throw const AuthFailure(AuthFailureKind.unavailable);
    }
  }
}
