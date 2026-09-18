/**
 * API error with the client-plane envelope `{ error: { code, message, details } }`.
 * Unknown codes are kept as opaque strings.
 */
export class ApiError extends Error {
  readonly code: string;
  readonly details: unknown;
  readonly status: number;
  readonly headers: Record<string, string>;
  readonly retryAfter?: number;

  constructor(
    code: string,
    message: string,
    details: unknown,
    status: number,
    headers: Record<string, string> = {},
  ) {
    super(message || code);
    this.name = "ApiError";
    this.code = code;
    this.details = details;
    this.status = status;
    this.headers = headers;
    const retry = headers["retry-after"];
    if (retry !== undefined) {
      const n = Number(retry);
      if (Number.isFinite(n)) this.retryAfter = n;
    }
  }
}

export class DeltaMagicError extends Error {
  constructor(message = "unknown or mismatched delta magic") {
    super(message);
    this.name = "DeltaMagicError";
  }
}

export class HashMismatchError extends Error {
  constructor(
    readonly expected: string,
    readonly actual: string,
  ) {
    super(`sha256 mismatch: expected ${expected}, got ${actual}`);
    this.name = "HashMismatchError";
  }
}

export class PathError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "PathError";
  }
}

export class ReplaceBusyError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "ReplaceBusyError";
  }
}

export function errorFromBody(
  status: number,
  body: Uint8Array,
  headers: Record<string, string>,
): ApiError {
  const text = new TextDecoder().decode(body);
  try {
    const parsed = JSON.parse(text) as {
      error?: { code?: unknown; message?: unknown; details?: unknown };
    };
    const err = parsed?.error;
    if (err && typeof err.code === "string") {
      return new ApiError(
        err.code,
        typeof err.message === "string" ? err.message : "",
        err.details,
        status,
        headers,
      );
    }
  } catch {
    // fall through
  }
  return new ApiError("UNKNOWN", text || `HTTP ${status}`, undefined, status, headers);
}
