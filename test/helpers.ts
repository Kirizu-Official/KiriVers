import type { Transport, TransportRequest, TransportResponse } from "../src/transport.js";

export class MockTransport implements Transport {
  readonly requests: TransportRequest[] = [];
  constructor(private readonly handler: (req: TransportRequest) => TransportResponse | Promise<TransportResponse>) {}

  async request(req: TransportRequest): Promise<TransportResponse> {
    this.requests.push(req);
    return this.handler(req);
  }
}

export function jsonResponse(
  status: number,
  body: unknown,
  headers: Record<string, string> = {},
): TransportResponse {
  const payload = body === "" || body === undefined ? new Uint8Array() : new TextEncoder().encode(JSON.stringify(body));
  return {
    status,
    headers: { "content-type": "application/json", ...headers },
    body: payload,
  };
}

export function bytesResponse(status: number, body: Uint8Array, headers: Record<string, string> = {}): TransportResponse {
  return { status, headers, body };
}

export function lastJsonBody(t: MockTransport): unknown {
  const req = t.requests[t.requests.length - 1];
  if (!req?.body) return undefined;
  return JSON.parse(new TextDecoder().decode(req.body));
}

export function pathnameOf(req: TransportRequest): string {
  return new URL(req.url).pathname;
}
