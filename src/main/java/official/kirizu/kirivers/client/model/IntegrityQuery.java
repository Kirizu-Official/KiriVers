package official.kirizu.kirivers.client.model;

/** Optional query fields for GET integrity. {@code os} and {@code arch} are required. */
public final class IntegrityQuery {
  public String os;
  public String arch;
  public String hashAlgo;
  public Boolean compact;
  public Boolean includeFileUrls;
  public String hwRev;
  public String channel;
  public String ifNoneMatch;
}
