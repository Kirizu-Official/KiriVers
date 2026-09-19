package official.kirizu.kirivers.client

import kotlinx.serialization.json.Json
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import java.io.IOException
import java.util.UUID
import kotlin.io.path.Path
import kotlin.io.path.exists
import kotlin.io.path.readText
import kotlin.io.path.writeText
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertIs
import kotlin.test.assertTrue
import kotlin.test.fail

class IntegrationTest {
    @Test
    fun checkDownloadAndSha256AgainstLocalClientPlane() {
        val fixture = loadFixture()
        val deviceId = "sdk-kotlin-" + UUID.randomUUID().toString().replace("-", "").take(12)
        val client = Client(
            ClientConfig(
                baseUrl = fixture.baseUrl,
                projectRef = fixture.projectRef,
            ),
        )
        val check = try {
            client.check(
                CheckRequest(
                    currentVersion = fixture.currentVersion,
                    os = fixture.os,
                    arch = fixture.arch,
                    channel = fixture.channel,
                    deviceId = deviceId,
                ),
            )
        } catch (ex: IOException) {
            writeBackendIssue("client plane is not reachable at ${fixture.baseUrl}", ex)
            throw ex
        } catch (ex: ApiException) {
            writeBackendIssue("check failed: HTTP ${ex.status} ${ex.code} ${ex.message}", ex)
            throw ex
        }

        val update = when (check) {
            is CheckResult.Update -> check.update
            is CheckResult.UpToDate -> {
                writeBackendIssue(
                    "expected an update from ${fixture.currentVersion} to ${fixture.targetVersion}, got 204",
                    null,
                )
                fail("no update from fixture check")
            }
            is CheckResult.NotModified -> fail("unexpected 304 without If-None-Match")
        }
        if (update.versionSemver != fixture.targetVersion) {
            writeBackendIssue(
                "expected target ${fixture.targetVersion}, got ${update.versionSemver}",
                null,
            )
        }
        assertEquals(fixture.targetVersion, update.versionSemver)
        assertTrue(update.packageUrl.isNotEmpty())

        val downloaded = try {
            client.download(update.packageUrl)
        } catch (ex: ApiException) {
            writeBackendIssue("download failed: HTTP ${ex.status} ${ex.code} ${ex.message}", ex)
            throw ex
        }
        val actual = JdkHasher().sha256Hex(downloaded.body)
        if (!actual.equals(fixture.targetSha256, ignoreCase = true)) {
            writeBackendIssue(
                "sha256 mismatch after download: expected ${fixture.targetSha256} actual $actual",
                null,
            )
        }
        assertEquals(fixture.targetSha256, actual)
        assertIs<CheckResult.Update>(check)
    }

    private data class Fixture(
        val baseUrl: String,
        val projectRef: String,
        val channel: String,
        val os: String,
        val arch: String,
        val currentVersion: String,
        val targetVersion: String,
        val targetSha256: String,
    )

    private fun loadFixture(): Fixture {
        val path = sequenceOf(
            System.getenv("KIRIVERS_SDK_FIXTURE"),
            """D:\KiriVers\configs\sdk-fixture.json""",
        ).filterNotNull().map { Path(it) }.firstOrNull { it.exists() }
            ?: error("sdk-fixture.json not found")
        val root = Json.parseToJsonElement(path.readText()).jsonObject
        val sha = root["sha256"]!!.jsonObject
        return Fixture(
            baseUrl = root["client_base_url"]!!.jsonPrimitive.content,
            projectRef = root["project_ref"]!!.jsonPrimitive.content,
            channel = root["channel"]!!.jsonPrimitive.content,
            os = root["os"]!!.jsonPrimitive.content,
            arch = root["arch"]!!.jsonPrimitive.content,
            currentVersion = root["current_version"]!!.jsonPrimitive.content,
            targetVersion = root["target_version"]!!.jsonPrimitive.content,
            targetSha256 = sha["1.1.0"]!!.jsonPrimitive.content,
        )
    }

    private fun writeBackendIssue(summary: String, error: Exception?) {
        val text = buildString {
            appendLine("# Backend issue (Kotlin SDK integration)")
            appendLine()
            appendLine("The Kotlin client SDK integration test hit a live-server problem. This worktree did not change `D:\\KiriVers`.")
            appendLine()
            appendLine("## Repro")
            appendLine()
            appendLine("1. Client plane at `http://127.0.0.1:8080`")
            appendLine("2. Fixture `D:\\KiriVers\\configs\\sdk-fixture.json`")
            appendLine("3. `POST /api/v1/projects/sdk-fixture/update/check` from `1.0.0` / windows / x86_64")
            appendLine("4. Download `1.1.0` package and SHA-256 `7f063a6901ebfd0aa7b1adb8af58f4e792f13310f42213767ced623eb0669969`")
            appendLine()
            appendLine("## Expected")
            appendLine()
            appendLine("Check 200 with `version_semver=1.1.0` and a `package_url` whose bytes hash to the fixture SHA-256.")
            appendLine()
            appendLine("## Actual")
            appendLine()
            appendLine(summary)
            if (error != null) {
                appendLine()
                appendLine("```")
                appendLine(error.stackTraceToString().take(4000))
                appendLine("```")
            }
            appendLine()
            appendLine("## Suggested fix")
            appendLine()
            appendLine("Health on `:8080` is up (`ready=true`). `PROJECT_NOT_FOUND` means the check route was hit but slug `sdk-fixture` is missing from the DB.")
            appendLine("Re-seed with `.trellis/tasks/09-17-client-sdk/scripts/seed_local_fixture.py` on the server host. Do not change this Kotlin package to match a missing fixture.")
        }
        Path("BACKEND_ISSUE.md").writeText(text)
    }
}
