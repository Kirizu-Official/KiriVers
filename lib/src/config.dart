/// Caller-supplied client configuration. The SDK never invents a device id.
class ClientConfig {
  ClientConfig({
    required String baseUrl,
    required this.projectRef,
    this.projectToken,
    this.channelToken,
    this.signingPublicKeyPem,
    this.signingAlgo,
    this.fileRoot,
    this.requestTimeout = const Duration(seconds: 30),
    this.packPollDeadline = const Duration(minutes: 5),
    this.packPollInitialDelay = const Duration(seconds: 1),
    this.packPollMaxDelay = const Duration(seconds: 15),
  }) : baseUrl = _stripSlash(baseUrl);

  /// Client plane origin, no trailing slash (for example `http://127.0.0.1:8080`).
  final String baseUrl;

  /// Project UUID, live slug, or unexpired slug alias.
  final String projectRef;

  /// `Authorization: Bearer` and `X-Project-Token`. Never logged.
  final String? projectToken;

  /// `X-Channel-Token`. Wrong value is not 403 on check. Never logged.
  final String? channelToken;

  /// PKIX PEM used when a response includes `signature`.
  final String? signingPublicKeyPem;

  /// `ed25519` or `rsa-sha256`. Empty tries Ed25519 then RSA.
  final String? signingAlgo;

  /// Root for the default dart:io [IoFileStore] when a writable store is used.
  final String? fileRoot;

  final Duration requestTimeout;
  final Duration packPollDeadline;
  final Duration packPollInitialDelay;
  final Duration packPollMaxDelay;

  static String _stripSlash(String url) {
    var u = url.trim();
    while (u.endsWith('/')) {
      u = u.substring(0, u.length - 1);
    }
    return u;
  }
}
