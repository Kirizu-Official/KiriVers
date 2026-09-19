package official.kirizu.kirivers.client

import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import java.security.MessageDigest
import kotlin.io.path.Path
import kotlin.io.path.exists
import kotlin.io.path.readBytes
import kotlin.io.path.readText
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertFalse
import kotlin.test.assertIs
import kotlin.test.assertNotNull
import kotlin.test.assertTrue
import kotlin.test.fail

class ContractTest {
    private val spec = loadOpenApi()

    @Test
    fun openapiRevisionMatchesSnapshot() {
        val file = listOf(Path("openapi.client.json"), Path("../openapi.client.json")).first { it.exists() }
        val digest = MessageDigest.getInstance("SHA-256").digest(file.readBytes())
        val short = digest.take(4).joinToString("") { "%02X".format(it) }
        val revision = Path("OPENAPI_REVISION").readText().trim()
        val version = spec["info"]!!.jsonObject["version"]!!.jsonPrimitive.content
        assertEquals("$version $short", revision)
    }

    @Test
    fun handwrittenClientCoversNativeJsonSurface() {
        val transport = RecordingTransport { req -> canned(req) }
        val client = Client(ClientConfig("http://127.0.0.1:8080", "sdk-fixture", transport = transport))

        client.health()
        client.project()
        client.reportDevice(DeviceReportRequest(deviceId = "dev-1", os = "windows", arch = "x86_64"))
        client.check(CheckRequest(currentVersion = "1.0.0", os = "windows", arch = "x86_64"))
        client.changelog(ChangelogQuery("stable", "windows", "x86_64"))
        client.integrity(IntegrityQuery(version = "1.1.0", os = "windows", arch = "x86_64"))
        client.diff(
            DiffRequest(
                sourceVersion = "1.0.0",
                targetVersion = "1.1.0",
                os = "windows",
                arch = "x86_64",
                localSha256 = "abc",
            ),
        )
        client.pack(
            PackRequest(
                sourceVersion = "1.0.0",
                targetVersion = "1.1.0",
                os = "windows",
                arch = "x86_64",
                neededPaths = listOf("bin/app"),
            ),
        )
        client.downloadPackage("7f063a6901ebfd0aa7b1adb8af58f4e792f13310f42213767ced623eb0669969")
        client.headPackage("7f063a6901ebfd0aa7b1adb8af58f4e792f13310f42213767ced623eb0669969")
        client.reportTelemetry(
            TelemetryRequest(
                os = "windows",
                arch = "x86_64",
                channel = "stable",
                fromVersion = "1.0.0",
                toVersion = "1.1.0",
                status = TelemetryStatus.INSTALLED,
            ),
        )
        client.channels()
        client.matrix()
        client.languages()
        client.announcements()
        client.downloadMedia("11111111-1111-1111-1111-111111111111")
        client.headMedia("11111111-1111-1111-1111-111111111111")

        val called = transport.methods().map { (method, path) -> method to openApiTemplate(path) }.toSet()
        assertEquals(nativeOps(spec), called, "handwritten client drifted from openapi.client.json")

        val integrity = transport.requests.first { RecordingTransport.pathOf(it.url).contains("/integrity") }
        val integrityQuery = integrity.url.substringAfter('?', missingDelimiterValue = "")
        assertTrue(integrityQuery.contains("os="), integrity.url)
        assertTrue(integrityQuery.contains("arch="), integrity.url)
        assertTrue(spec["paths"]!!.jsonObject.keys.any { it.contains("/store/") })
    }

    @Test
    fun checkSendsRequiredFieldsAndDefaultFullPackageOnly() {
        val transport = RecordingTransport { jsonOk(CHECK_200) }
        val client = Client(ClientConfig("http://example.test", "demo", transport = transport))
        client.check(CheckRequest(currentVersion = "1.0.0", os = "windows", arch = "x86_64"))
        val body = KiriversJson.parseToJsonElement(transport.last().body!!.toString(Charsets.UTF_8)).jsonObject
        assertEquals("1.0.0", body["current_version"]!!.jsonPrimitive.content)
        assertEquals("windows", body["os"]!!.jsonPrimitive.content)
        assertEquals("x86_64", body["arch"]!!.jsonPrimitive.content)
        val caps = body["capabilities"]!!.jsonArray.map { it.jsonPrimitive.content }
        assertEquals(listOf(Capability.FULL_PACKAGE), caps)
        assertFalse(body.containsKey("local_sha256"))
        assertFalse(body.containsKey("dirty_paths"))
        assertFalse(body.containsKey("accepted_delta_algos"))
        for (name in requiredBodyFields("/api/v1/projects/{project_ref}/update/check", "post")) {
            assertTrue(body.containsKey(name), "check missing $name")
        }
    }

    @Test
    fun check204IsNotAnErrorAnd304IsEtagHit() {
        val transport = RecordingTransport { req ->
            when {
                req.headers["If-None-Match"] == "\"abc\"" ->
                    jsonOk("", 304, mapOf("ETag" to listOf("\"abc\"")))
                else -> jsonOk("", 204, mapOf("ETag" to listOf("\"abc\"")))
            }
        }
        val client = Client(ClientConfig("http://example.test", "demo", transport = transport))
        assertIs<CheckResult.UpToDate>(client.check(CheckRequest("1.0.0", "linux", "arm64")))
        val nm = client.check(CheckRequest("1.0.0", "linux", "arm64"), ifNoneMatch = "\"abc\"")
        assertIs<CheckResult.NotModified>(nm)
    }

    @Test
    fun errorEnvelopeSurfacesStableCode() {
        val transport = RecordingTransport { errorJson(404, "PROJECT_NOT_FOUND", "missing") }
        val client = Client(ClientConfig("http://example.test", "demo", transport = transport))
        val ex = assertFailsWith<ApiException> { client.project() }
        assertEquals(404, ex.status)
        assertEquals("PROJECT_NOT_FOUND", ex.code)
        assertEquals("missing", ex.message)
        assertNotNull(ex.details)
    }

    @Test
    fun requiredBodiesMatchOpenApiNames() {
        val transport = RecordingTransport { canned(it) }
        val client = Client(ClientConfig("http://example.test", "p", transport = transport))
        client.reportDevice(DeviceReportRequest("device-a"))
        client.diff(DiffRequest("1.0.0", "1.1.0", "windows", "x86_64"))
        client.pack(PackRequest("1.0.0", "1.1.0", "windows", "x86_64"))
        client.reportTelemetry(
            TelemetryRequest("windows", "x86_64", "stable", "1.0.0", "1.1.0", "downloading"),
        )
        val byPath = transport.requests.associateBy { RecordingTransport.pathOf(it.url) }
        val report = KiriversJson.parseToJsonElement(byPath["/api/v1/projects/p/clients/report"]!!.body!!.decodeToString()).jsonObject
        for (name in requiredBodyFields("/api/v1/projects/{project_ref}/clients/report", "post")) {
            assertTrue(report.containsKey(name), "report missing $name")
        }
        val diff = KiriversJson.parseToJsonElement(byPath["/api/v1/projects/p/update/diff"]!!.body!!.decodeToString()).jsonObject
        for (name in requiredBodyFields("/api/v1/projects/{project_ref}/update/diff", "post")) {
            assertTrue(diff.containsKey(name), "diff missing $name")
        }
        val pack = KiriversJson.parseToJsonElement(byPath["/api/v1/projects/p/update/pack"]!!.body!!.decodeToString()).jsonObject
        for (name in requiredBodyFields("/api/v1/projects/{project_ref}/update/pack", "post")) {
            assertTrue(pack.containsKey(name), "pack missing $name")
        }
        val tel = KiriversJson.parseToJsonElement(byPath["/api/v1/projects/p/telemetry/report"]!!.body!!.decodeToString()).jsonObject
        for (name in requiredBodyFields("/api/v1/projects/{project_ref}/telemetry/report", "post")) {
            assertTrue(tel.containsKey(name), "telemetry missing $name")
        }
    }

    @Test
    fun leftoverPathsAreNeverCalled() {
        val transport = RecordingTransport { canned(it) }
        val client = Client(ClientConfig("http://example.test", "p", transport = transport))
        client.check(CheckRequest("1.0.0", "windows", "x86_64"))
        client.pack(PackRequest("1.0.0", "1.1.0", "windows", "x86_64"))
        val paths = transport.requests.map { RecordingTransport.pathOf(it.url) }
        for (req in transport.requests) {
            val path = RecordingTransport.pathOf(req.url)
            if (path.endsWith("/update/check")) {
                assertEquals("POST", req.method)
            }
        }
        assertFalse(paths.any { it.contains("/clients/login") })
        assertFalse(paths.any { it.contains("/pack/status") })
        assertFalse(paths.any { it.contains("/store/") })
        assertFalse(paths.any { it.contains("/manifest") })
        assertFalse(paths.any { it.contains("/artifacts/") })
        assertFalse(paths.any { it.endsWith("/ready") || it.contains("/api/v1/ready") })
        assertFalse(transport.requests.any { it.method == "GET" && RecordingTransport.pathOf(it.url).endsWith("/update/check") })
        val checkOps = spec["paths"]!!.jsonObject["/api/v1/projects/{project_ref}/update/check"]!!.jsonObject
        assertFalse(checkOps.containsKey("get"))
    }

    @Test
    fun downloadKeepsQueryAndRange() {
        val transport = RecordingTransport { HttpResponse(200, body = byteArrayOf(1, 2, 3)) }
        val client = Client(ClientConfig("http://example.test", "p", transport = transport))
        client.download("/api/v1/projects/p/packages/abcd?exp=9&sig=zz", range = "bytes=0-1")
        val req = transport.last()
        assertEquals("GET", req.method)
        assertTrue(req.url.contains("exp=9"))
        assertTrue(req.url.contains("sig=zz"))
        assertEquals("bytes=0-1", req.headers["Range"])
    }

    @Test
    fun missingTransportFails() {
        val client = Client(ClientConfig("http://example.test", "p", transport = null))
        assertFailsWith<MissingTransportException> { client.health() }
    }

    private fun openApiTemplate(path: String): String {
        var p = path.replace("/projects/sdk-fixture", "/projects/{project_ref}")
        p = p.replace("/projects/p/", "/projects/{project_ref}/")
        p = p.replace("/projects/p", "/projects/{project_ref}")
        p = p.replace("/changelog/stable/windows/x86_64", "/changelog/{channel}/{os}/{arch}")
        p = p.replace("/versions/1.1.0/integrity", "/versions/{version}/integrity")
        p = Regex("/packages/[^/]+").replace(p, "/packages/{ref}")
        p = Regex("/media/[^/]+").replace(p, "/media/{id}")
        return p
    }

    private fun nativeOps(spec: JsonObject): Set<Pair<String, String>> {
        val skipKeys = setOf("parameters", "summary", "description", "servers")
        val out = linkedSetOf<Pair<String, String>>()
        for ((path, item) in spec["paths"]!!.jsonObject) {
            if (path.contains("/store/") || path.endsWith("/openapi.json")) continue
            for (method in item.jsonObject.keys) {
                if (method.startsWith("x-") || method in skipKeys || method.equals("options", ignoreCase = true)) {
                    continue
                }
                val upper = method.uppercase()
                if (upper in setOf("GET", "POST", "HEAD")) {
                    out += upper to path
                }
            }
        }
        return out
    }

    private fun requiredBodyFields(template: String, method: String): Set<String> {
        val op = spec["paths"]!!.jsonObject[template]?.jsonObject?.get(method)?.jsonObject
            ?: fail("OpenAPI missing $method $template")
        val schema = op["requestBody"]
            ?.jsonObject?.get("content")
            ?.jsonObject?.get("application/json")
            ?.jsonObject?.get("schema")
            ?.jsonObject
            ?: return emptySet()
        val resolved = schema["\$ref"]?.jsonPrimitive?.content?.substringAfterLast('/')?.let { name ->
            spec["components"]!!.jsonObject["schemas"]!!.jsonObject[name]!!.jsonObject
        } ?: schema
        return resolved["required"]?.jsonArray?.map { it.jsonPrimitive.content }?.toSet() ?: emptySet()
    }

    private fun canned(req: HttpRequest): HttpResponse {
        val path = RecordingTransport.pathOf(req.url)
        return when {
            path.endsWith("/health") -> jsonOk("""{"status":"ok","ready":true}""")
            path.endsWith("/clients/report") ->
                jsonOk("""{"ip":"127.0.0.1","country_code":"","region_code":"","geo_i18n":{}}""")
            path.endsWith("/update/check") -> jsonOk(CHECK_200)
            path.contains("/changelog/") -> jsonOk("""{"changelog":"# hi","changelog_versions":[]}""")
            path.contains("/integrity") -> jsonOk(
                """
                {
                  "version_integer": null,
                  "version_semver": "1.1.0",
                  "channel": "stable",
                  "package_type": "single_file",
                  "root_hash": "",
                  "full_package_url": "/pkg",
                  "file_name": "a.bin",
                  "size": 1,
                  "sha256": "aa",
                  "files": [{"path":"a.bin","size":1,"install_policy":"OVERWRITE","integrity_check":true,"sha256":"aa"}]
                }
                """.trimIndent(),
            )
            path.endsWith("/update/diff") -> jsonOk(
                """
                {
                  "diff_mode": "full_package",
                  "root_hash": "",
                  "version_integer": null,
                  "version_semver": "1.1.0",
                  "channel": "stable",
                  "compare_engine": "semver"
                }
                """.trimIndent(),
            )
            path.endsWith("/update/pack") -> jsonOk("""{"status":"ready","package_url":"/pkg","sha256":"aa"}""")
            path.endsWith("/telemetry/report") -> jsonOk("""{"status":"accepted"}""", 202)
            path.endsWith("/channels") -> jsonOk("""{"channels":[]}""")
            path.endsWith("/matrix") -> jsonOk("""{"matrix":[]}""")
            path.endsWith("/languages") -> jsonOk("""{"languages":[]}""")
            path.endsWith("/announcements") -> jsonOk("""{"announcements":[]}""")
            req.method == "HEAD" -> HttpResponse(200)
            path.contains("/packages/") || path.contains("/media/") -> HttpResponse(200, body = byteArrayOf(0))
            path.matches(Regex("/api/v1/projects/[^/]+$")) ->
                jsonOk("""{"uuid":"u","slug":"p","compare_engine":"semver"}""")
            else -> errorJson(404, "NOT_FOUND", path)
        }
    }

    private fun loadOpenApi(): kotlinx.serialization.json.JsonObject {
        val file = listOf(Path("openapi.client.json"), Path("../openapi.client.json")).first { it.exists() }
        return Json.parseToJsonElement(file.readText()).jsonObject
    }
}
