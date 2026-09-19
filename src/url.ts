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

/** True when `url` is the client-plane origin (tokens must not go to a public S3/CDN host). */
export function sameOrigin(baseUrl: string, url: string): boolean {
  try {
    const a = new URL(baseUrl);
    const b = new URL(url);
    return a.protocol === b.protocol && a.host === b.host;
  } catch {
    return false;
  }
}

export function headerGet(headers: Record<string, string>, name: string): string | undefined {
  const want = name.toLowerCase();
  if (Object.prototype.hasOwnProperty.call(headers, want) && headers[want] !== undefined) {
    return headers[want];
  }
  for (const [k, v] of Object.entries(headers)) {
    if (k.toLowerCase() === want) return v;
  }
  return undefined;
}

export function lowerHeaderRecord(headers: Record<string, string>): Record<string, string> {
  const out: Record<string, string> = {};
  for (const [k, v] of Object.entries(headers)) {
    out[k.toLowerCase()] = v;
  }
  return out;
}

export function headerMap(headers: Headers): Record<string, string> {
  const out: Record<string, string> = {};
  headers.forEach((value, key) => {
    out[key.toLowerCase()] = value;
  });
  return out;
}

export function etagOf(headers: Record<string, string>): string | undefined {
  return headerGet(headers, "etag");
}
