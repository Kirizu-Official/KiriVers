/**
 * Same payload as `pkg/signature.BuildCheckPayload`:
 * version_integer \\n version_semver \\n root_hash \\n package_url \\n size \\n sha256
 * Missing fields are empty strings so positions stay distinguishable.
 */
export function buildCheckPayload(
  versionInteger: string,
  versionSemver: string,
  rootHash: string,
  packageUrl: string,
  size: string,
  sha256Hex: string,
): string {
  return [versionInteger, versionSemver, rootHash, packageUrl, size, sha256Hex].join("\n");
}

export function payloadFromCheckLike(fields: {
  version_integer?: number | null;
  version_semver?: string | null;
  root_hash?: string;
  package_url?: string;
  size?: number;
  sha256?: string;
}): string {
  return buildCheckPayload(
    fields.version_integer == null ? "" : String(fields.version_integer),
    fields.version_semver ?? "",
    fields.root_hash ?? "",
    fields.package_url ?? "",
    fields.size == null ? "" : String(fields.size),
    fields.sha256 ?? "",
  );
}
