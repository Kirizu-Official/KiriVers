package official.kirizu.kirivers.client

import java.text.Normalizer

/**
 * Install-path rules matching the server `pathutil` contract: `/` separators, Unicode NFC,
 * reject `.` / `..`, drive letters, leading slashes, and control characters.
 */
object InstallPaths {
    private val driveLetter = Regex("^[a-zA-Z]:")

    fun normalize(raw: String): String {
        val trimmed = raw.trim()
        if (trimmed.isEmpty()) throw InvalidPathException(raw)
        for (ch in trimmed) {
            if (ch.code < 0x20) throw InvalidPathException(raw)
        }
        if (trimmed.startsWith("/") || trimmed.startsWith("\\")) throw InvalidPathException(raw)
        if (driveLetter.containsMatchIn(trimmed)) throw InvalidPathException(raw)
        val slashed = trimmed.replace('\\', '/')
        for (seg in slashed.split('/')) {
            if (driveLetter.containsMatchIn(seg)) throw InvalidPathException(raw)
            if (seg == "." || seg == "..") throw InvalidPathException(raw)
        }
        val nfc = Normalizer.normalize(slashed, Normalizer.Form.NFC)
        val cleaned = nfc.replace(Regex("/+"), "/").trim('/')
        if (cleaned.isEmpty()) throw InvalidPathException(raw)
        for (seg in cleaned.split('/')) {
            if (seg == "." || seg == ".." || seg.isEmpty()) throw InvalidPathException(raw)
        }
        return cleaned
    }
}
