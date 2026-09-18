package official.kirizu.kirivers.client

/**
 * HTTP adapter used by [Client]. The default is JDK 17 [java.net.http.HttpClient].
 * Callers may inject a fake implementation in tests. Do not add OkHttp or Ktor Client.
 */
fun interface Transport {
    fun execute(request: HttpRequest): HttpResponse
}

data class HttpRequest(
    val method: String,
    val url: String,
    val headers: Map<String, String> = emptyMap(),
    val body: ByteArray? = null,
)

data class HttpResponse(
    val status: Int,
    val headers: Map<String, List<String>> = emptyMap(),
    val body: ByteArray = ByteArray(0),
) {
    fun header(name: String): String? = headerValue(headers, name)

    fun bodyText(): String = body.toString(Charsets.UTF_8)
}
