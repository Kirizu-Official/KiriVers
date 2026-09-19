package official.kirizu.kirivers.client

import java.nio.file.Files
import java.nio.file.Path
import java.security.MessageDigest
import java.text.Normalizer
import java.util.zip.ZipInputStream

/** SHA-256 (and MD5 when integrity asks). Default: JDK [MessageDigest]. */
interface Hasher {
    fun sha256Hex(bytes: ByteArray): String
    fun md5Hex(bytes: ByteArray): String
}

/** Stage downloads and compare a local tree. Paths are `/` + NFC. */
interface FileStore {
    /** When true, check may advertise `file_list`. */
    fun canWriteIndividualFiles(): Boolean = true
    fun write(relativePath: String, bytes: ByteArray)
    fun read(relativePath: String): ByteArray?
    fun exists(relativePath: String): Boolean
    fun list(relativeRoot: String = ""): List<String>
    /**
     * Filesystem location of [relativePath], or the store root when [relativePath] is empty.
     * Null when this store is not path-backed.
     */
    fun resolvePath(relativePath: String): Path? = null
}

/**
 * Apply a binary delta. Official SDK does not bundle JNI/hpatchz.
 * Unknown magic must fail — never cross-decode.
 */
interface Patcher {
    fun supportedAlgos(): List<String>
    fun apply(oldBytes: ByteArray, delta: ByteArray, algo: String): ByteArray
}

/** Replace a running install. No default: this SDK does not claim one-click install. */
fun interface Replacer {
    fun replace(staged: Path, currentInstall: Path)
}

/** Unpack a native zip whose members are content hashes mapped through [IntegrityFile.path]. */
fun interface ArchiveUnpacker {
    fun unpack(archive: ByteArray, files: List<IntegrityFile>, store: FileStore)
}

data class SigningKey(
    val algorithm: String,
    val publicKey: ByteArray,
) {
    override fun equals(other: Any?): Boolean {
        if (this === other) return true
        if (other !is SigningKey) return false
        return algorithm == other.algorithm && publicKey.contentEquals(other.publicKey)
    }
    override fun hashCode(): Int = 31 * algorithm.hashCode() + publicKey.contentHashCode()
}

fun interface SignatureVerifier {
    fun verify(algorithm: String, publicKey: ByteArray, payload: ByteArray, signatureB64: String): Boolean
}

class JdkHasher : Hasher {
    override fun sha256Hex(bytes: ByteArray): String = hex("SHA-256", bytes)
    override fun md5Hex(bytes: ByteArray): String = hex("MD5", bytes)

    private fun hex(algo: String, bytes: ByteArray): String {
        val digest = MessageDigest.getInstance(algo).digest(bytes)
        return digest.joinToString("") { b -> "%02x".format(b) }
    }
}

class JdkFileStore(private val root: Path) : FileStore {
    override fun write(relativePath: String, bytes: ByteArray) {
        val target = resolve(relativePath)
        Files.createDirectories(target.parent)
        Files.write(target, bytes)
    }

    override fun read(relativePath: String): ByteArray? {
        val target = resolve(relativePath)
        if (!Files.isRegularFile(target)) return null
        return Files.readAllBytes(target)
    }

    override fun exists(relativePath: String): Boolean = Files.exists(resolve(relativePath))

    override fun list(relativeRoot: String): List<String> {
        val start = if (relativeRoot.isEmpty()) root else resolve(relativeRoot)
        if (!Files.exists(start)) return emptyList()
        val rootNorm = root.toAbsolutePath().normalize()
        return Files.walk(start).use { stream ->
            stream.filter { Files.isRegularFile(it) }.map { path ->
                val rel = rootNorm.relativize(path.toAbsolutePath().normalize()).toString().replace('\\', '/')
                Normalizer.normalize(rel, Normalizer.Form.NFC)
            }.toList()
        }
    }

    override fun resolvePath(relativePath: String): Path =
        if (relativePath.isEmpty()) root.toAbsolutePath().normalize() else resolve(relativePath)

    private fun resolve(relativePath: String): Path {
        val normalized = InstallPaths.normalize(relativePath)
        val rootNorm = root.toAbsolutePath().normalize()
        val resolved = rootNorm.resolve(normalized).normalize()
        if (!resolved.startsWith(rootNorm)) throw InvalidPathException(relativePath)
        return resolved
    }
}

class JdkZipUnpacker : ArchiveUnpacker {
    private val hasher = JdkHasher()

    override fun unpack(archive: ByteArray, files: List<IntegrityFile>, store: FileStore) {
        val byHash = files.mapNotNull { file ->
            val hash = file.sha256?.lowercase() ?: return@mapNotNull null
            hash to file
        }.toMap()
        ZipInputStream(archive.inputStream()).use { zip ->
            while (true) {
                val entry = zip.nextEntry ?: break
                if (entry.isDirectory) continue
                val bytes = zip.readBytes()
                val nameHash = entry.name.substringAfterLast('/').substringBefore('.').lowercase()
                val file = byHash[nameHash] ?: byHash[hasher.sha256Hex(bytes)]
                if (file != null) {
                    store.write(file.path, bytes)
                }
            }
        }
    }
}
