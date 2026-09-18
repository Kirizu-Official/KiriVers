package official.kirizu.kirivers.client

/**
 * Unified error envelope from the client plane: `{ "error": { "code", "message", "details" } }`.
 * [code] is a stable token; unknown codes stay opaque strings.
 */
open class KiriversException(message: String) : RuntimeException(message)

class ApiException(
    val status: Int,
    val code: String,
    override val message: String,
    val details: String? = null,
    val retryAfter: Int? = null,
) : KiriversException("$code: $message")

class HashMismatchException(
    val expected: String,
    val actual: String,
) : KiriversException("sha256 mismatch: expected $expected actual $actual")

class InvalidPathException(path: String) : KiriversException("invalid install path: $path")

class UnknownDeltaMagicException(
    val algo: String,
) : KiriversException("unknown or mismatched delta magic for algo $algo")

class SignatureVerifyException(message: String) : KiriversException(message)

class PackTimeoutException : KiriversException("pack still pending after deadline")

class MissingTransportException : KiriversException("Transport is required for network methods")
