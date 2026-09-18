/// HTTP / protocol errors from the client plane.
///
/// The wire envelope is `{ "error": { "code", "message", "details" } }`.
/// Unknown codes are kept as opaque strings.
class ApiException implements Exception {
  ApiException({
    required this.statusCode,
    required this.code,
    required this.message,
    this.details,
    this.retryAfterSeconds,
  });

  final int statusCode;
  final String code;
  final String message;
  final Object? details;

  /// Parsed from `Retry-After` on 429 `RATE_LIMITED`.
  final int? retryAfterSeconds;

  bool get isRateLimited => code == 'RATE_LIMITED' || statusCode == 429;

  @override
  String toString() => 'ApiException($statusCode $code: $message)';
}

class PathException implements Exception {
  PathException(this.message);

  final String message;

  @override
  String toString() => 'PathException($message)';
}

class PatchException implements Exception {
  PatchException(this.message);

  final String message;

  @override
  String toString() => 'PatchException($message)';
}

class SignatureException implements Exception {
  SignatureException(this.message);

  final String message;

  @override
  String toString() => 'SignatureException($message)';
}

class HashMismatchException implements Exception {
  HashMismatchException({required this.expected, required this.actual});

  final String expected;
  final String actual;

  @override
  String toString() =>
      'HashMismatchException(expected: $expected, actual: $actual)';
}

class TransportException implements Exception {
  TransportException(this.message);

  final String message;

  @override
  String toString() => 'TransportException($message)';
}
