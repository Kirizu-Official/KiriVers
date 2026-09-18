package official.kirizu.kirivers.client

import java.util.concurrent.CopyOnWriteArrayList

class RecordingTransport(
    private val handler: (HttpRequest) -> HttpResponse,
) : Transport {
    val requests: MutableList<HttpRequest> = CopyOnWriteArrayList()

    override fun execute(request: HttpRequest): HttpResponse {
        requests += request
        return handler(request)
    }

    fun last(): HttpRequest = requests.last()

    fun methods(): List<Pair<String, String>> = requests.map { it.method to pathOf(it.url) }

    companion object {
        fun pathOf(url: String): String {
            val noQuery = url.substringBefore('?')
            val idx = noQuery.indexOf("/api/")
            return if (idx >= 0) noQuery.substring(idx) else noQuery
        }
    }
}

fun jsonOk(body: String, status: Int = 200, headers: Map<String, List<String>> = emptyMap()): HttpResponse =
    HttpResponse(status, headers, body.toByteArray(Charsets.UTF_8))

fun errorJson(status: Int, code: String, message: String): HttpResponse =
    jsonOk("""{"error":{"code":"$code","message":"$message","details":{"hint":"x"}}}""", status)

val CHECK_200 = """
{
  "has_update": true,
  "is_mandatory": false,
  "is_downgrade": false,
  "reason": "normal",
  "compare_engine": "semver",
  "version_integer": null,
  "version_semver": "1.1.0",
  "target_channel": "stable",
  "target_hw_rev": null,
  "package_type": "single_file",
  "root_hash": "",
  "package_url": "/api/v1/projects/demo/packages/abc",
  "file_name": "app.bin",
  "size": 4,
  "sha256": "aaaa",
  "delta_available": false
}
""".trimIndent()
