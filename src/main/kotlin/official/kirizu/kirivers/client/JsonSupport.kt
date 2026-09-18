package official.kirizu.kirivers.client

import kotlinx.serialization.json.Json
import java.net.URLEncoder
import java.nio.charset.StandardCharsets

/** kotlinx.serialization codec for the client plane. JSON is not an injectable adapter. */
internal val KiriversJson: Json = Json {
    ignoreUnknownKeys = true
    explicitNulls = false
    encodeDefaults = false
    isLenient = true
}

internal fun urlEncode(value: String): String =
    URLEncoder.encode(value, StandardCharsets.UTF_8).replace("+", "%20")

internal fun queryString(params: List<Pair<String, String?>>): String {
    val parts = params.mapNotNull { (key, value) ->
        if (value.isNullOrEmpty()) null else "${urlEncode(key)}=${urlEncode(value)}"
    }
    return if (parts.isEmpty()) "" else "?" + parts.joinToString("&")
}

internal fun headerValue(headers: Map<String, List<String>>, name: String): String? =
    headers.entries.firstOrNull { it.key.equals(name, ignoreCase = true) }?.value?.firstOrNull()
