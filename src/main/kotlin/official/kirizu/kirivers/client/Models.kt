package official.kirizu.kirivers.client

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject

@Serializable
data class ErrorEnvelope(val error: ErrorBody? = null)

@Serializable
data class ErrorBody(
    val code: String = "UNKNOWN",
    val message: String = "",
    val details: JsonElement? = null,
)

@Serializable
data class HealthStatus(
    val status: String,
    val ready: Boolean,
)

@Serializable
data class ProjectPublic(
    val uuid: String? = null,
    val slug: String? = null,
    @SerialName("compare_engine") val compareEngine: String? = null,
    @SerialName("created_at") val createdAt: String? = null,
    @SerialName("updated_at") val updatedAt: String? = null,
    @SerialName("default_locale") val defaultLocale: String? = null,
    @SerialName("device_id_policy") val deviceIdPolicy: String? = null,
    @SerialName("force_https") val forceHttps: Boolean? = null,
    @SerialName("minimum_supported_version") val minimumSupportedVersion: String? = null,
    @SerialName("require_client_token") val requireClientToken: Boolean? = null,
    @SerialName("storage_visibility") val storageVisibility: String? = null,
)

@Serializable
data class DeviceReportRequest(
    @SerialName("device_id") val deviceId: String,
    val os: String? = null,
    val arch: String? = null,
    val channel: String? = null,
    val version: String? = null,
    val custom: JsonObject? = null,
)

@Serializable
data class DeviceGeo(
    val ip: String = "",
    @SerialName("country_code") val countryCode: String = "",
    @SerialName("region_code") val regionCode: String = "",
    @SerialName("geo_i18n") val geoI18n: JsonObject = JsonObject(emptyMap()),
)

@Serializable
data class CheckRequest(
    @SerialName("current_version") val currentVersion: String,
    val os: String,
    val arch: String,
    val channel: String? = null,
    @SerialName("hw_rev") val hwRev: String? = null,
    @SerialName("os_version") val osVersion: String? = null,
    @SerialName("device_id") val deviceId: String? = null,
    val capabilities: List<String>? = null,
    @SerialName("accepted_delta_algos") val acceptedDeltaAlgos: List<String>? = null,
)

@Serializable
data class UpdateCheck(
    @SerialName("has_update") val hasUpdate: Boolean,
    @SerialName("is_mandatory") val isMandatory: Boolean,
    @SerialName("is_downgrade") val isDowngrade: Boolean,
    val reason: String,
    @SerialName("compare_engine") val compareEngine: String,
    @SerialName("version_integer") val versionInteger: Long? = null,
    @SerialName("version_semver") val versionSemver: String? = null,
    @SerialName("target_channel") val targetChannel: String,
    @SerialName("target_hw_rev") val targetHwRev: String? = null,
    @SerialName("package_type") val packageType: String,
    @SerialName("publish_time") val publishTime: String? = null,
    @SerialName("platform_notes") val platformNotes: String? = null,
    @SerialName("root_hash") val rootHash: String = "",
    @SerialName("package_url") val packageUrl: String = "",
    @SerialName("file_name") val fileName: String = "",
    val size: Long = 0,
    val sha256: String = "",
    @SerialName("delta_available") val deltaAvailable: Boolean = false,
    @SerialName("delta_algo") val deltaAlgo: String? = null,
    val signature: String? = null,
    @SerialName("artifact_signature") val artifactSignature: String? = null,
)

sealed class CheckResult {
    data class Update(val update: UpdateCheck, val etag: String? = null) : CheckResult()
    data class UpToDate(val etag: String? = null) : CheckResult()
    data class NotModified(val etag: String? = null) : CheckResult()
}

@Serializable
data class Changelog(
    val changelog: String? = null,
    @SerialName("changelog_versions") val changelogVersions: List<ChangelogVersion>? = null,
)

@Serializable
data class ChangelogVersion(
    val channel: String,
    val status: String,
    val changelog: String,
    @SerialName("had_artifact_for_request_platform") val hadArtifactForRequestPlatform: Boolean,
    @SerialName("version_integer") val versionInteger: Long? = null,
    @SerialName("version_semver") val versionSemver: String? = null,
    val title: String? = null,
    @SerialName("platform_notes") val platformNotes: String? = null,
)

data class ChangelogQuery(
    val channel: String,
    val os: String,
    val arch: String,
    val fromVersion: String? = null,
    val toVersion: String? = null,
    val scope: String? = null,
    val layout: String? = null,
    val includeRevoked: Boolean? = null,
    val includePlatformNotes: Boolean? = null,
    val changelogLocale: String? = null,
    val locale: String? = null,
    val ifNoneMatch: String? = null,
)

sealed class Cached<out T> {
    data class Fresh<T>(val value: T, val etag: String? = null) : Cached<T>()
    data class NotModified(val etag: String? = null) : Cached<Nothing>()
}

@Serializable
data class IntegrityManifest(
    @SerialName("version_integer") val versionInteger: Long? = null,
    @SerialName("version_semver") val versionSemver: String? = null,
    val channel: String,
    @SerialName("package_type") val packageType: String,
    @SerialName("root_hash") val rootHash: String = "",
    @SerialName("full_package_url") val fullPackageUrl: String = "",
    @SerialName("file_name") val fileName: String = "",
    val size: Long = 0,
    val sha256: String = "",
    val signature: String? = null,
    val files: List<IntegrityFile> = emptyList(),
    val volumes: List<IntegrityVolume>? = null,
)

@Serializable
data class IntegrityFile(
    val path: String,
    val size: Long = 0,
    @SerialName("install_policy") val installPolicy: String,
    @SerialName("integrity_check") val integrityCheck: Boolean = true,
    val sha256: String? = null,
    val md5: String? = null,
    val url: String? = null,
)

@Serializable
data class IntegrityVolume(
    val sha256: String? = null,
    val size: Long? = null,
    val url: String? = null,
)

data class IntegrityQuery(
    val version: String,
    val os: String,
    val arch: String,
    val channel: String? = null,
    val hwRev: String? = null,
    val hashAlgo: String? = null,
    val compact: Boolean? = null,
    val includeFileUrls: Boolean? = null,
    val ifNoneMatch: String? = null,
)

@Serializable
data class DiffRequest(
    @SerialName("source_version") val sourceVersion: String,
    @SerialName("target_version") val targetVersion: String,
    val os: String,
    val arch: String,
    val channel: String? = null,
    @SerialName("device_id") val deviceId: String? = null,
    @SerialName("hw_rev") val hwRev: String? = null,
    @SerialName("local_sha256") val localSha256: String? = null,
    val capabilities: List<String>? = null,
    @SerialName("accepted_delta_algos") val acceptedDeltaAlgos: List<String>? = null,
    @SerialName("prefer_full") val preferFull: Boolean? = null,
)

@Serializable
data class DiffResponse(
    @SerialName("diff_mode") val diffMode: String,
    @SerialName("root_hash") val rootHash: String = "",
    @SerialName("version_integer") val versionInteger: Long? = null,
    @SerialName("version_semver") val versionSemver: String? = null,
    val channel: String,
    @SerialName("compare_engine") val compareEngine: String,
    @SerialName("delta_algo") val deltaAlgo: String? = null,
    @SerialName("file_name") val fileName: String? = null,
    @SerialName("package_url") val packageUrl: String? = null,
    val sha256: String? = null,
    val size: Long? = null,
    val signature: String? = null,
    val files: List<IntegrityFile>? = null,
    @SerialName("deleted_paths") val deletedPaths: List<String>? = null,
    @SerialName("invalid_paths") val invalidPaths: List<String>? = null,
)

@Serializable
data class PackRequest(
    @SerialName("source_version") val sourceVersion: String,
    @SerialName("target_version") val targetVersion: String,
    val os: String,
    val arch: String,
    val channel: String? = null,
    @SerialName("device_id") val deviceId: String? = null,
    @SerialName("hw_rev") val hwRev: String? = null,
    @SerialName("needed_paths") val neededPaths: List<String>? = null,
)

@Serializable
data class PackResponse(
    val status: String,
    val channel: String? = null,
    @SerialName("compare_engine") val compareEngine: String? = null,
    val compression: String? = null,
    @SerialName("diff_mode") val diffMode: String? = null,
    @SerialName("file_name") val fileName: String? = null,
    @SerialName("package_url") val packageUrl: String? = null,
    @SerialName("root_hash") val rootHash: String? = null,
    val sha256: String? = null,
    val signature: String? = null,
    val size: Long? = null,
    @SerialName("version_integer") val versionInteger: Long? = null,
    @SerialName("version_semver") val versionSemver: String? = null,
    val files: List<IntegrityFile>? = null,
    @SerialName("deleted_paths") val deletedPaths: List<String>? = null,
    @SerialName("invalid_paths") val invalidPaths: List<String>? = null,
)

@Serializable
data class TelemetryRequest(
    val os: String,
    val arch: String,
    val channel: String,
    @SerialName("from_version") val fromVersion: String,
    @SerialName("to_version") val toVersion: String,
    val status: String,
    @SerialName("device_id") val deviceId: String? = null,
    @SerialName("diff_mode") val diffMode: String? = null,
    @SerialName("error_code") val errorCode: String? = null,
    @SerialName("error_message") val errorMessage: String? = null,
)

@Serializable
data class TelemetryAccepted(val status: String? = null)

@Serializable
data class ChannelList(val channels: List<ClientChannel> = emptyList())

@Serializable
data class ClientChannel(
    val name: String? = null,
    val slug: String? = null,
    @SerialName("stability_rank") val stabilityRank: Int? = null,
)

@Serializable
data class MatrixList(val matrix: List<MatrixRow> = emptyList())

@Serializable
data class MatrixRow(
    val os: String? = null,
    val arch: String? = null,
    @SerialName("package_type") val packageType: String? = null,
)

@Serializable
data class LanguageList(val languages: List<ProjectLanguage> = emptyList())

@Serializable
data class ProjectLanguage(
    val code: String? = null,
    @SerialName("display_name") val displayName: String? = null,
    @SerialName("is_default") val isDefault: Boolean? = null,
    @SerialName("sort_order") val sortOrder: Int? = null,
)

@Serializable
data class AnnouncementList(val announcements: List<Announcement> = emptyList())

@Serializable
data class Announcement(
    val id: String? = null,
    val locale: String? = null,
    val title: String? = null,
    val subtitle: String? = null,
    val markdown: String? = null,
    @SerialName("starts_at") val startsAt: String? = null,
    @SerialName("ends_at") val endsAt: String? = null,
)

data class AnnouncementQuery(
    val version: String? = null,
    val os: String? = null,
    val arch: String? = null,
    val locale: String? = null,
    val acceptLanguage: String? = null,
    val ifNoneMatch: String? = null,
)

data class BinaryBody(
    val status: Int,
    val headers: Map<String, List<String>>,
    val body: ByteArray,
) {
    fun header(name: String): String? = headerValue(headers, name)
    override fun equals(other: Any?): Boolean {
        if (this === other) return true
        if (other !is BinaryBody) return false
        return status == other.status && headers == other.headers && body.contentEquals(other.body)
    }
    override fun hashCode(): Int = 31 * status + headers.hashCode() + body.contentHashCode()
}

data class PackPollOptions(
    val initialDelayMs: Long = 1_000,
    val maxDelayMs: Long = 15_000,
    val deadlineMs: Long = 120_000,
)
