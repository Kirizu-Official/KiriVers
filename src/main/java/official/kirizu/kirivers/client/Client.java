package official.kirizu.kirivers.client;

import official.kirizu.kirivers.client.adapter.HttpRequest;
import official.kirizu.kirivers.client.adapter.HttpResponse;
import official.kirizu.kirivers.client.model.AnnouncementList;
import official.kirizu.kirivers.client.model.AnnouncementsQuery;
import official.kirizu.kirivers.client.model.ChangelogQuery;
import official.kirizu.kirivers.client.model.ChangelogResponse;
import official.kirizu.kirivers.client.model.ClientChannelList;
import official.kirizu.kirivers.client.model.ClientLanguageList;
import official.kirizu.kirivers.client.model.ClientLoginInput;
import official.kirizu.kirivers.client.model.ClientMatrixList;
import official.kirizu.kirivers.client.model.ClientReportOutput;
import official.kirizu.kirivers.client.model.Diff200;
import official.kirizu.kirivers.client.model.DiffRequest;
import official.kirizu.kirivers.client.model.Integrity200;
import official.kirizu.kirivers.client.model.IntegrityQuery;
import official.kirizu.kirivers.client.model.Pack200;
import official.kirizu.kirivers.client.model.PackRequest;
import official.kirizu.kirivers.client.model.ProjectPublic;
import official.kirizu.kirivers.client.model.StatusOutput;
import official.kirizu.kirivers.client.model.TelemetryAccepted;
import official.kirizu.kirivers.client.model.TelemetryRequest;
import official.kirizu.kirivers.client.model.UpdateCheck200;
import official.kirizu.kirivers.client.model.UpdateCheckRequest;

import java.io.IOException;
import java.net.URI;
import java.nio.charset.StandardCharsets;
import java.time.Duration;
import java.time.Instant;
import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.Objects;

/**
 * Handwritten native JSON client for the KiriVers client plane. No OpenAPI codegen.
 * Callers supply {@code device_id}; this type never generates one and never logs it.
 */
public final class Client {

  private final Config config;

  public Client(Config config) {
    this.config = Objects.requireNonNull(config, "config");
    if (config.transport() == null) {
      throw new IllegalArgumentException("Transport is required");
    }
  }

  public Config config() {
    return config;
  }

  public StatusOutput health() {
    HttpResponse resp = send("GET", abs(NativePaths.HEALTH), acceptJson(), null, false);
    requireOk(resp, 200);
    return readJson(resp, StatusOutput.class);
  }

  public ProjectPublic project() {
    HttpResponse resp = send("GET", abs(NativePaths.project(config.projectRef())), acceptJson(), null, true);
    requireOk(resp, 200);
    return readJson(resp, ProjectPublic.class);
  }

  public ClientReportOutput deviceReport(ClientLoginInput input) {
    Objects.requireNonNull(input, "input");
    if (input.deviceId == null || input.deviceId.isBlank()) {
      throw new IllegalArgumentException("device_id is required");
    }
    HttpResponse resp =
        send("POST", abs(NativePaths.deviceReport(config.projectRef())), jsonHeaders(), writeJson(input), true);
    requireOk(resp, 200);
    return readJson(resp, ClientReportOutput.class);
  }

  public CheckOutcome check(UpdateCheckRequest request) {
    return check(request, null);
  }

  public CheckOutcome check(UpdateCheckRequest request, String ifNoneMatch) {
    Objects.requireNonNull(request, "request");
    if (request.currentVersion == null || request.os == null || request.arch == null) {
      throw new IllegalArgumentException("current_version, os and arch are required");
    }
    UpdateCheckRequest body = copyCheck(request);
    if (body.capabilities == null || body.capabilities.isEmpty()) {
      body.capabilities = new ArrayList<>(config.capabilities());
    }
    if (!body.capabilities.contains("full_package")) {
      body.capabilities.add(0, "full_package");
    }
    if (body.acceptedDeltaAlgos == null) {
      List<String> algos = config.acceptedDeltaAlgos();
      body.acceptedDeltaAlgos = algos.isEmpty() ? null : new ArrayList<>(algos);
    } else {
      body.acceptedDeltaAlgos = sanitizeAlgos(body.acceptedDeltaAlgos);
    }
    Map<String, String> headers = jsonHeaders();
    if (ifNoneMatch != null && !ifNoneMatch.isBlank()) {
      headers.put("If-None-Match", ifNoneMatch);
    }
    HttpResponse resp =
        send("POST", abs(NativePaths.check(config.projectRef())), headers, writeJson(body), true);
    String etag = resp.header("ETag");
    if (resp.status() == 304) {
      return new CheckOutcome(CheckOutcome.Status.NOT_MODIFIED, null, etag, resp);
    }
    if (resp.status() == 204) {
      return new CheckOutcome(CheckOutcome.Status.NO_UPDATE, null, etag, resp);
    }
    requireOk(resp, 200);
    return new CheckOutcome(CheckOutcome.Status.UPDATE, readJson(resp, UpdateCheck200.class), etag, resp);
  }

  public ResourceOutcome<ChangelogResponse> changelog(String channel, String os, String arch, ChangelogQuery query) {
    Objects.requireNonNull(channel, "channel");
    Objects.requireNonNull(os, "os");
    Objects.requireNonNull(arch, "arch");
    ChangelogQuery q = query == null ? new ChangelogQuery() : query;
    String path =
        NativePaths.withQuery(
            NativePaths.changelog(config.projectRef(), channel, os, arch),
            "from_version",
            q.fromVersion,
            "to_version",
            q.toVersion,
            "changelog_scope",
            q.changelogScope,
            "changelog_layout",
            q.changelogLayout,
            "changelog_include_revoked",
            bool(q.changelogIncludeRevoked),
            "changelog_include_platform_notes",
            bool(q.changelogIncludePlatformNotes),
            "changelog_locale",
            q.changelogLocale,
            "locale",
            q.locale);
    Map<String, String> headers = acceptJson();
    if (q.ifNoneMatch != null && !q.ifNoneMatch.isBlank()) {
      headers.put("If-None-Match", q.ifNoneMatch);
    }
    HttpResponse resp = send("GET", abs(path), headers, null, true);
    return jsonGet(resp, ChangelogResponse.class);
  }

  public ResourceOutcome<Integrity200> integrity(String version, IntegrityQuery query) {
    Objects.requireNonNull(version, "version");
    Objects.requireNonNull(query, "query");
    if (query.os == null || query.arch == null) {
      throw new IllegalArgumentException("os and arch are required");
    }
    String path =
        NativePaths.withQuery(
            NativePaths.integrity(config.projectRef(), version),
            "os",
            query.os,
            "arch",
            query.arch,
            "hash_algo",
            query.hashAlgo,
            "compact",
            bool(query.compact),
            "include_file_urls",
            bool(query.includeFileUrls),
            "hw_rev",
            query.hwRev,
            "channel",
            query.channel);
    Map<String, String> headers = acceptJson();
    if (query.ifNoneMatch != null && !query.ifNoneMatch.isBlank()) {
      headers.put("If-None-Match", query.ifNoneMatch);
    }
    HttpResponse resp = send("GET", abs(path), headers, null, true);
    return jsonGet(resp, Integrity200.class);
  }

  public Diff200 diff(DiffRequest request) {
    Objects.requireNonNull(request, "request");
    if (request.sourceVersion == null || request.targetVersion == null || request.os == null || request.arch == null) {
      throw new IllegalArgumentException("source_version, target_version, os and arch are required");
    }
    HttpResponse resp =
        send("POST", abs(NativePaths.diff(config.projectRef())), jsonHeaders(), writeJson(request), true);
    requireOk(resp, 200);
    return readJson(resp, Diff200.class);
  }

  public PackOutcome pack(PackRequest request) {
    Objects.requireNonNull(request, "request");
    if (request.sourceVersion == null || request.targetVersion == null || request.os == null || request.arch == null) {
      throw new IllegalArgumentException("source_version, target_version, os and arch are required");
    }
    if (request.neededPaths == null) {
      request.neededPaths = List.of();
    }
    byte[] json = writeJson(request);
    HttpResponse resp = send("POST", abs(NativePaths.pack(config.projectRef())), jsonHeaders(), json, true);
    if (resp.status() != 200 && resp.status() != 202) {
      throw apiError(resp);
    }
    return new PackOutcome(resp.status(), readJson(resp, Pack200.class));
  }

  /**
   * POST pack; if HTTP 202 or {@code status=pending}, wait with bounded backoff and POST the
   * identical JSON until {@code ready}, {@code full_package}, error, or deadline.
   */
  public PackOutcome packUntilReady(PackRequest request) {
    Instant deadline = Instant.now().plus(config.packDeadline());
    Duration delay = config.packPollInitial();
    PackOutcome last = pack(request);
    while (last.pending()) {
      if (Instant.now().isAfter(deadline)) {
        throw new ApiException(408, "PACK_TIMEOUT", "pack polling deadline exceeded", null, null);
      }
      sleep(delay);
      last = pack(request);
      long nextMs = Math.min(delay.toMillis() * 2, config.packPollMax().toMillis());
      delay = Duration.ofMillis(Math.max(nextMs, 1));
    }
    return last;
  }

  public BinaryOutcome downloadPackage(String contentSha256, String exp, String sig, String range) {
    String path =
        NativePaths.withQuery(NativePaths.packages(config.projectRef(), contentSha256), "exp", exp, "sig", sig);
    return download("GET", abs(path), range, true);
  }

  public BinaryOutcome headPackage(String contentSha256, String exp, String sig) {
    String path =
        NativePaths.withQuery(NativePaths.packages(config.projectRef(), contentSha256), "exp", exp, "sig", sig);
    return download("HEAD", abs(path), null, true);
  }

  /** Download using a check/diff/pack {@code package_url}, keeping {@code exp}/{@code sig}. */
  public BinaryOutcome downloadUrl(String packageUrl, String range) {
    return download("GET", resolve(packageUrl), range, true);
  }

  public BinaryOutcome headUrl(String packageUrl) {
    return download("HEAD", resolve(packageUrl), null, true);
  }

  public ClientChannelList channels() {
    HttpResponse resp = send("GET", abs(NativePaths.channels(config.projectRef())), acceptJson(), null, true);
    requireOk(resp, 200);
    return readJson(resp, ClientChannelList.class);
  }

  public ClientMatrixList matrix() {
    HttpResponse resp = send("GET", abs(NativePaths.matrix(config.projectRef())), acceptJson(), null, true);
    requireOk(resp, 200);
    return readJson(resp, ClientMatrixList.class);
  }

  public ClientLanguageList languages() {
    HttpResponse resp = send("GET", abs(NativePaths.languages(config.projectRef())), acceptJson(), null, true);
    requireOk(resp, 200);
    return readJson(resp, ClientLanguageList.class);
  }

  public ResourceOutcome<AnnouncementList> announcements(AnnouncementsQuery query) {
    AnnouncementsQuery q = query == null ? new AnnouncementsQuery() : query;
    String path =
        NativePaths.withQuery(
            NativePaths.announcements(config.projectRef()),
            "version",
            q.version,
            "os",
            q.os,
            "arch",
            q.arch,
            "locale",
            q.locale);
    Map<String, String> headers = acceptJson();
    if (q.acceptLanguage != null && !q.acceptLanguage.isBlank()) {
      headers.put("Accept-Language", q.acceptLanguage);
    }
    if (q.ifNoneMatch != null && !q.ifNoneMatch.isBlank()) {
      headers.put("If-None-Match", q.ifNoneMatch);
    }
    HttpResponse resp = send("GET", abs(path), headers, null, true);
    return jsonGet(resp, AnnouncementList.class);
  }

  public TelemetryAccepted reportTelemetry(TelemetryRequest request) {
    Objects.requireNonNull(request, "request");
    if (request.os == null
        || request.arch == null
        || request.channel == null
        || request.fromVersion == null
        || request.toVersion == null
        || request.status == null) {
      throw new IllegalArgumentException("os, arch, channel, from_version, to_version and status are required");
    }
    HttpResponse resp =
        send("POST", abs(NativePaths.telemetry(config.projectRef())), jsonHeaders(), writeJson(request), true);
    requireOk(resp, 202);
    if (resp.body() == null || resp.body().length == 0) {
      TelemetryAccepted ok = new TelemetryAccepted();
      ok.status = "accepted";
      return ok;
    }
    return readJson(resp, TelemetryAccepted.class);
  }

  public BinaryOutcome downloadMedia(String mediaId, String range) {
    return download("GET", abs(NativePaths.media(config.projectRef(), mediaId)), range, true);
  }

  public BinaryOutcome headMedia(String mediaId) {
    return download("HEAD", abs(NativePaths.media(config.projectRef(), mediaId)), null, true);
  }

  private BinaryOutcome download(String method, String url, String range, boolean auth) {
    Map<String, String> headers = new LinkedHashMap<>();
    headers.put("Accept", "*/*");
    applyAuth(headers, auth);
    if (range != null && !range.isBlank()) {
      headers.put("Range", range);
    }
    HttpResponse resp = send(method, url, headers, null, auth);
    if (resp.status() != 200 && resp.status() != 206) {
      throw apiError(resp);
    }
    return new BinaryOutcome(resp.status(), resp.body(), resp);
  }

  private <T> ResourceOutcome<T> jsonGet(HttpResponse resp, Class<T> type) {
    if (resp.status() == 304) {
      return new ResourceOutcome<>(304, null, resp.header("ETag"), resp);
    }
    requireOk(resp, 200);
    return new ResourceOutcome<>(200, readJson(resp, type), resp.header("ETag"), resp);
  }

  private HttpResponse send(String method, String url, Map<String, String> headers, byte[] body, boolean auth) {
    Map<String, String> h = headers == null ? new LinkedHashMap<>() : new LinkedHashMap<>(headers);
    applyAuth(h, auth);
    if (config.channelToken() != null) {
      h.put("X-Channel-Token", config.channelToken());
    }
    try {
      return config.transport().execute(new HttpRequest(method, url, h, body));
    } catch (IOException e) {
      throw new ApiException(0, "TRANSPORT_ERROR", e.getMessage(), null, null, e);
    }
  }

  private void applyAuth(Map<String, String> headers, boolean auth) {
    if (!auth || config.projectToken() == null) {
      return;
    }
    headers.putIfAbsent("Authorization", "Bearer " + config.projectToken());
    headers.putIfAbsent("X-Project-Token", config.projectToken());
  }

  private Map<String, String> acceptJson() {
    Map<String, String> h = new LinkedHashMap<>();
    h.put("Accept", "application/json");
    return h;
  }

  private Map<String, String> jsonHeaders() {
    Map<String, String> h = acceptJson();
    h.put("Content-Type", "application/json; charset=utf-8");
    return h;
  }

  private void requireOk(HttpResponse resp, int expected) {
    if (resp.status() != expected) {
      throw apiError(resp);
    }
  }

  private ApiException apiError(HttpResponse resp) {
    Integer retryAfter = parseRetryAfter(resp.header("Retry-After"));
    String text = new String(resp.body() == null ? new byte[0] : resp.body(), StandardCharsets.UTF_8);
    try {
      ErrorEnvelope env = Json.MAPPER.readValue(resp.body(), ErrorEnvelope.class);
      if (ErrorEnvelope.looksLike(env)) {
        return new ApiException(resp.status(), env.code(), env.message(), env.details(), retryAfter);
      }
    } catch (Exception ignored) {
      // not the JSON envelope
    }
    String code = resp.status() == 429 ? "RATE_LIMITED" : "HTTP_" + resp.status();
    return new ApiException(resp.status(), code, text.isBlank() ? code : text, null, retryAfter);
  }

  private static Integer parseRetryAfter(String raw) {
    if (raw == null || raw.isBlank()) {
      return null;
    }
    try {
      return Integer.parseInt(raw.trim());
    } catch (NumberFormatException e) {
      return null;
    }
  }

  private <T> T readJson(HttpResponse resp, Class<T> type) {
    try {
      return Json.MAPPER.readValue(resp.body() == null ? "{}".getBytes(StandardCharsets.UTF_8) : resp.body(), type);
    } catch (IOException e) {
      throw new ApiException(resp.status(), "INVALID_RESPONSE", "unable to parse JSON", null, null);
    }
  }

  private byte[] writeJson(Object value) {
    try {
      return Json.MAPPER.writeValueAsBytes(value);
    } catch (IOException e) {
      throw new IllegalArgumentException("unable to encode JSON", e);
    }
  }

  private String abs(String pathAndQuery) {
    return resolve(pathAndQuery);
  }

  String resolve(String urlOrPath) {
    if (urlOrPath == null || urlOrPath.isBlank()) {
      throw new IllegalArgumentException("url is required");
    }
    String u = urlOrPath.trim();
    if (u.startsWith("http://") || u.startsWith("https://")) {
      return u;
    }
    return URI.create(config.baseUrl()).resolve(u).toString();
  }

  private static String bool(Boolean v) {
    return v == null ? null : Boolean.toString(v);
  }

  private static List<String> sanitizeAlgos(List<String> algos) {
    List<String> out = new ArrayList<>();
    for (String raw : algos) {
      if (raw != null && !raw.isBlank()) {
        out.add(raw.trim());
      }
    }
    return out.isEmpty() ? null : out;
  }

  private static UpdateCheckRequest copyCheck(UpdateCheckRequest src) {
    UpdateCheckRequest d = new UpdateCheckRequest();
    d.currentVersion = src.currentVersion;
    d.os = src.os;
    d.arch = src.arch;
    d.channel = src.channel;
    d.hwRev = src.hwRev;
    d.osVersion = src.osVersion;
    d.deviceId = src.deviceId;
    d.capabilities = src.capabilities == null ? null : new ArrayList<>(src.capabilities);
    d.acceptedDeltaAlgos = src.acceptedDeltaAlgos == null ? null : new ArrayList<>(src.acceptedDeltaAlgos);
    return d;
  }

  private static void sleep(Duration delay) {
    try {
      Thread.sleep(Math.max(delay.toMillis(), 1));
    } catch (InterruptedException e) {
      Thread.currentThread().interrupt();
      throw new ApiException(0, "INTERRUPTED", "pack poll interrupted", null, null);
    }
  }
}
