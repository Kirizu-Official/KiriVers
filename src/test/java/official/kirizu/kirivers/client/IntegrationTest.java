package official.kirizu.kirivers.client;

import com.fasterxml.jackson.databind.JsonNode;
import official.kirizu.kirivers.client.model.UpdateCheckRequest;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.io.TempDir;
import org.opentest4j.TestAbortedException;

import java.nio.file.Files;
import java.nio.file.Path;
import java.util.Locale;
import java.util.UUID;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertTrue;

/**
 * Live client-plane integration. Does not start or stop Docker / KiriVers.
 * Fixture: {@code D:\\KiriVers\\configs\\sdk-fixture.json}.
 */
class IntegrationTest {

  private static final Path FIXTURE = Path.of("D:\\KiriVers\\configs\\sdk-fixture.json");
  private static final String TARGET_SHA = "7f063a6901ebfd0aa7b1adb8af58f4e792f13310f42213767ced623eb0669969";

  @Test
  void checkDownloadAndSha256(@TempDir Path stage) throws Exception {
    if (!Files.isRegularFile(FIXTURE)) {
      writeBackendIssue("sdk-fixture.json is missing", "Read D:\\KiriVers\\configs\\sdk-fixture.json", "file not found");
      throw new TestAbortedException("sdk-fixture.json missing at " + FIXTURE);
    }
    JsonNode fx = Json.MAPPER.readTree(Files.readAllBytes(FIXTURE));
    String base = fx.path("client_base_url").asText("http://127.0.0.1:8080");
    String project = fx.path("project_ref").asText("sdk-fixture");
    String os = fx.path("os").asText("windows");
    String arch = fx.path("arch").asText("x86_64");
    String channel = fx.path("channel").asText("stable");
    String current = fx.path("current_version").asText("1.0.0");
    String expectVer = fx.path("target_version").asText("1.1.0");
    String expectSha = fx.path("sha256").path("1.1.0").asText(TARGET_SHA);

    Client client = new Client(Config.builder().baseUrl(base).projectRef(project).build());
    try {
      client.health();
    } catch (ApiException e) {
      writeBackendIssue("client plane is not reachable", "GET " + base + "/api/v1/health → 200", e.getMessage());
      throw new TestAbortedException("client plane not reachable: " + e.code() + " " + e.getMessage());
    }

    UpdateCheckRequest req = new UpdateCheckRequest();
    req.currentVersion = current;
    req.os = os;
    req.arch = arch;
    req.channel = channel;
    req.deviceId = "sdk-java-" + UUID.randomUUID();
    CheckOutcome check;
    try {
      check = client.check(req);
    } catch (ApiException e) {
      writeBackendIssue("POST /update/check failed", "200 with version 1.1.0", e.code() + " " + e.getMessage());
      throw new TestAbortedException("live check failed: " + e.code() + " " + e.getMessage());
    }
    if (check.status() != CheckOutcome.Status.UPDATE || check.body() == null) {
      writeBackendIssue(
          "check from 1.0.0 did not return an update",
          "200 version_semver=" + expectVer + " sha256=" + expectSha,
          String.valueOf(check.status()));
      throw new TestAbortedException("expected update, got " + check.status());
    }
    assertEquals(expectVer, check.body().versionSemver);
    assertEquals(expectSha.toLowerCase(Locale.ROOT), check.body().sha256.toLowerCase(Locale.ROOT));
    assertTrue(Boolean.TRUE.equals(check.body().hasUpdate));

    BinaryOutcome bin;
    try {
      bin = client.downloadUrl(check.body().packageUrl, null);
    } catch (ApiException e) {
      writeBackendIssue("package download failed", "GET package_url → 200 bytes", e.code() + " " + e.getMessage());
      throw new TestAbortedException("package download failed: " + e.code() + " " + e.getMessage());
    }
    String got = client.config().hasher().sha256(bin.body());
    if (!expectSha.equalsIgnoreCase(got)) {
      writeBackendIssue(
          "downloaded package hash mismatch",
          "sha256 " + expectSha,
          got);
      throw new AssertionError("sha256 mismatch: expected " + expectSha + " got " + got);
    }
    assertEquals(expectSha.toLowerCase(Locale.ROOT), got);

    UpdateRequest upd = new UpdateRequest();
    upd.currentVersion = current;
    upd.os = os;
    upd.arch = arch;
    upd.channel = channel;
    upd.deviceId = "sdk-java-" + UUID.randomUUID();
    upd.stageDir = stage;
    UpdateResult result = new Updater(client).update(upd);
    assertEquals(UpdateResult.Status.STAGED, result.status());
    assertEquals(expectSha.toLowerCase(Locale.ROOT), result.sha256().toLowerCase(Locale.ROOT));
    assertTrue(Files.isRegularFile(result.stagedPath()));
  }

  private static void writeBackendIssue(String repro, String expected, String actual) throws Exception {
    String md =
        """
        # Backend issue (Java SDK integration)

        Live client plane: `http://127.0.0.1:8080`
        Fixture: `D:\\\\KiriVers\\\\configs\\\\sdk-fixture.json`
        The client plane is up (`GET /api/v1/health` → 200). This worktree did **not** edit server code.

        ## Repro
        %s

        ## Expected
        %s

        ## Actual
        %s

        ## Suggested fix
        Recreate or restore the `sdk-fixture` project on the live client plane so `1.0.0` → `1.1.0`
        for `windows/x86_64` is published again. Package SHA-256
        `7f063a6901ebfd0aa7b1adb8af58f4e792f13310f42213767ced623eb0669969`.
        """
            .formatted(repro, expected, actual);
    Files.writeString(Path.of("BACKEND_ISSUE.md"), md);
  }
}
