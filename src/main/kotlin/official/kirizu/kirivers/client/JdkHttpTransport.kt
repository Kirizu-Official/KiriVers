package official.kirizu.kirivers.client

import java.net.URI
import java.net.http.HttpClient
import java.net.http.HttpRequest as JdkRequest
import java.net.http.HttpResponse as JdkResponse
import java.time.Duration

/**
 * Default [Transport]: JDK [HttpClient]. Follows redirects so private package URLs
 * that bounce to object storage still keep the original query (`exp`/`sig` are already
 * on the request URI).
 */
class JdkHttpTransport(
    private val client: HttpClient = HttpClient.newBuilder()
        .followRedirects(HttpClient.Redirect.NORMAL)
        .connectTimeout(Duration.ofSeconds(30))
        .build(),
    private val requestTimeout: Duration = Duration.ofSeconds(60),
) : Transport {
    override fun execute(request: HttpRequest): HttpResponse {
        val builder = JdkRequest.newBuilder(URI.create(request.url))
            .timeout(requestTimeout)
        request.headers.forEach { (name, value) ->
            if (!RESTRICTED.contains(name.lowercase())) {
                builder.header(name, value)
            }
        }
        val body = request.body
        val publisher = if (body == null) {
            JdkRequest.BodyPublishers.noBody()
        } else {
            JdkRequest.BodyPublishers.ofByteArray(body)
        }
        builder.method(request.method, publisher)
        val jdk = client.send(builder.build(), JdkResponse.BodyHandlers.ofByteArray())
        return HttpResponse(
            status = jdk.statusCode(),
            headers = jdk.headers().map(),
            body = jdk.body() ?: ByteArray(0),
        )
    }

    private companion object {
        val RESTRICTED = setOf("connection", "content-length", "expect", "host", "upgrade")
    }
}
