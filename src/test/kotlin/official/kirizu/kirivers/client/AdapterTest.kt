package official.kirizu.kirivers.client

import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.jsonObject
import java.nio.file.Files
import java.text.Normalizer
import java.util.zip.ZipEntry
import java.util.zip.ZipOutputStream
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertFalse
import kotlin.test.assertIs
import kotlin.test.assertTrue

class AdapterTest {
    @Test
    fun nfcAndSlashNormalizationMatchesServerRules() {
        val nfd = Normalizer.normalize("é", Normalizer.Form.NFD)
        val nfc = Normalizer.normalize("é", Normalizer.Form.NFC)
        assertEquals(InstallPaths.normalize("dir\\café"), InstallPaths.normalize("dir/café"))
        assertEquals(InstallPaths.normalize("a/${nfd}/b"), InstallPaths.normalize("a/${nfc}/b"))
        assertFailsWith<InvalidPathException> { InstallPaths.normalize("../x") }
        assertFailsWith<InvalidPathException> { InstallPaths.normalize("/abs") }
        assertFailsWith<InvalidPathException> { InstallPaths.normalize("C:windows") }
    }

    @Test
    fun updaterCapabilitiesFollowAdapters() {
        val transport = RecordingTransport { jsonOk(CHECK_200) }
        val client = Client(ClientConfig("http://example.test", "p", transport = transport))
        val none = Updater(client, fileStore = null, archiveUnpacker = null, patcher = null)
        assertEquals(listOf(Capability.FULL_PACKAGE) to null, none.capabilities())

        val withZip = Updater(client, fileStore = null, archiveUnpacker = JdkZipUnpacker(), patcher = null)
        assertEquals(
            listOf(Capability.FULL_PACKAGE, Capability.PATCH_PACKAGE) to null,
            withZip.capabilities(),
        )

        val tmp = Files.createTempDirectory("kv-fs")
        val store = JdkFileStore(tmp)
        val withFiles = Updater(client, fileStore = store, archiveUnpacker = null, patcher = null)
        assertTrue(withFiles.capabilities().first.contains(Capability.FILE_LIST))

        val patcher = object : Patcher {
            override fun supportedAlgos() = listOf(DeltaAlgo.BSDIFF)
            override fun apply(oldBytes: ByteArray, delta: ByteArray, algo: String) = oldBytes
        }
        val withPatch = Updater(client, fileStore = null, archiveUnpacker = null, patcher = patcher)
        val (caps, algos) = withPatch.capabilities()
        assertTrue(caps.contains(Capability.BINARY_DELTA))
        assertEquals(listOf(DeltaAlgo.BSDIFF), algos)

        client.check(
            CheckRequest(
                currentVersion = "1.0.0",
                os = "windows",
                arch = "x86_64",
                capabilities = caps,
                acceptedDeltaAlgos = algos,
            ),
        )
        val body = KiriversJson.parseToJsonElement(transport.last().body!!.decodeToString()).jsonObject
        assertTrue(body["capabilities"].toString().contains("binary_delta"))
        assertTrue(body["accepted_delta_algos"].toString().contains("bsdiff"))
    }

    @Test
    fun packPollRepeatsIdenticalJson() {
        var hits = 0
        val transport = RecordingTransport { req ->
            if (RecordingTransport.pathOf(req.url).endsWith("/update/pack")) {
                hits += 1
                if (hits < 3) jsonOk("""{"status":"pending"}""", 202)
                else jsonOk("""{"status":"ready","package_url":"/pkg","sha256":"00"}""")
            } else {
                errorJson(404, "NOT_FOUND", "no")
            }
        }
        val client = Client(ClientConfig("http://example.test", "p", transport = transport))
        val req = PackRequest("1.0.0", "1.1.0", "windows", "x86_64", neededPaths = listOf("a/b", "c"))
        val result = client.packUntilReady(req, PackPollOptions(initialDelayMs = 1, maxDelayMs = 2, deadlineMs = 5_000))
        assertEquals("ready", result.status)
        val bodies = transport.requests
            .filter { it.method == "POST" }
            .map { it.body!!.decodeToString() }
        assertEquals(3, bodies.size)
        assertEquals(bodies[0], bodies[1])
        assertEquals(bodies[1], bodies[2])
        assertTrue(bodies[0].contains("needed_paths"))
    }

    @Test
    fun unknownMagicDoesNotCrossDecodeAndUpdaterFallsBack() {
        assertFailsWith<UnknownDeltaMagicException> {
            DeltaMagic.require(DeltaAlgo.BSDIFF, "HDIFF13&xxxx".toByteArray())
        }
        assertTrue(DeltaMagic.matches(DeltaAlgo.XDELTA3, byteArrayOf(0xD6.toByte(), 0xC3.toByte(), 0xC4.toByte(), 1)))
        assertFalse(DeltaMagic.matches(DeltaAlgo.HDIFFPATCH, "BSDIFF40".toByteArray()))

        val full = byteArrayOf(9, 9, 9)
        val hasher = JdkHasher()
        val fullHash = hasher.sha256Hex(full)
        val check = CHECK_200.replace("\"delta_available\": false", "\"delta_available\": true")
            .replace("\"sha256\": \"aaaa\"", "\"sha256\": \"$fullHash\"")
        val transport = RecordingTransport { req ->
            when {
                RecordingTransport.pathOf(req.url).endsWith("/update/check") -> jsonOk(check)
                RecordingTransport.pathOf(req.url).endsWith("/update/diff") -> jsonOk(
                    """
                    {
                      "diff_mode": "binary_delta",
                      "root_hash": "",
                      "version_integer": null,
                      "version_semver": "1.1.0",
                      "channel": "stable",
                      "compare_engine": "semver",
                      "delta_algo": "bsdiff",
                      "package_url": "/delta",
                      "sha256": "dead"
                    }
                    """.trimIndent(),
                )
                RecordingTransport.pathOf(req.url).endsWith("/delta") ->
                    HttpResponse(200, body = "HDIFF13&nope".toByteArray())
                RecordingTransport.pathOf(req.url).contains("/packages/") ||
                    req.url.contains("aaaa") || req.url.endsWith("/abc") ->
                    HttpResponse(200, body = full)
                RecordingTransport.pathOf(req.url).endsWith("/telemetry/report") -> jsonOk("""{"status":"ok"}""", 202)
                else -> errorJson(404, "NOT_FOUND", RecordingTransport.pathOf(req.url))
            }
        }
        val client = Client(ClientConfig("http://example.test", "demo", transport = transport))
        val patcher = object : Patcher {
            override fun supportedAlgos() = listOf(DeltaAlgo.BSDIFF)
            override fun apply(oldBytes: ByteArray, delta: ByteArray, algo: String): ByteArray {
                DeltaMagic.require(algo, delta)
                return oldBytes
            }
        }
        val outcome = Updater(client, archiveUnpacker = null, patcher = patcher).run(
            UpdateIdentity("1.0.0", "windows", "x86_64", localPackage = byteArrayOf(1, 2, 3)),
        )
        assertTrue(outcome.fallbackToFull)
        assertEquals("full_package", outcome.diffMode)
        assertTrue(outcome.bytes.contentEquals(full))
    }

    @Test
    fun zipUnpackerMapsHashNamedMembers() {
        val hasher = JdkHasher()
        val payload = "hello-zip".toByteArray()
        val hash = hasher.sha256Hex(payload)
        val zipBytes = java.io.ByteArrayOutputStream().use { out ->
            ZipOutputStream(out).use { zip ->
                zip.putNextEntry(ZipEntry(hash))
                zip.write(payload)
                zip.closeEntry()
            }
            out.toByteArray()
        }
        val tmp = Files.createTempDirectory("kv-zip")
        val store = JdkFileStore(tmp)
        JdkZipUnpacker().unpack(
            zipBytes,
            listOf(
                IntegrityFile(
                    path = "bin/app.exe",
                    size = payload.size.toLong(),
                    installPolicy = "OVERWRITE",
                    sha256 = hash,
                ),
            ),
            store,
        )
        assertTrue(payload.contentEquals(store.read("bin/app.exe")))
    }

    @Test
    fun keepIfExistsOmittedFromNeededPaths() {
        val tmp = Files.createTempDirectory("kv-keep")
        val store = JdkFileStore(tmp)
        val bytes = "keep-me".toByteArray()
        store.write("data/local.dat", bytes)
        val hasher = JdkHasher()
        val transport = RecordingTransport { jsonOk(CHECK_200) }
        val client = Client(ClientConfig("http://example.test", "p", transport = transport))
        val updater = Updater(client, fileStore = store, archiveUnpacker = null)
        val needed = updater.neededPaths(
            IntegrityManifest(
                channel = "stable",
                packageType = "multi_file",
                files = listOf(
                    IntegrityFile(
                        path = "data/local.dat",
                        size = bytes.size.toLong(),
                        installPolicy = "KEEP_IF_EXISTS",
                        integrityCheck = true,
                        sha256 = hasher.sha256Hex(bytes),
                    ),
                    IntegrityFile(
                        path = "bin/app",
                        size = 1,
                        installPolicy = "OVERWRITE",
                        sha256 = "ff",
                    ),
                ),
            ),
            store,
        )
        assertEquals(listOf("bin/app"), needed)
    }

    @Test
    fun updaterWithoutReplacerStagesInsteadOfInstalling() {
        val payload = byteArrayOf(1, 2, 3, 4)
        val hash = JdkHasher().sha256Hex(payload)
        val check = CHECK_200.replace("\"sha256\": \"aaaa\"", "\"sha256\": \"$hash\"")
            .replace("\"size\": 4", "\"size\": ${payload.size}")
        val transport = RecordingTransport { req ->
            when {
                RecordingTransport.pathOf(req.url).endsWith("/update/check") -> jsonOk(check)
                RecordingTransport.pathOf(req.url).endsWith("/telemetry/report") -> jsonOk("{}", 202)
                else -> HttpResponse(200, body = payload)
            }
        }
        val client = Client(ClientConfig("http://example.test", "demo", transport = transport))
        val outcome = Updater(client, archiveUnpacker = null, replacer = null).run(
            UpdateIdentity("1.0.0", "windows", "x86_64"),
        )
        assertIs<UpdateKind>(outcome.kind)
        assertEquals(UpdateKind.STAGED, outcome.kind)
        assertFalse(outcome.applied)
        assertTrue(payload.contentEquals(outcome.bytes))
    }

    @Test
    fun checkPayloadJoinsSixFields() {
        assertEquals(
            "1\n1.1.0\nroot\nu\n10\nabc",
            CheckPayload.build(1, "1.1.0", "root", "u", 10, "abc"),
        )
        assertEquals(
            "\n\n\n\n\n",
            CheckPayload.build(null, null, null, null, null, null),
        )
    }

    @Test
    fun defaultUpdaterAdvertisesZipButNotDeltaOrFileList() {
        val transport = RecordingTransport { jsonOk(CHECK_200) }
        val client = Client(ClientConfig("http://example.test", "p", transport = transport))
        val (caps, algos) = Updater(client).capabilities()
        assertEquals(listOf(Capability.FULL_PACKAGE, Capability.PATCH_PACKAGE), caps)
        assertEquals(null, algos)
    }

    @Test
    fun packReadyDoesNotCompareArchiveHashToFullPackageSha() {
        val hasher = JdkHasher()
        val payload = "file-a".toByteArray()
        val fileHash = hasher.sha256Hex(payload)
        val zipBytes = java.io.ByteArrayOutputStream().use { out ->
            ZipOutputStream(out).use { zip ->
                zip.putNextEntry(ZipEntry(fileHash))
                zip.write(payload)
                zip.closeEntry()
            }
            out.toByteArray()
        }
        val zipHash = hasher.sha256Hex(zipBytes)
        val check = CHECK_200
            .replace("\"package_type\": \"single_file\"", "\"package_type\": \"multi_file\"")
            .replace("\"sha256\": \"aaaa\"", "\"sha256\": \"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff\"")
        val integrity = """
            {
              "version_semver": "1.1.0",
              "channel": "stable",
              "package_type": "multi_file",
              "root_hash": "",
              "full_package_url": "/full",
              "file_name": "pkg.zip",
              "size": 1,
              "sha256": "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
              "files": [{"path":"bin/app","size":${payload.size},"install_policy":"OVERWRITE","integrity_check":true,"sha256":"$fileHash"}]
            }
        """.trimIndent()
        val transport = RecordingTransport { req ->
            when {
                RecordingTransport.pathOf(req.url).endsWith("/update/check") -> jsonOk(check)
                RecordingTransport.pathOf(req.url).contains("/integrity") -> jsonOk(integrity)
                RecordingTransport.pathOf(req.url).endsWith("/update/pack") -> jsonOk(
                    """{"status":"ready","package_url":"/pkg.zip","sha256":"$zipHash","files":[{"path":"bin/app","size":${payload.size},"install_policy":"OVERWRITE","integrity_check":true,"sha256":"$fileHash"}]}""",
                )
                RecordingTransport.pathOf(req.url).endsWith("/pkg.zip") -> HttpResponse(200, body = zipBytes)
                RecordingTransport.pathOf(req.url).endsWith("/telemetry/report") -> jsonOk("{}", 202)
                else -> errorJson(404, "NOT_FOUND", RecordingTransport.pathOf(req.url))
            }
        }
        val client = Client(ClientConfig("http://example.test", "demo", transport = transport))
        val tmp = Files.createTempDirectory("kv-pack-ready")
        val store = JdkFileStore(tmp)
        val outcome = Updater(client, fileStore = store).run(
            UpdateIdentity("1.0.0", "windows", "x86_64"),
        )
        assertEquals("patch_package", outcome.diffMode)
        assertFalse(outcome.fallbackToFull)
        assertTrue(payload.contentEquals(store.read("bin/app")))
        assertFalse(store.exists("package.bin"))
    }

    @Test
    fun replacerReceivesFileStorePathNotInstallRootRelative() {
        val payload = byteArrayOf(1, 2, 3, 4)
        val hash = JdkHasher().sha256Hex(payload)
        val check = CHECK_200.replace("\"sha256\": \"aaaa\"", "\"sha256\": \"$hash\"")
            .replace("\"size\": 4", "\"size\": ${payload.size}")
        val transport = RecordingTransport { req ->
            when {
                RecordingTransport.pathOf(req.url).endsWith("/update/check") -> jsonOk(check)
                RecordingTransport.pathOf(req.url).endsWith("/telemetry/report") -> jsonOk("{}", 202)
                else -> HttpResponse(200, body = payload)
            }
        }
        val client = Client(ClientConfig("http://example.test", "demo", transport = transport))
        val stage = Files.createTempDirectory("kv-stage")
        val install = Files.createTempDirectory("kv-install")
        var seenStaged: java.nio.file.Path? = null
        var seenInstall: java.nio.file.Path? = null
        val replacer = Replacer { staged, current ->
            seenStaged = staged
            seenInstall = current
        }
        val outcome = Updater(
            client,
            fileStore = JdkFileStore(stage),
            archiveUnpacker = null,
            replacer = replacer,
        ).run(
            UpdateIdentity("1.0.0", "windows", "x86_64", installRoot = install),
        )
        assertEquals(UpdateKind.APPLIED, outcome.kind)
        assertTrue(outcome.applied)
        assertEquals(stage.resolve("package.bin").toAbsolutePath().normalize(), seenStaged?.toAbsolutePath()?.normalize())
        assertEquals(install.toAbsolutePath().normalize(), seenInstall?.toAbsolutePath()?.normalize())
        assertFalse(install.resolve("package.bin").toFile().exists())
    }

    @Test
    fun updaterForwardsCustomOnDeviceReport() {
        val transport = RecordingTransport { req ->
            when {
                RecordingTransport.pathOf(req.url).endsWith("/clients/report") ->
                    jsonOk("""{"ip":"127.0.0.1","country_code":"","region_code":"","geo_i18n":{}}""")
                RecordingTransport.pathOf(req.url).endsWith("/update/check") -> jsonOk("", 204)
                else -> errorJson(404, "NOT_FOUND", RecordingTransport.pathOf(req.url))
            }
        }
        val client = Client(ClientConfig("http://example.test", "p", transport = transport))
        Updater(client, archiveUnpacker = null).run(
            UpdateIdentity(
                currentVersion = "1.0.0",
                os = "windows",
                arch = "x86_64",
                deviceId = "dev-custom",
                custom = JsonObject(mapOf("k" to JsonPrimitive("v"))),
                reportDevice = true,
            ),
        )
        val report = transport.requests.first { RecordingTransport.pathOf(it.url).endsWith("/clients/report") }
        val body = KiriversJson.parseToJsonElement(report.body!!.decodeToString()).jsonObject
        assertEquals("v", body["custom"]!!.jsonObject["k"].toString().trim('"'))
    }
}
