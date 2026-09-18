import 'package:http/http.dart' as http;

import 'adapters.dart';

/// Default [Transport] using `package:http` (D18). Injectable for tests.
class HttpTransport implements Transport {
  HttpTransport({
    http.Client? client,
    this.timeout = const Duration(seconds: 30),
  })  : _client = client ?? http.Client(),
        _ownsClient = client == null;

  final http.Client _client;
  final Duration timeout;
  final bool _ownsClient;

  @override
  Future<TransportResponse> send(TransportRequest request) async {
    final req = http.Request(request.method, request.url);
    req.followRedirects = true;
    req.persistentConnection = true;
    request.headers.forEach((key, value) {
      req.headers[key] = value;
    });
    if (request.body != null) {
      req.bodyBytes = request.body!;
    }
    final streamed = await _client.send(req).timeout(timeout);
    final bytes = await streamed.stream.toBytes().timeout(timeout);
    return TransportResponse(
      statusCode: streamed.statusCode,
      headers: Map<String, String>.from(streamed.headers),
      body: bytes,
    );
  }

  void close() {
    if (_ownsClient) {
      _client.close();
    }
  }
}
