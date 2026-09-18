export interface TransportRequest {
  method: string;
  url: string;
  headers: Record<string, string>;
  body?: Uint8Array | null;
  signal?: AbortSignal;
}

export interface TransportResponse {
  status: number;
  headers: Record<string, string>;
  body: Uint8Array;
}

/** HTTP GET/HEAD/POST with Range and preserved query strings. */
export interface Transport {
  request(req: TransportRequest): Promise<TransportResponse>;
}

function toHeaderRecord(headers: Headers): Record<string, string> {
  const out: Record<string, string> = {};
  headers.forEach((value, key) => {
    out[key.toLowerCase()] = value;
  });
  return out;
}

/** Node 20+ / Electron app-level `globalThis.fetch`. Do not use axios or node-fetch. */
export class FetchTransport implements Transport {
  constructor(private readonly fetchImpl: typeof fetch = globalThis.fetch.bind(globalThis)) {}

  async request(req: TransportRequest): Promise<TransportResponse> {
    const headers = new Headers(req.headers);
    const init: RequestInit = {
      method: req.method,
      headers,
      signal: req.signal,
    };
    if (req.body && req.method !== "GET" && req.method !== "HEAD") {
      init.body = req.body;
    }
    const res = await this.fetchImpl(req.url, init);
    const body = new Uint8Array(await res.arrayBuffer());
    return { status: res.status, headers: toHeaderRecord(res.headers), body };
  }
}
