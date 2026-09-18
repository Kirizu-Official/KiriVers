package official.kirizu.kirivers.client

object Capability {
    const val FULL_PACKAGE = "full_package"
    const val FILE_LIST = "file_list"
    const val PATCH_PACKAGE = "patch_package"
    const val BINARY_DELTA = "binary_delta"
}

object DeltaAlgo {
    const val HDIFFPATCH = "hdiffpatch"
    const val BSDIFF = "bsdiff"
    const val XDELTA3 = "xdelta3"
}

object TelemetryStatus {
    const val DOWNLOADING = "downloading"
    const val APPLYING = "applying"
    const val INSTALLED = "installed"
    const val FAILED = "failed"
    const val ROLLED_BACK = "rolled_back"
}

object DeltaMagic {
    const val KVDIFFHP1 = "KVDIFFHP1\n"
    const val HDIFF13 = "HDIFF13&"
    const val BSDIFF40 = "BSDIFF40"
    val VCDIFF: ByteArray = byteArrayOf(0xD6.toByte(), 0xC3.toByte(), 0xC4.toByte())

    fun matches(algo: String, delta: ByteArray): Boolean =
        when (algo.lowercase()) {
            DeltaAlgo.HDIFFPATCH -> startsWith(delta, KVDIFFHP1) || startsWith(delta, HDIFF13)
            DeltaAlgo.BSDIFF -> startsWith(delta, BSDIFF40)
            DeltaAlgo.XDELTA3 -> delta.size >= 3 &&
                delta[0] == VCDIFF[0] && delta[1] == VCDIFF[1] && delta[2] == VCDIFF[2]
            else -> false
        }

    fun require(algo: String, delta: ByteArray) {
        if (!matches(algo, delta)) throw UnknownDeltaMagicException(algo)
    }

    private fun startsWith(data: ByteArray, ascii: String): Boolean {
        val prefix = ascii.toByteArray(Charsets.US_ASCII)
        if (data.size < prefix.size) return false
        return prefix.indices.all { data[it] == prefix[it] }
    }
}
