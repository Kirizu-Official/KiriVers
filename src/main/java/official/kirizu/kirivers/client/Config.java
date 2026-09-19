package official.kirizu.kirivers.client;

import official.kirizu.kirivers.client.adapter.ArchiveUnpacker;
import official.kirizu.kirivers.client.adapter.FileStore;
import official.kirizu.kirivers.client.adapter.Hasher;
import official.kirizu.kirivers.client.adapter.JdkHttpTransport;
import official.kirizu.kirivers.client.adapter.JdkSignatureVerifier;
import official.kirizu.kirivers.client.adapter.MessageDigestHasher;
import official.kirizu.kirivers.client.adapter.NioFileStore;
import official.kirizu.kirivers.client.adapter.Patcher;
import official.kirizu.kirivers.client.adapter.Replacer;
import official.kirizu.kirivers.client.adapter.SignatureVerifier;
import official.kirizu.kirivers.client.adapter.SigningKey;
import official.kirizu.kirivers.client.adapter.Transport;
import official.kirizu.kirivers.client.adapter.ZipArchiveUnpacker;

import java.time.Duration;
import java.util.ArrayList;
import java.util.List;
import java.util.Objects;

/**
 * Caller-supplied SDK configuration. {@code device_id}, channel, os/arch and
 * current version are per-call — this object never invents a device identity.
 */
public final class Config {

  private final String baseUrl;
  private final String projectRef;
  private final String projectToken;
  private final String channelToken;
  private final Transport transport;
  private final FileStore fileStore;
  private final Hasher hasher;
  private final SignatureVerifier signatureVerifier;
  private final ArchiveUnpacker archiveUnpacker;
  private final Patcher patcher;
  private final Replacer replacer;
  private final List<SigningKey> signingKeys;
  private final Duration packPollInitial;
  private final Duration packPollMax;
  private final Duration packDeadline;

  private Config(Builder b) {
    this.baseUrl = trimSlash(Objects.requireNonNull(b.baseUrl, "baseUrl"));
    this.projectRef = Objects.requireNonNull(b.projectRef, "projectRef");
    this.projectToken = emptyToNull(b.projectToken);
    this.channelToken = emptyToNull(b.channelToken);
    this.transport = b.transport == null ? new JdkHttpTransport() : b.transport;
    this.hasher = b.hasher == null ? new MessageDigestHasher() : b.hasher;
    this.fileStore = b.fileStoreDisabled ? null : (b.fileStore == null ? new NioFileStore(this.hasher) : b.fileStore);
    this.signatureVerifier = b.signatureVerifier == null ? new JdkSignatureVerifier() : b.signatureVerifier;
    this.archiveUnpacker = b.archiveUnpackerDisabled ? null : (b.archiveUnpacker == null ? new ZipArchiveUnpacker() : b.archiveUnpacker);
    this.patcher = b.patcher;
    this.replacer = b.replacer;
    this.signingKeys = List.copyOf(b.signingKeys);
    this.packPollInitial = b.packPollInitial == null ? Duration.ofSeconds(1) : b.packPollInitial;
    this.packPollMax = b.packPollMax == null ? Duration.ofSeconds(15) : b.packPollMax;
    this.packDeadline = b.packDeadline == null ? Duration.ofMinutes(2) : b.packDeadline;
  }

  public static Builder builder() {
    return new Builder();
  }

  public String baseUrl() {
    return baseUrl;
  }

  public String projectRef() {
    return projectRef;
  }

  public String projectToken() {
    return projectToken;
  }

  public String channelToken() {
    return channelToken;
  }

  public Transport transport() {
    return transport;
  }

  public FileStore fileStore() {
    return fileStore;
  }

  public Hasher hasher() {
    return hasher;
  }

  public SignatureVerifier signatureVerifier() {
    return signatureVerifier;
  }

  public ArchiveUnpacker archiveUnpacker() {
    return archiveUnpacker;
  }

  public Patcher patcher() {
    return patcher;
  }

  public Replacer replacer() {
    return replacer;
  }

  public List<SigningKey> signingKeys() {
    return signingKeys;
  }

  public Duration packPollInitial() {
    return packPollInitial;
  }

  public Duration packPollMax() {
    return packPollMax;
  }

  public Duration packDeadline() {
    return packDeadline;
  }

  /** D13: never advertise a capability the live adapters cannot honour. */
  public List<String> capabilities() {
    List<String> caps = new ArrayList<>();
    caps.add("full_package");
    if (archiveUnpacker != null) {
      caps.add("patch_package");
    }
    if (fileStore != null && fileStore.canWriteIndividualFiles()) {
      caps.add("file_list");
    }
    if (!acceptedDeltaAlgos().isEmpty()) {
      caps.add("binary_delta");
    }
    return List.copyOf(caps);
  }

  public List<String> acceptedDeltaAlgos() {
    if (patcher == null || patcher.supportedAlgos() == null) {
      return List.of();
    }
    List<String> out = new ArrayList<>();
    for (String raw : patcher.supportedAlgos()) {
      if (raw != null && !raw.isBlank()) {
        out.add(raw.trim());
      }
    }
    return List.copyOf(out);
  }

  private static String trimSlash(String url) {
    String u = url.trim();
    while (u.endsWith("/")) {
      u = u.substring(0, u.length() - 1);
    }
    return u;
  }

  private static String emptyToNull(String s) {
    return s == null || s.isBlank() ? null : s;
  }

  public static final class Builder {
    private String baseUrl;
    private String projectRef;
    private String projectToken;
    private String channelToken;
    private Transport transport;
    private FileStore fileStore;
    private boolean fileStoreDisabled;
    private Hasher hasher;
    private SignatureVerifier signatureVerifier;
    private ArchiveUnpacker archiveUnpacker;
    private boolean archiveUnpackerDisabled;
    private Patcher patcher;
    private Replacer replacer;
    private final List<SigningKey> signingKeys = new ArrayList<>();
    private Duration packPollInitial;
    private Duration packPollMax;
    private Duration packDeadline;

    public Builder baseUrl(String baseUrl) {
      this.baseUrl = baseUrl;
      return this;
    }

    public Builder projectRef(String projectRef) {
      this.projectRef = projectRef;
      return this;
    }

    public Builder projectToken(String projectToken) {
      this.projectToken = projectToken;
      return this;
    }

    public Builder channelToken(String channelToken) {
      this.channelToken = channelToken;
      return this;
    }

    public Builder transport(Transport transport) {
      this.transport = transport;
      return this;
    }

    public Builder fileStore(FileStore fileStore) {
      this.fileStore = fileStore;
      this.fileStoreDisabled = fileStore == null;
      return this;
    }

    public Builder hasher(Hasher hasher) {
      this.hasher = hasher;
      return this;
    }

    public Builder signatureVerifier(SignatureVerifier signatureVerifier) {
      this.signatureVerifier = signatureVerifier;
      return this;
    }

    public Builder archiveUnpacker(ArchiveUnpacker archiveUnpacker) {
      this.archiveUnpacker = archiveUnpacker;
      this.archiveUnpackerDisabled = archiveUnpacker == null;
      return this;
    }

    public Builder patcher(Patcher patcher) {
      this.patcher = patcher;
      return this;
    }

    public Builder replacer(Replacer replacer) {
      this.replacer = replacer;
      return this;
    }

    public Builder addSigningKey(SigningKey key) {
      if (key != null) {
        this.signingKeys.add(key);
      }
      return this;
    }

    public Builder packPollInitial(Duration d) {
      this.packPollInitial = d;
      return this;
    }

    public Builder packPollMax(Duration d) {
      this.packPollMax = d;
      return this;
    }

    public Builder packDeadline(Duration d) {
      this.packDeadline = d;
      return this;
    }

    public Config build() {
      return new Config(this);
    }
  }
}
