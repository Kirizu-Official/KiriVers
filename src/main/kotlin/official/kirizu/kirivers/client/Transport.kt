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
) {
    override fun equals(other: Any?): Boolean {
        if (this === other) return true
        if (other !is HttpRequest) return false
        return method == other.method &&
            url == other.url &&
            headers == other.headers &&
            body.contentEquals(other.body)
    }

    override fun hashCode(): Int {
        var result = method.hashCode()
        result = 31 * result + url.hashCode()
        result = 31 * result + headers.hashCode()
        result = 31 * result + (body?.contentHashCode() ?: 0)
        return result
    }
}

data class HttpResponse(
    val status: Int,
    val headers: Map<String, List<String>> = emptyMap(),
    val body: ByteArray = ByteArray(0),
) {
    fun header(name: String): String? = headerValue(headers, name)

    fun bodyText(): String = body.toString(Charsets.UTF_8)

    override fun equals(other: Any?): Boolean {
        if (this === other) return true
        if (other !is HttpResponse) return false
        return status == other.status && headers == other.headers && body.contentEquals(other.body)
    }

    override fun hashCode(): Int {
        var result = status
        result = 31 * result + headers.hashCode()
        result = 31 * result + body.contentHashCode()
        return result
    }
}
