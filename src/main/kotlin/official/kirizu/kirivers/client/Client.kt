package official.kirizu.kirivers.client

import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.JsonElement
import java.net.URI

data class ClientConfig(
    val baseUrl: String,
    val projectRef: String,
    val projectToken: String? = null,
    val channelToken: String? = null,
    val transport: Transport? = JdkHttpTransport(),
    val userAgent: String = "kirivers-client-kotlin/0.1.0",
)

/**
 * Handwritten native JSON client for the KiriVers client plane.
 * Covers every in-SDK operation in `client-plane-surface.md`. Does not wrap leftover
 * paths (`GET /update/check`, `POST /clients/login`, store feeds, pack/status).
 */
class Client(val config: ClientConfig) {
    private val base: String = config.baseUrl.trimEnd('/')
    private val project: String = urlEncode(config.projectRef)

    fun health(): HealthStatus = decode(send("GET", "$base/api/v1/health"))

    fun project(): ProjectPublic = decode(send("GET", projectUrl("")))

    fun reportDevice(request: DeviceReportRequest): DeviceGeo =
        decode(sendJson("POST", projectUrl("/clients/report"), request))

    /**
     * POST `/update/check`. HTTP 204 is [CheckResult.UpToDate], not an error.
     * 304 is [CheckResult.NotModified]. Default capabilities are `["full_package"]`.
     */
    fun check(request: CheckRequest, ifNoneMatch: String? = null): CheckResult {
        val outgoing = request.copy(
            capabilities = request.capabilities?.takeIf { it.isNotEmpty() } ?: listOf(Capability.FULL_PACKAGE),
            acceptedDeltaAlgos = request.acceptedDeltaAlgos?.takeIf { it.isNotEmpty() },
        )
        val headers = LinkedHashMap<String, String>()
        if (!ifNoneMatch.isNullOrEmpty()) headers["If-None-Match"] = ifNoneMatch
        val resp = sendJson("POST", projectUrl("/update/check"), outgoing, headers, allow = setOf(200, 204, 304))
        val etag = resp.header("ETag")
        return when (resp.status) {
            204 -> CheckResult.UpToDate(etag)
            304 -> CheckResult.NotModified(etag)
            else -> CheckResult.Update(decode(resp), etag)
        }
    }

    fun changelog(query: ChangelogQuery): Cached<Changelog> {
        val q = queryString(
            listOf(
                "from_version" to query.fromVersion,
                "to_version" to query.toVersion,
                "changelog_scope" to query.scope,
                "changelog_layout" to query.layout,
                "changelog_include_revoked" to query.includeRevoked?.toString(),
                "changelog_include_platform_notes" to query.includePlatformNotes?.toString(),
                "changelog_locale" to query.changelogLocale,
                "locale" to query.locale,
            ),
        )
        val path = projectUrl(
            "/changelog/${urlEncode(query.channel)}/${urlEncode(query.os)}/${urlEncode(query.arch)}$q",
        )
        return cachedGet(path, query.ifNoneMatch)
    }

    fun integrity(query: IntegrityQuery): Cached<IntegrityManifest> {
        val q = queryString(
            listOf(
                "os" to query.os,
                "arch" to query.arch,
                "channel" to query.channel,
                "hw_rev" to query.hwRev,
                "hash_algo" to query.hashAlgo,
                "compact" to query.compact?.toString(),
                "include_file_urls" to query.includeFileUrls?.toString(),
            ),
        )
        return cachedGet(projectUrl("/versions/${urlEncode(query.version)}/integrity$q"), query.ifNoneMatch)
    }

    fun diff(request: DiffRequest): DiffResponse =
        decode(sendJson("POST", projectUrl("/update/diff"), request))

    fun pack(request: PackRequest): PackResponse {
        val resp = sendJson("POST", projectUrl("/update/pack"), request, allow = setOf(200, 202))
        return decode(resp)
    }

    /**
     * POST the **identical** JSON until `ready` / `full_package` / error.
     * Backoff starts at 1s and caps at 15s. Never calls admin jobs.
     */
    fun packUntilReady(request: PackRequest, options: PackPollOptions = PackPollOptions()): PackResponse {
        var delay = options.initialDelayMs
        val start = System.currentTimeMillis()
        while (true) {
            val result = pack(request)
            if (result.status != "pending") return result
            val elapsed = System.currentTimeMillis() - start
            if (elapsed + delay >= options.deadlineMs) throw PackTimeoutException()
            try {
                Thread.sleep(delay)
            } catch (ex: InterruptedException) {
                Thread.currentThread().interrupt()
                throw PackTimeoutException()
            }
            delay = minOf(delay * 2, options.maxDelayMs)
        }
    }

    fun downloadPackage(
        ref: String,
        exp: String? = null,
        sig: String? = null,
        range: String? = null,
        hwRev: String? = null,
    ): BinaryBody {
        val q = queryString(listOf("exp" to exp, "sig" to sig, "hw_rev" to hwRev))
        return downloadRaw("GET", projectUrl("/packages/${urlEncode(ref)}$q"), range)
    }

    fun headPackage(
        ref: String,
        exp: String? = null,
        sig: String? = null,
        hwRev: String? = null,
    ): BinaryBody {
        val q = queryString(listOf("exp" to exp, "sig" to sig, "hw_rev" to hwRev))
        return downloadRaw("HEAD", projectUrl("/packages/${urlEncode(ref)}$q"), range = null)
    }

    /**
     * Download from a check/diff/pack `package_url`. Relative URLs resolve against [ClientConfig.baseUrl].
     * Query string (`exp`/`sig`) is preserved. Optional HTTP `Range`.
     */
    fun download(url: String, range: String? = null): BinaryBody =
        downloadRaw("GET", resolveUrl(url), range)

    fun head(url: String): BinaryBody = downloadRaw("HEAD", resolveUrl(url), range = null)

    fun channels(): ChannelList = decode(send("GET", projectUrl("/channels")))

    fun matrix(): MatrixList = decode(send("GET", projectUrl("/matrix")))

    fun languages(): LanguageList = decode(send("GET", projectUrl("/languages")))

    fun announcements(query: AnnouncementQuery = AnnouncementQuery()): Cached<AnnouncementList> {
        val q = queryString(
            listOf(
                "version" to query.version,
                "os" to query.os,
                "arch" to query.arch,
                "locale" to query.locale,
            ),
        )
        val headers = LinkedHashMap<String, String>()
        if (!query.ifNoneMatch.isNullOrEmpty()) headers["If-None-Match"] = query.ifNoneMatch
        if (!query.acceptLanguage.isNullOrEmpty()) headers["Accept-Language"] = query.acceptLanguage
        val resp = send("GET", projectUrl("/announcements$q"), headers, allow = setOf(200, 304))
        val etag = resp.header("ETag")
        return if (resp.status == 304) Cached.NotModified(etag) else Cached.Fresh(decode(resp), etag)
    }

    fun reportTelemetry(request: TelemetryRequest): TelemetryAccepted {
        val resp = sendJson("POST", projectUrl("/telemetry/report"), request, allow = setOf(202))
        return if (resp.body.isEmpty()) TelemetryAccepted("accepted") else decode(resp)
    }

    fun downloadMedia(id: String, range: String? = null): BinaryBody =
        downloadRaw("GET", projectUrl("/media/${urlEncode(id)}"), range)

    fun headMedia(id: String): BinaryBody =
        downloadRaw("HEAD", projectUrl("/media/${urlEncode(id)}"), range = null)

    internal fun resolveUrl(pathOrUrl: String): String {
        if (pathOrUrl.startsWith("http://", ignoreCase = true) ||
            pathOrUrl.startsWith("https://", ignoreCase = true)
        ) {
            return pathOrUrl
        }
        return URI("$base/").resolve(pathOrUrl).toString()
    }

    private inline fun <reified T> cachedGet(url: String, ifNoneMatch: String?): Cached<T> {
        val headers = LinkedHashMap<String, String>()
        if (!ifNoneMatch.isNullOrEmpty()) headers["If-None-Match"] = ifNoneMatch
        val resp = send("GET", url, headers, allow = setOf(200, 304))
        val etag = resp.header("ETag")
        return if (resp.status == 304) Cached.NotModified(etag) else Cached.Fresh(decode(resp), etag)
    }

    private fun downloadRaw(method: String, url: String, range: String?): BinaryBody {
        val headers = LinkedHashMap<String, String>()
        if (!range.isNullOrEmpty()) headers["Range"] = range
        val resp = send(method, url, headers, allow = setOf(200, 206))
        return BinaryBody(resp.status, resp.headers, resp.body)
    }

    private inline fun <reified T> sendJson(
        method: String,
        url: String,
        body: T,
        extra: Map<String, String> = emptyMap(),
        allow: Set<Int> = setOf(200),
    ): HttpResponse {
        val json = KiriversJson.encodeToString(body)
        return send(method, url, extra, json.toByteArray(Charsets.UTF_8), allow)
    }

    private fun send(
        method: String,
        url: String,
        extra: Map<String, String> = emptyMap(),
        body: ByteArray? = null,
        allow: Set<Int> = setOf(200),
    ): HttpResponse {
        val transport = config.transport ?: throw MissingTransportException()
        val headers = LinkedHashMap<String, String>()
        headers["User-Agent"] = config.userAgent
        val token = config.projectToken
        if (!token.isNullOrEmpty()) {
            headers["Authorization"] = "Bearer $token"
            headers["X-Project-Token"] = token
        }
        val channelToken = config.channelToken
        if (!channelToken.isNullOrEmpty()) {
            headers["X-Channel-Token"] = channelToken
        }
        if (body != null) headers["Content-Type"] = "application/json"
        headers.putAll(extra)
        val resp = transport.execute(HttpRequest(method, url, headers, body))
        if (resp.status !in allow) throw apiError(resp)
        return resp
    }

    private inline fun <reified T> decode(resp: HttpResponse): T =
        KiriversJson.decodeFromString(resp.bodyText().ifEmpty { "{}" })

    private fun apiError(resp: HttpResponse): ApiException {
        val retry = resp.header("Retry-After")?.toIntOrNull()
        val text = resp.bodyText()
        val parsed = runCatching { KiriversJson.decodeFromString<ErrorEnvelope>(text) }.getOrNull()
        val body = parsed?.error
        val details = body?.details?.let { stringifyDetails(it) }
        return ApiException(
            status = resp.status,
            code = body?.code ?: "UNKNOWN",
            message = body?.message ?: text.take(500).ifBlank { "HTTP ${resp.status}" },
            details = details,
            retryAfter = retry,
        )
    }

    private fun stringifyDetails(details: JsonElement): String = details.toString()

    private fun projectUrl(suffix: String): String = "$base/api/v1/projects/$project$suffix"
}
