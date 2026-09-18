package official.kirizu.kirivers.client

import java.security.KeyFactory
import java.security.Signature
import java.security.spec.X509EncodedKeySpec
import java.util.Base64

/**
 * Check / integrity / diff payload: `integer\nsemver\nroot_hash\npackage_url\nsize\nsha256`.
 * Empty fields stay as empty strings so position is stable.
 */
object CheckPayload {
    fun build(
        versionInteger: Long?,
        versionSemver: String?,
        rootHash: String?,
        packageUrl: String?,
        size: Long?,
        sha256: String?,
    ): String = listOf(
        versionInteger?.toString() ?: "",
        versionSemver ?: "",
        rootHash ?: "",
        packageUrl ?: "",
        size?.toString() ?: "",
        sha256 ?: "",
    ).joinToString("\n")
}

class JdkSignatureVerifier : SignatureVerifier {
    override fun verify(
        algorithm: String,
        publicKey: ByteArray,
        payload: ByteArray,
        signatureB64: String,
    ): Boolean {
        val sigBytes = Base64.getDecoder().decode(signatureB64)
        val der = pemToDer(publicKey)
        return when (algorithm.lowercase()) {
            "ed25519" -> {
                val key = ed25519Key(der)
                val sig = Signature.getInstance("Ed25519")
                sig.initVerify(key)
                sig.update(payload)
                sig.verify(sigBytes)
            }
            "rsa-sha256" -> {
                val key = KeyFactory.getInstance("RSA").generatePublic(X509EncodedKeySpec(der))
                val sig = Signature.getInstance("SHA256withRSA")
                sig.initVerify(key)
                sig.update(payload)
                sig.verify(sigBytes)
            }
            else -> false
        }
    }

    private fun ed25519Key(der: ByteArray) = try {
        KeyFactory.getInstance("Ed25519").generatePublic(X509EncodedKeySpec(der))
    } catch (_: Exception) {
        if (der.size != 32) throw SignatureVerifyException("ed25519 public key must be SPKI or 32 raw bytes")
        val spki = ED25519_SPKI_PREFIX + der
        KeyFactory.getInstance("Ed25519").generatePublic(X509EncodedKeySpec(spki))
    }

    private fun pemToDer(material: ByteArray): ByteArray {
        val text = material.toString(Charsets.US_ASCII)
        if (!text.contains("BEGIN")) return material
        val b64 = text.lineSequence()
            .filter { !it.startsWith("-----") }
            .joinToString("")
            .filter { !it.isWhitespace() }
        return Base64.getDecoder().decode(b64)
    }

    private companion object {
        // SubjectPublicKeyInfo prefix for a 32-byte Ed25519 key.
        val ED25519_SPKI_PREFIX = byteArrayOf(
            0x30, 0x2a, 0x30, 0x05, 0x06, 0x03, 0x2b, 0x65, 0x70, 0x03, 0x21, 0x00,
        )
    }
}
