package official.kirizu.kirivers.client.adapter;

import java.io.IOException;

/**
 * HTTP adapter. Implementations must support GET/HEAD/POST, arbitrary headers
 * (including {@code Range}), and must preserve the request URL query string
 * ({@code exp}/{@code sig} on private downloads).
 */
@FunctionalInterface
public interface Transport {

  HttpResponse execute(HttpRequest request) throws IOException;
}
