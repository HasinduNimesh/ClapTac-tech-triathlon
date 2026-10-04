import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:http/http.dart' as http;

import '../auth/auth_failure.dart';
import '../auth/auth_gateway.dart';

/// A message dispatch sent to the driver about this trip, optionally about one stop
/// (`GET /api/v1/delivery/trips/{id}/messages`).
class DispatcherMessage {
  const DispatcherMessage({
    required this.id,
    required this.tripId,
    required this.body,
    required this.sentBy,
    required this.createdAt,
    this.stopId = '',
    this.acknowledged = false,
  });

  factory DispatcherMessage.fromJson(Map<String, Object?> json) {
    final id = json['id'] as String? ?? '';
    final created = DateTime.tryParse(json['createdAt'] as String? ?? '');
    if (id.isEmpty || created == null) throw const FormatException('message id or time');
    final acknowledgedBy = (json['acknowledgedBy'] as String?) ?? '';
    return DispatcherMessage(
      id: id,
      tripId: json['tripId'] as String? ?? '',
      stopId: json['stopId'] as String? ?? '',
      body: (json['body'] as String?) ?? '',
      sentBy: (json['sentBy'] as String?) ?? '',
      createdAt: created.toUtc(),
      acknowledged: acknowledgedBy.isNotEmpty || json['acknowledgedAt'] != null,
    );
  }

  final String id;
  final String tripId;

  /// The stop the message is about, or empty when it is about the whole trip.
  final String stopId;
  final String body;
  final String sentBy;
  final DateTime createdAt;
  final bool acknowledged;

  DispatcherMessage asAcknowledged() => DispatcherMessage(id: id, tripId: tripId, stopId: stopId, body: body, sentBy: sentBy, createdAt: createdAt, acknowledged: true);
}

enum MessagesFailureKind { unauthorized, forbidden, notFound, unavailable }

class MessagesFailure implements Exception {
  const MessagesFailure(this.kind);

  final MessagesFailureKind kind;

  @override
  String toString() => 'MessagesFailure($kind)';
}

class MessagesApi {
  MessagesApi({required http.Client client, required String baseUrl, this.timeout = const Duration(seconds: 20)})
      : _client = client,
        _baseUrl = baseUrl.replaceAll(RegExp(r'/+$'), '');

  final http.Client _client;
  final String _baseUrl;
  final Duration timeout;

  /// The trip's messages, oldest first.
  Future<List<DispatcherMessage>> list(String tripId, String accessToken) async {
    final body = await _send('GET', '/api/v1/delivery/trips/${Uri.encodeComponent(tripId)}/messages', accessToken);
    final items = body is Map<String, Object?> ? body['items'] : null;
    if (items is! List) throw const MessagesFailure(MessagesFailureKind.unavailable);
    try {
      return [for (final item in items) if (item is Map<String, Object?>) DispatcherMessage.fromJson(item)];
    } on FormatException {
      throw const MessagesFailure(MessagesFailureKind.unavailable);
    }
  }

  /// Marks the message as read by this driver. The server repeats this harmlessly, so a retry after
  /// a lost response is fine.
  Future<void> acknowledge(String tripId, String messageId, String accessToken) async {
    await _send('POST', '/api/v1/delivery/trips/${Uri.encodeComponent(tripId)}/messages/${Uri.encodeComponent(messageId)}/ack', accessToken);
  }

  Future<Object?> _send(String method, String path, String accessToken) async {
    final request = http.Request(method, Uri.parse('$_baseUrl$path'))..headers.addAll({'Authorization': 'Bearer $accessToken', 'Accept': 'application/json'});
    final http.Response response;
    try {
      response = await http.Response.fromStream(await _client.send(request).timeout(timeout));
    } on SocketException {
      throw const MessagesFailure(MessagesFailureKind.unavailable);
    } on TimeoutException {
      throw const MessagesFailure(MessagesFailureKind.unavailable);
    } on http.ClientException {
      throw const MessagesFailure(MessagesFailureKind.unavailable);
    }
    switch (response.statusCode) {
      case 200:
        try {
          return jsonDecode(response.body);
        } on FormatException {
          throw const MessagesFailure(MessagesFailureKind.unavailable);
        }
      case 401:
        throw const MessagesFailure(MessagesFailureKind.unauthorized);
      case 403:
        throw const MessagesFailure(MessagesFailureKind.forbidden);
      case 404:
        throw const MessagesFailure(MessagesFailureKind.notFound);
      default:
        throw const MessagesFailure(MessagesFailureKind.unavailable);
    }
  }
}

/// What reading the trip's messages came back with.
class MessageLoad {
  const MessageLoad.loaded(List<DispatcherMessage> this.messages)
      : failure = null,
        signInExpired = false;
  const MessageLoad.failed(String this.failure, {this.signInExpired = false}) : messages = null;

  final List<DispatcherMessage>? messages;
  final String? failure;
  final bool signInExpired;
}

/// What acknowledging a message came back with: done, or why not (the message then stays unread).
class MessageAck {
  const MessageAck.done()
      : failure = null,
        signInExpired = false;
  const MessageAck.failed(String this.failure, {this.signInExpired = false});

  final String? failure;
  final bool signInExpired;
  bool get isDone => failure == null;
}

abstract class MessageSource {
  Future<MessageLoad> load(String tripId);
  Future<MessageAck> acknowledge(String tripId, String messageId);
}

/// Reads and acknowledges messages through the Waypoint API with the signed-in driver's token.
class ApiMessageSource implements MessageSource {
  ApiMessageSource({required this.api, required this.auth});

  final MessagesApi api;
  final AuthGateway auth;

  static const _unreachable = 'Could not reach Waypoint. Check your connection and try again.';

  @override
  Future<MessageLoad> load(String tripId) async {
    final token = await _token();
    if (token == null) return const MessageLoad.failed('Your sign-in expired. Sign in again.', signInExpired: true);
    if (token.isEmpty) return const MessageLoad.failed(_unreachable);
    try {
      return MessageLoad.loaded(await api.list(tripId, token));
    } on MessagesFailure catch (failure) {
      return MessageLoad.failed(_message(failure), signInExpired: failure.kind == MessagesFailureKind.unauthorized);
    }
  }

  @override
  Future<MessageAck> acknowledge(String tripId, String messageId) async {
    final token = await _token();
    if (token == null) return const MessageAck.failed('Your sign-in expired. Sign in again.', signInExpired: true);
    if (token.isEmpty) return const MessageAck.failed(_unreachable);
    try {
      await api.acknowledge(tripId, messageId, token);
      return const MessageAck.done();
    } on MessagesFailure catch (failure) {
      return MessageAck.failed(_message(failure), signInExpired: failure.kind == MessagesFailureKind.unauthorized);
    }
  }

  /// The access token, null when the sign-in is over, or empty when a refresh needed a connection.
  Future<String?> _token() async {
    try {
      return await auth.accessToken();
    } on AuthFailure {
      return '';
    }
  }

  static String _message(MessagesFailure failure) {
    switch (failure.kind) {
      case MessagesFailureKind.unauthorized:
        return 'Your sign-in was not accepted. Sign in again.';
      case MessagesFailureKind.forbidden:
        return 'This account cannot read messages for this trip.';
      case MessagesFailureKind.notFound:
        return 'This trip has not been started in Waypoint yet, so it has no messages.';
      case MessagesFailureKind.unavailable:
        return _unreachable;
    }
  }
}
