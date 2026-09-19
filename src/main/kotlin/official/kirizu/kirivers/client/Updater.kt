package official.kirizu.kirivers.client

import kotlinx.serialization.json.JsonObject
import java.nio.file.Path
import java.time.Duration

data class UpdateIdentity(
    val currentVersion: String,
    val os: String,
    val arch: String,
    val channel: String = "stable",
    val deviceId: String? = null,
    val hwRev: String? = null,
    val osVersion: String? = null,
    val custom: JsonObject? = null,
    val reportDevice: Boolean = false,
    val localPackage: ByteArray? = null,
    val installRoot: Path? = null,
)

enum class UpdateKind {
    UP_TO_DATE,
    STAGED,
    APPLIED,
}

data class UpdateOutcome(
    val kind: UpdateKind,
    val check: UpdateCheck? = null,
    val stagedPath: String? = null,
    val bytes: ByteArray? = null,
    val sha256: String? = null,
    val applied: Boolean = false,
    val fallbackToFull: Boolean = false,
    val diffMode: String? = null,
) {
    override fun equals(other: Any?): Boolean {
        if (this === other) return true
        if (other !is UpdateOutcome) return false
        return kind == other.kind &&
            check == other.check &&
            stagedPath == other.stagedPath &&
            bytes.contentEquals(other.bytes) &&
            sha256 == other.sha256 &&
            applied == other.applied &&
            fallbackToFull == other.fallbackToFull &&
            diffMode == other.diffMode
    }

    override fun hashCode(): Int {
        var result = kind.hashCode()
        result = 31 * result + (check?.hashCode() ?: 0)
        result = 31 * result + (stagedPath?.hashCode() ?: 0)
        result = 31 * result + (bytes?.contentHashCode() ?: 0)
        result = 31 * result + (sha256?.hashCode() ?: 0)
        result = 31 * result + applied.hashCode()
        result = 31 * result + fallbackToFull.hashCode()
        result = 31 * result + (diffMode?.hashCode() ?: 0)
        return result
    }
}

/**
 * High-level check → download → verify → optional patch/unpack → optional replace.
 * Missing [Replacer] is not a failure: verified bytes are returned as staged.
 * Telemetry errors never fail the outcome. Capabilities follow D13.
 */
class Updater(
    private val client: Client,
    private val fileStore: FileStore? = null,
    private val hasher: Hasher = JdkHasher(),
    private val archiveUnpacker: ArchiveUnpacker? = JdkZipUnpacker(),
    private val signatureVerifier: SignatureVerifier? = JdkSignatureVerifier(),
    private val patcher: Patcher? = null,
    private val replacer: Replacer? = null,
    private val signingKeys: List<SigningKey> = emptyList(),
    private val packOptions: PackPollOptions = PackPollOptions(),
    private val stageName: String = "package.bin",
) {
    fun capabilities(): Pair<List<String>, List<String>?> {
        val caps = linkedSetOf(Capability.FULL_PACKAGE)
        if (fileStore?.canWriteIndividualFiles() == true) caps += Capability.FILE_LIST
        if (archiveUnpacker != null) caps += Capability.PATCH_PACKAGE
        val algos = patcher?.supportedAlgos().orEmpty().filter { it.isNotBlank() }
        if (algos.isNotEmpty()) {
            caps += Capability.BINARY_DELTA
            return caps.toList() to algos
        }
        return caps.toList() to null
    }

    fun run(identity: UpdateIdentity): UpdateOutcome {
        if (identity.reportDevice && !identity.deviceId.isNullOrEmpty()) {
            runCatching {
                client.reportDevice(
                    DeviceReportRequest(
                        deviceId = identity.deviceId,
                        os = identity.os,
                        arch = identity.arch,
                        channel = identity.channel,
                        version = identity.currentVersion,
                        custom = identity.custom,
                    ),
                )
            }
        }
        val (caps, algos) = capabilities()
        val checked = client.check(
            CheckRequest(
                currentVersion = identity.currentVersion,
                os = identity.os,
                arch = identity.arch,
                channel = identity.channel,
                hwRev = identity.hwRev,
                osVersion = identity.osVersion,
                deviceId = identity.deviceId,
                capabilities = caps,
                acceptedDeltaAlgos = algos,
            ),
        )
        val update = when (checked) {
            is CheckResult.UpToDate, is CheckResult.NotModified ->
                return UpdateOutcome(kind = UpdateKind.UP_TO_DATE)
            is CheckResult.Update -> checked.update
        }
        return try {
            val (bytes, sha, mode, fallback) = obtainBytes(identity, update, caps, algos)
            if (mode == "full_package") {
                verifyHash(sha, update.sha256)
            }
            verifySignature(update)
            val staged = writeStage(bytes, mode)
            val applied = applyReplace(staged, identity.installRoot)
            report(identity, update, TelemetryStatus.INSTALLED, mode)
            UpdateOutcome(
                kind = if (applied) UpdateKind.APPLIED else UpdateKind.STAGED,
                check = update,
                stagedPath = staged,
                bytes = bytes,
                sha256 = sha,
                applied = applied,
                fallbackToFull = fallback,
                diffMode = mode,
            )
        } catch (ex: Exception) {
            report(identity, update, TelemetryStatus.FAILED, error = ex)
            throw ex
        }
    }

    private data class Obtained(
        val bytes: ByteArray,
        val sha256: String,
        val mode: String,
        val fallback: Boolean,
    )

    private fun obtainBytes(
        identity: UpdateIdentity,
        update: UpdateCheck,
        caps: List<String>,
        algos: List<String>?,
    ): Obtained {
        val wantDelta = patcher != null &&
            update.deltaAvailable &&
            !update.isDowngrade &&
            update.packageType == "single_file" &&
            identity.localPackage != null
        var deltaAttempted = false
        if (wantDelta) {
            deltaAttempted = true
            try {
                return applyDelta(identity, update, caps, algos)
            } catch (_: Exception) {
                // Unknown magic, hash miss, or diff failure → full package.
            }
        }
        if (update.packageType == "multi_file" && fileStore != null && archiveUnpacker != null) {
            val packed = runPack(identity, update)
            if (packed != null) return packed
        }
        val body = client.download(update.packageUrl).body
        val hex = hasher.sha256Hex(body)
        if (update.sha256.isNotEmpty() && !hex.equals(update.sha256, ignoreCase = true)) {
            throw HashMismatchException(update.sha256, hex)
        }
        return Obtained(body, hex, "full_package", fallback = deltaAttempted)
    }

    private fun applyDelta(
        identity: UpdateIdentity,
        update: UpdateCheck,
        caps: List<String>,
        algos: List<String>?,
    ): Obtained {
        val live = patcher ?: throw UnknownDeltaMagicException("none")
        val local = identity.localPackage ?: throw UnknownDeltaMagicException("none")
        val localHash = hasher.sha256Hex(local)
        val target = update.versionSemver ?: update.versionInteger?.toString()
            ?: throw ApiException(400, "INVALID_REQUEST", "check response missing target version")
        val diff = client.diff(
            DiffRequest(
                sourceVersion = identity.currentVersion,
                targetVersion = target,
                os = identity.os,
                arch = identity.arch,
                channel = identity.channel,
                deviceId = identity.deviceId,
                hwRev = identity.hwRev,
                localSha256 = localHash,
                capabilities = caps,
                acceptedDeltaAlgos = algos,
            ),
        )
        if (diff.diffMode != "binary_delta" || diff.packageUrl.isNullOrEmpty()) {
            val body = client.download(update.packageUrl).body
            return Obtained(body, hasher.sha256Hex(body), "full_package", fallback = true)
        }
        val algo = diff.deltaAlgo ?: update.deltaAlgo ?: live.supportedAlgos().first()
        val delta = client.download(diff.packageUrl).body
        DeltaMagic.require(algo, delta)
        val patched = live.apply(local, delta, algo)
        val hex = hasher.sha256Hex(patched)
        if (update.sha256.isNotEmpty() && !hex.equals(update.sha256, ignoreCase = true)) {
            throw HashMismatchException(update.sha256, hex)
        }
        return Obtained(patched, hex, "binary_delta", fallback = false)
    }

    private fun runPack(identity: UpdateIdentity, update: UpdateCheck): Obtained? {
        val store = fileStore ?: return null
        val target = update.versionSemver ?: update.versionInteger?.toString() ?: return null
        val integrity = when (
            val cached = client.integrity(
                IntegrityQuery(
                    version = target,
                    os = identity.os,
                    arch = identity.arch,
                    channel = identity.channel,
                    hwRev = identity.hwRev,
                ),
            )
        ) {
            is Cached.NotModified -> return null
            is Cached.Fresh -> cached.value
        }
        val needed = neededPaths(integrity, store)
        val pack = client.packUntilReady(
            PackRequest(
                sourceVersion = identity.currentVersion,
                targetVersion = target,
                os = identity.os,
                arch = identity.arch,
                channel = identity.channel,
                deviceId = identity.deviceId,
                hwRev = identity.hwRev,
                neededPaths = needed,
            ),
            packOptions,
        )
        return when (pack.status) {
            "full_package" -> {
                val url = pack.packageUrl ?: update.packageUrl
                val body = client.download(url).body
                Obtained(body, hasher.sha256Hex(body), "full_package", fallback = true)
            }
            "ready" -> {
                val url = pack.packageUrl ?: return null
                val archive = client.download(url).body
                if (!pack.sha256.isNullOrEmpty()) {
                    val hex = hasher.sha256Hex(archive)
                    if (!hex.equals(pack.sha256, ignoreCase = true)) {
                        throw HashMismatchException(pack.sha256, hex)
                    }
                }
                val unpacker = archiveUnpacker ?: return null
                unpacker.unpack(archive, pack.files ?: integrity.files, store)
                Obtained(archive, hasher.sha256Hex(archive), "patch_package", fallback = false)
            }
            else -> null
        }
    }

    internal fun neededPaths(manifest: IntegrityManifest, store: FileStore): List<String> {
        val needed = LinkedHashSet<String>()
        for (file in manifest.files) {
            val path = try {
                InstallPaths.normalize(file.path)
            } catch (_: InvalidPathException) {
                continue
            }
            val local = store.read(path)
            val keep = file.installPolicy == "KEEP_IF_EXISTS" && local != null
            if (keep) {
                if (!file.integrityCheck) continue
                val expected = file.sha256
                if (expected.isNullOrEmpty()) continue
                if (hasher.sha256Hex(local).equals(expected, ignoreCase = true)) continue
            }
            needed += path
        }
        return needed.toList()
    }

    private fun verifyHash(actual: String, expected: String?) {
        if (expected.isNullOrEmpty()) return
        if (!actual.equals(expected, ignoreCase = true)) {
            throw HashMismatchException(expected, actual)
        }
    }

    private fun verifySignature(update: UpdateCheck) {
        val signature = update.signature ?: return
        val verifier = signatureVerifier ?: return
        if (signingKeys.isEmpty()) return
        val payload = CheckPayload.build(
            versionInteger = update.versionInteger,
            versionSemver = update.versionSemver,
            rootHash = update.rootHash,
            packageUrl = update.packageUrl,
            size = update.size,
            sha256 = update.sha256,
        ).toByteArray(Charsets.UTF_8)
        val ok = signingKeys.any { key ->
            runCatching {
                verifier.verify(key.algorithm, key.publicKey, payload, signature)
            }.getOrDefault(false)
        }
        if (!ok) throw SignatureVerifyException("check signature mismatch")
    }

    private fun writeStage(bytes: ByteArray, mode: String): String? {
        val store = fileStore ?: return null
        if (mode == "patch_package") {
            // Members are already unpacked into the store; do not drop the zip in as package.bin.
            return store.resolvePath("")?.toString()
        }
        store.write(stageName, bytes)
        return store.resolvePath(stageName)?.toString() ?: stageName
    }

    private fun applyReplace(stagedPath: String?, installRoot: Path?): Boolean {
        val live = replacer ?: return false
        val staged = stagedPath ?: return false
        val root = installRoot ?: return false
        live.replace(Path.of(staged), root)
        return true
    }

    private fun report(
        identity: UpdateIdentity,
        update: UpdateCheck,
        status: String,
        diffMode: String? = null,
        error: Exception? = null,
    ) {
        val to = update.versionSemver ?: update.versionInteger?.toString() ?: identity.currentVersion
        runCatching {
            client.reportTelemetry(
                TelemetryRequest(
                    os = identity.os,
                    arch = identity.arch,
                    channel = identity.channel,
                    fromVersion = identity.currentVersion,
                    toVersion = to,
                    status = status,
                    deviceId = identity.deviceId,
                    diffMode = diffMode,
                    errorCode = (error as? ApiException)?.code,
                    errorMessage = error?.message?.take(200),
                ),
            )
        }
    }

    companion object {
        fun packBackoffDelay(step: Int, initial: Duration = Duration.ofSeconds(1), cap: Duration = Duration.ofSeconds(15)): Duration {
            var ms = initial.toMillis()
            repeat(step) { ms = minOf(ms * 2, cap.toMillis()) }
            return Duration.ofMillis(ms)
        }
    }
}
