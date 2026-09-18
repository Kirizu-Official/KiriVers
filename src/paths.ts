/** Native JSON routes the handwritten client implements. Store feeds and leftover paths are omitted. */
export const PATHS = {
  health: "/api/v1/health",
  project: "/api/v1/projects/{project_ref}",
  announcements: "/api/v1/projects/{project_ref}/announcements",
  channels: "/api/v1/projects/{project_ref}/channels",
  languages: "/api/v1/projects/{project_ref}/languages",
  matrix: "/api/v1/projects/{project_ref}/matrix",
  media: "/api/v1/projects/{project_ref}/media/{id}",
  packages: "/api/v1/projects/{project_ref}/packages/{ref}",
  telemetry: "/api/v1/projects/{project_ref}/telemetry/report",
  check: "/api/v1/projects/{project_ref}/update/check",
  diff: "/api/v1/projects/{project_ref}/update/diff",
  pack: "/api/v1/projects/{project_ref}/update/pack",
  integrity: "/api/v1/projects/{project_ref}/versions/{version}/integrity",
  deviceReport: "/api/v1/projects/{project_ref}/clients/report",
  changelog: "/api/v1/projects/{project_ref}/changelog/{channel}/{os}/{arch}",
} as const;

export const LEFTOVER_PATHS = [
  "/update/check",
  "/clients/login",
  "/manifest",
  "/update/pack/status",
  "/store/",
  "/artifacts/",
  "/api/v1/ready",
] as const;

export function expandPath(template: string, params: Record<string, string>): string {
  return template.replace(/\{([^{}]+)\}/g, (_, key: string) => {
    const value = params[key];
    if (value === undefined) {
      throw new Error(`missing path parameter ${key}`);
    }
    return encodeURIComponent(value);
  });
}
