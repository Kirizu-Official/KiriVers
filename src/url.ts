/** Resolve a possibly relative `package_url` against the client-plane base URL. Query (`exp`/`sig`) is kept. */
export function resolveUrl(baseUrl: string, pathOrUrl: string): string {
  const base = baseUrl.replace(/\/+$/, "");
  if (/^https?:\/\//i.test(pathOrUrl)) {
    return pathOrUrl;
  }
  return new URL(pathOrUrl, `${base}/`).toString();
}

export function withQuery(url: string, query: Record<string, string | boolean | number | undefined>): string {
  const u = new URL(url);
  for (const [k, v] of Object.entries(query)) {
    if (v === undefined) continue;
    u.searchParams.set(k, String(v));
  }
  return u.toString();
}

export function headerMap(headers: Headers): Record<string, string> {
  const out: Record<string, string> = {};
  headers.forEach((value, key) => {
    out[key.toLowerCase()] = value;
  });
  return out;
}

export function etagOf(headers: Record<string, string>): string | undefined {
  return headers.etag;
}
