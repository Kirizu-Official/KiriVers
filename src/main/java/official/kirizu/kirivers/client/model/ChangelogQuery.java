package official.kirizu.kirivers.client.model;

/** Optional query/header fields for GET changelog. */
public final class ChangelogQuery {
  public String fromVersion;
  public String toVersion;
  public String changelogScope;
  public String changelogLayout;
  public Boolean changelogIncludeRevoked;
  public Boolean changelogIncludePlatformNotes;
  public String changelogLocale;
  public String locale;
  public String ifNoneMatch;
}
