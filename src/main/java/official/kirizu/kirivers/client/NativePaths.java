package official.kirizu.kirivers.client;

import java.net.URLEncoder;
import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.List;

/** Native JSON paths from {@code openapi.client.json}. Store feeds and leftover routes are not represented. */
public final class NativePaths {

  public static final String HEALTH = "/api/v1/health";

  private NativePaths() {}

  public static String project(String projectRef) {
    return "/api/v1/projects/" + enc(projectRef);
  }

  public static String deviceReport(String projectRef) {
    return project(projectRef) + "/clients/report";
  }

  public static String check(String projectRef) {
    return project(projectRef) + "/update/check";
  }

  public static String changelog(String projectRef, String channel, String os, String arch) {
    return project(projectRef) + "/changelog/" + enc(channel) + "/" + enc(os) + "/" + enc(arch);
  }

  public static String integrity(String projectRef, String version) {
    return project(projectRef) + "/versions/" + enc(version) + "/integrity";
  }

  public static String diff(String projectRef) {
    return project(projectRef) + "/update/diff";
  }

  public static String pack(String projectRef) {
    return project(projectRef) + "/update/pack";
  }

  public static String packages(String projectRef, String ref) {
    return project(projectRef) + "/packages/" + enc(ref);
  }

  public static String telemetry(String projectRef) {
    return project(projectRef) + "/telemetry/report";
  }

  public static String channels(String projectRef) {
    return project(projectRef) + "/channels";
  }

  public static String matrix(String projectRef) {
    return project(projectRef) + "/matrix";
  }

  public static String languages(String projectRef) {
    return project(projectRef) + "/languages";
  }

  public static String announcements(String projectRef) {
    return project(projectRef) + "/announcements";
  }

  public static String media(String projectRef, String id) {
    return project(projectRef) + "/media/" + enc(id);
  }

  public static String withQuery(String path, String... keyValues) {
    if (keyValues == null || keyValues.length == 0) {
      return path;
    }
    if (keyValues.length % 2 != 0) {
      throw new IllegalArgumentException("query pairs required");
    }
    List<String> parts = new ArrayList<>();
    for (int i = 0; i < keyValues.length; i += 2) {
      String k = keyValues[i];
      String v = keyValues[i + 1];
      if (k == null || v == null || v.isEmpty()) {
        continue;
      }
      parts.add(enc(k) + "=" + enc(v));
    }
    if (parts.isEmpty()) {
      return path;
    }
    return path + "?" + String.join("&", parts);
  }

  public static String enc(String s) {
    return URLEncoder.encode(s, StandardCharsets.UTF_8).replace("+", "%20");
  }
}
