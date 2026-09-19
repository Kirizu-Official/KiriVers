import { errorFromBody } from "./errors.js";
import { expandPath, PATHS } from "./paths.js";
import { FetchTransport, type Transport, type TransportResponse } from "./transport.js";
import type {
  AnnouncementQuery,
  BinaryResult,
  ChangelogBody,
  ChangelogQuery,
  CheckResult,
  ClientAnnouncement,
  ClientChannel,
  ClientLanguage,
  ClientMatrixRow,
  DeviceReportInput,
  DeviceReportOutput,
  Diff200,
  DiffRequest,
  DownloadOptions,
  HealthStatus,
  Integrity200,
  IntegrityQuery,
  JsonResult,
  Pack200,
  PackRequest,
  ProjectPublic,
  TelemetryReport,
  UpdateCheck200,
  UpdateCheckRequest,
} from "./types.js";
import { CAPABILITY_FULL_PACKAGE } from "./types.js";
import { etagOf, lowerHeaderRecord, resolveUrl, sameOrigin, withQuery } from "./url.js";

export interface PackPollOptions {
  initialDelayMs?: number;
  maxDelayMs?: number;
  deadlineMs?: number;
  sleep?: (ms: number) => Promise<void>;
}

export interface ClientOptions {
  /** Client-plane base URL with no trailing slash, e.g. `http://127.0.0.1:8080`. */
  baseUrl: string;
  projectRef: string;
  projectToken?: string;
  /** Never logged. */
  channelToken?: string;
  transport?: Transport;
}

function omitUndefined<T extends Record<string, unknown>>(obj: T): T {
  const out: Record<string, unknown> = {};
  for (const [k, v] of Object.entries(obj)) {
    if (v !== undefined) out[k] = v;
  }
  return out as T;
}

function defaultSleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

/**
 * Handwritten native JSON client. Callers supply `device_id` / channel / os / arch / version.
 * The SDK never invents a device identity and never writes plaintext `device_id` to logs.
 */
export class Client {
  readonly baseUrl: string;
  readonly projectRef: string;
  readonly transport: Transport;
  private readonly projectToken?: string;
  private readonly channelToken?: string;

  constructor(opts: ClientOptions) {
    if (!opts.baseUrl) throw new Error("baseUrl is required");
    if (!opts.projectRef) throw new Error("projectRef is required");
    this.baseUrl = opts.baseUrl.replace(/\/+$/, "");
    this.projectRef = opts.projectRef;
    this.projectToken = opts.projectToken;
    this.channelToken = opts.channelToken;
    this.transport = opts.transport ?? new FetchTransport();
  }

  async health(): Promise<HealthStatus> {
    const res = await this.send("GET", PATHS.health, { expected: [200] });
    return this.parseJson<HealthStatus>(res);
  }

  async project(): Promise<ProjectPublic> {
    const res = await this.send("GET", this.p(PATHS.project), { expected: [200] });
    return this.parseJson<ProjectPublic>(res);
  }

  async deviceReport(input: DeviceReportInput): Promise<DeviceReportOutput> {
    const res = await this.send("POST", this.p(PATHS.deviceReport), {
      json: omitUndefined({ ...input }),
      expected: [200],
    });
    return this.parseJson<DeviceReportOutput>(res);
  }

  async check(req: UpdateCheckRequest, opts?: { ifNoneMatch?: string }): Promise<CheckResult> {
    const body: Record<string, unknown> = {
      current_version: req.current_version,
      os: req.os,
      arch: req.arch,
    };
    if (req.channel !== undefined) body.channel = req.channel;
    if (req.hw_rev !== undefined) body.hw_rev = req.hw_rev;
    if (req.os_version !== undefined) body.os_version = req.os_version;
    if (req.device_id !== undefined) body.device_id = req.device_id;
    body.capabilities =
      req.capabilities && req.capabilities.length > 0
        ? req.capabilities
        : [CAPABILITY_FULL_PACKAGE];
    if (req.accepted_delta_algos && req.accepted_delta_algos.length > 0) {
      body.accepted_delta_algos = req.accepted_delta_algos;
    }
    const headers: Record<string, string> = {};
    if (opts?.ifNoneMatch) headers["If-None-Match"] = opts.ifNoneMatch;
    const res = await this.send("POST", this.p(PATHS.check), {
      json: body,
      headers,
      expected: [200, 204, 304],
    });
    const etag = etagOf(res.headers);
    if (res.status === 304) return { kind: "not_modified", status: 304, etag, headers: res.headers };
    if (res.status === 204) return { kind: "no_update", status: 204, etag, headers: res.headers };
    return {
      kind: "update",
      status: 200,
      body: this.parseJson<UpdateCheck200>(res),
      etag,
      headers: res.headers,
    };
  }

  async changelog(
    channel: string,
    os: string,
    arch: string,
    query: ChangelogQuery = {},
  ): Promise<JsonResult<ChangelogBody> | { status: 304; etag?: string; headers: Record<string, string> }> {
    const path = this.p(PATHS.changelog, { channel, os, arch });
    const url = withQuery(resolveUrl(this.baseUrl, path), {
      from_version: query.from_version,
      to_version: query.to_version,
      changelog_scope: query.changelog_scope,
      changelog_layout: query.changelog_layout,
      changelog_include_revoked: query.changelog_include_revoked,
      changelog_include_platform_notes: query.changelog_include_platform_notes,
      changelog_locale: query.changelog_locale,
      locale: query.locale,
    });
    const headers: Record<string, string> = {};
    if (query.ifNoneMatch) headers["If-None-Match"] = query.ifNoneMatch;
    const res = await this.sendUrl("GET", url, { headers, expected: [200, 304] });
    if (res.status === 304) {
      return { status: 304, etag: etagOf(res.headers), headers: res.headers };
    }
    return {
      status: 200,
      body: this.parseJson<ChangelogBody>(res),
      etag: etagOf(res.headers),
      headers: res.headers,
    };
  }

  async integrity(
    version: string,
    query: IntegrityQuery,
  ): Promise<JsonResult<Integrity200> | { status: 304; etag?: string; headers: Record<string, string> }> {
    const path = this.p(PATHS.integrity, { version });
    const url = withQuery(resolveUrl(this.baseUrl, path), {
      os: query.os,
      arch: query.arch,
      hash_algo: query.hash_algo,
      compact: query.compact,
      include_file_urls: query.include_file_urls,
      hw_rev: query.hw_rev,
      channel: query.channel,
    });
    const headers: Record<string, string> = {};
    if (query.ifNoneMatch) headers["If-None-Match"] = query.ifNoneMatch;
    const res = await this.sendUrl("GET", url, { headers, expected: [200, 304] });
    if (res.status === 304) {
      return { status: 304, etag: etagOf(res.headers), headers: res.headers };
    }
    return {
      status: 200,
      body: this.parseJson<Integrity200>(res),
      etag: etagOf(res.headers),
      headers: res.headers,
    };
  }

  async diff(req: DiffRequest): Promise<Diff200> {
    const json = omitUndefined({ ...req });
    const res = await this.send("POST", this.p(PATHS.diff), { json, expected: [200] });
    return this.parseJson<Diff200>(res);
  }

  async pack(req: PackRequest): Promise<{ status: number; body: Pack200; headers: Record<string, string> }> {
    const json = omitUndefined({ ...req });
    const res = await this.send("POST", this.p(PATHS.pack), { json, expected: [200, 202] });
    return { status: res.status, body: this.parseJson<Pack200>(res), headers: res.headers };
  }

  /**
   * POST pack until `ready` / `full_package` / error. Pending (HTTP 202 or `status=pending`)
   * repeats the **identical** JSON. Never calls admin jobs.
   */
  async packUntilReady(req: PackRequest, poll: PackPollOptions = {}): Promise<Pack200> {
    const json = omitUndefined({ ...req });
    const initial = poll.initialDelayMs ?? 1000;
    const maxDelay = poll.maxDelayMs ?? 15_000;
    const deadline = Date.now() + (poll.deadlineMs ?? 5 * 60_000);
    const sleep = poll.sleep ?? defaultSleep;
    let delay = initial;
    for (;;) {
      const res = await this.send("POST", this.p(PATHS.pack), { json, expected: [200, 202] });
      const body = this.parseJson<Pack200>(res);
      const pending = res.status === 202 || body.status === "pending";
      if (!pending) return body;
      if (Date.now() >= deadline) {
        throw new Error("pack poll deadline exceeded");
      }
      await sleep(delay);
      delay = Math.min(maxDelay, delay * 2);
    }
  }

  async downloadPackage(ref: string, opts: DownloadOptions = {}): Promise<BinaryResult> {
    const path = this.p(PATHS.packages, { ref });
    const url = withQuery(resolveUrl(this.baseUrl, path), {
      exp: opts.exp,
      sig: opts.sig,
      hw_rev: opts.hw_rev,
    });
    return this.downloadUrl(url, { range: opts.range });
  }

  async headPackage(ref: string, opts: DownloadOptions = {}): Promise<BinaryResult> {
    const path = this.p(PATHS.packages, { ref });
    const url = withQuery(resolveUrl(this.baseUrl, path), {
      exp: opts.exp,
      sig: opts.sig,
      hw_rev: opts.hw_rev,
    });
    return this.headUrl(url);
  }

  /** Download a check/diff/pack `package_url`, keeping `exp`/`sig` query parameters. */
  async downloadUrl(packageUrl: string, opts?: { range?: string }): Promise<BinaryResult> {
    const url = resolveUrl(this.baseUrl, packageUrl);
    const headers: Record<string, string> = {};
    if (opts?.range) headers.Range = opts.range;
    const res = await this.sendUrl("GET", url, { headers, expected: [200, 206], binary: true });
    return { status: res.status, body: res.body, headers: res.headers };
  }

  async headUrl(packageUrl: string): Promise<BinaryResult> {
    const url = resolveUrl(this.baseUrl, packageUrl);
    const res = await this.sendUrl("HEAD", url, { expected: [200, 206], binary: true });
    return { status: res.status, body: res.body, headers: res.headers };
  }

  async channels(): Promise<{ channels: ClientChannel[] }> {
    const res = await this.send("GET", this.p(PATHS.channels), { expected: [200] });
    return this.parseJson(res);
  }

  async matrix(): Promise<{ matrix: ClientMatrixRow[] }> {
    const res = await this.send("GET", this.p(PATHS.matrix), { expected: [200] });
    return this.parseJson(res);
  }

  async languages(): Promise<{ languages: ClientLanguage[] }> {
    const res = await this.send("GET", this.p(PATHS.languages), { expected: [200] });
    return this.parseJson(res);
  }

  async announcements(
    query: AnnouncementQuery = {},
  ): Promise<
    JsonResult<{ announcements: ClientAnnouncement[] }> | { status: 304; etag?: string; headers: Record<string, string> }
  > {
    const url = withQuery(resolveUrl(this.baseUrl, this.p(PATHS.announcements)), {
      version: query.version,
      os: query.os,
      arch: query.arch,
      locale: query.locale,
    });
    const headers: Record<string, string> = {};
    if (query.ifNoneMatch) headers["If-None-Match"] = query.ifNoneMatch;
    if (query.acceptLanguage) headers["Accept-Language"] = query.acceptLanguage;
    const res = await this.sendUrl("GET", url, { headers, expected: [200, 304] });
    if (res.status === 304) {
      return { status: 304, etag: etagOf(res.headers), headers: res.headers };
    }
    return {
      status: 200,
      body: this.parseJson(res),
      etag: etagOf(res.headers),
      headers: res.headers,
    };
  }

  /**
   * Telemetry is accepted as HTTP 202. Callers/Updater must not treat a telemetry
   * failure as a failed apply; this method still surfaces API errors to the caller.
   */
  async reportTelemetry(body: TelemetryReport): Promise<{ status: string } | undefined> {
    const json = omitUndefined({ ...body });
    const res = await this.send("POST", this.p(PATHS.telemetry), { json, expected: [202] });
    if (res.body.length === 0) return undefined;
    return this.parseJson(res);
  }

  async getMedia(id: string, opts?: { range?: string }): Promise<BinaryResult> {
    const url = resolveUrl(this.baseUrl, this.p(PATHS.media, { id }));
    const headers: Record<string, string> = {};
    if (opts?.range) headers.Range = opts.range;
    const res = await this.sendUrl("GET", url, { headers, expected: [200, 206], binary: true });
    return { status: res.status, body: res.body, headers: res.headers };
  }

  async headMedia(id: string): Promise<BinaryResult> {
    const url = resolveUrl(this.baseUrl, this.p(PATHS.media, { id }));
    const res = await this.sendUrl("HEAD", url, { expected: [200], binary: true });
    return { status: res.status, body: res.body, headers: res.headers };
  }

  private p(template: string, extra: Record<string, string> = {}): string {
    return expandPath(template, { project_ref: this.projectRef, ...extra });
  }

  private authHeaders(): Record<string, string> {
    const headers: Record<string, string> = {};
    if (this.projectToken) {
      headers.Authorization = `Bearer ${this.projectToken}`;
      headers["X-Project-Token"] = this.projectToken;
    }
    if (this.channelToken) {
      headers["X-Channel-Token"] = this.channelToken;
    }
    return headers;
  }

  private async send(
    method: string,
    path: string,
    opts: {
      json?: unknown;
      headers?: Record<string, string>;
      expected: number[];
      binary?: boolean;
    },
  ): Promise<TransportResponse> {
    return this.sendUrl(method, resolveUrl(this.baseUrl, path), opts);
  }

  private async sendUrl(
    method: string,
    url: string,
    opts: {
      json?: unknown;
      headers?: Record<string, string>;
      expected: number[];
      binary?: boolean;
    },
  ): Promise<TransportResponse> {
    const headers: Record<string, string> = {
      Accept: opts.binary ? "*/*" : "application/json",
    };
    if (sameOrigin(this.baseUrl, url)) {
      Object.assign(headers, this.authHeaders());
    }
    Object.assign(headers, opts.headers);
    if (!sameOrigin(this.baseUrl, url)) {
      for (const key of Object.keys(headers)) {
        const lower = key.toLowerCase();
        if (lower === "authorization" || lower === "x-project-token" || lower === "x-channel-token") {
          delete headers[key];
        }
      }
    }
    let body: Uint8Array | null = null;
    if (opts.json !== undefined) {
      headers["Content-Type"] = "application/json";
      body = new TextEncoder().encode(JSON.stringify(opts.json));
    }
    const res = await this.transport.request({ method, url, headers, body });
    const normalized = { ...res, headers: lowerHeaderRecord(res.headers) };
    if (!opts.expected.includes(normalized.status)) {
      throw errorFromBody(normalized.status, normalized.body, normalized.headers);
    }
    return normalized;
  }

  private parseJson<T>(res: TransportResponse): T {
    const text = new TextDecoder().decode(res.body);
    if (text.trim() === "") {
      throw errorFromBody(res.status, res.body, res.headers);
    }
    try {
      return JSON.parse(text) as T;
    } catch {
      throw errorFromBody(res.status, res.body, res.headers);
    }
  }
}
