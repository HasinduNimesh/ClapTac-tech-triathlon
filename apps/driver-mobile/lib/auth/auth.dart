class AuthSession {
  const AuthSession({required this.subject, required this.accessToken});

  final String subject;
  final String accessToken;
}

abstract class AuthRepository {
  Future<AuthSession?> current();
}

class PlaceholderAuthRepository implements AuthRepository {
  @override
  Future<AuthSession?> current() async => null;
}
