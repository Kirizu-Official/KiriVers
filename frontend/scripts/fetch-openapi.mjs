/**
 * scripts/fetch-openapi.mjs
 *
 * Prepare hey-api inputs from the plane contracts (the only committed OpenAPI JSON):
 * 1. Read internal/controller/openapi.admin.json and openapi.client.json.
 *    Set KIRIVERS_OPENAPI_URL / KIRIVERS_OPENAPI_CLIENT_URL to download from a running server.
 * 2. Inject deterministic operationId values from NAMES (gitignored snapshots only).
 * 3. Write frontend/.openapi/{admin,client}.json for openapi-ts dual jobs.
 */
import { mkdir, readFile, writeFile } from 'node:fs/promises'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = resolve(dirname(fileURLToPath(import.meta.url)), '../..')

/** 方法 + 精确路径 → 可读 operationId。未命中则回退自动命名。 */
const NAMES = {
  'POST /api/v1/admin/auth/login': 'adminLogin',
  'POST /api/v1/admin/auth/logout': 'adminLogout',
  'POST /api/v1/admin/auth/2fa/totp/setup': 'adminTotpSetup',
  'POST /api/v1/admin/auth/2fa/totp/confirm': 'adminTotpConfirm',
  'POST /api/v1/admin/auth/2fa/recovery/ack': 'adminRecoveryAck',
  'POST /api/v1/admin/auth/2fa/skip-passkey': 'adminSkipPasskey',
  'POST /api/v1/admin/auth/2fa/verify': 'adminVerify2fa',
  'POST /api/v1/admin/auth/2fa/webauthn/login/begin': 'adminWebauthnLoginBegin',
  'POST /api/v1/admin/auth/2fa/webauthn/login/finish': 'adminWebauthnLoginFinish',
  'POST /api/v1/admin/auth/2fa/webauthn/register/begin': 'adminWebauthnRegisterBegin',
  'POST /api/v1/admin/auth/2fa/webauthn/register/finish': 'adminWebauthnRegisterFinish',
  'GET /api/v1/admin/auth/2fa': 'getAdmin2fa',
  'POST /api/v1/admin/auth/2fa/totp/rotate/setup': 'adminTotpRotateSetup',
  'POST /api/v1/admin/auth/2fa/totp/rotate/confirm': 'adminTotpRotateConfirm',
  'POST /api/v1/admin/auth/2fa/recovery/regenerate': 'adminRecoveryRegenerate',
  'GET /api/v1/admin/auth/2fa/passkeys': 'listAdminPasskeys',
  'POST /api/v1/admin/auth/2fa/passkeys/begin': 'adminPasskeyBegin',
  'POST /api/v1/admin/auth/2fa/passkeys/finish': 'adminPasskeyFinish',
  'DELETE /api/v1/admin/auth/2fa/passkeys/{id}': 'deleteAdminPasskey',
  'GET /api/v1/admin/admins': 'listAdmins',
  'POST /api/v1/admin/admins': 'createAdmin',
  'PATCH /api/v1/admin/admins/{id}': 'updateAdmin',
  'DELETE /api/v1/admin/admins/{id}': 'deleteAdmin',
  'GET /api/v1/admin/projects': 'listProjects',
  'POST /api/v1/admin/projects': 'createProject',
  'GET /api/v1/admin/projects/{project_ref}': 'getProject',
  'PATCH /api/v1/admin/projects/{project_ref}': 'updateProject',
  'DELETE /api/v1/admin/projects/{project_ref}': 'deleteProject',
  'GET /api/v1/admin/projects/{project_ref}/tokens': 'listProjectTokens',
  'POST /api/v1/admin/projects/{project_ref}/tokens': 'createProjectToken',
  'DELETE /api/v1/admin/projects/{project_ref}/tokens/{token_id}': 'deleteProjectToken',
  'GET /api/v1/admin/projects/{project_ref}/ci-tokens': 'listCiTokens',
  'POST /api/v1/admin/projects/{project_ref}/ci-tokens': 'createCiToken',
  'DELETE /api/v1/admin/projects/{project_ref}/ci-tokens/{token_id}': 'deleteCiToken',
  'GET /api/v1/admin/projects/{project_ref}/channels': 'listChannels',
  'POST /api/v1/admin/projects/{project_ref}/channels': 'createChannel',
  'PATCH /api/v1/admin/projects/{project_ref}/channels/{slug}': 'updateChannel',
  'DELETE /api/v1/admin/projects/{project_ref}/channels/{slug}': 'deleteChannel',
  'GET /api/v1/admin/projects/{project_ref}/install-policy-rules': 'getProjectInstallPolicyRules',
  'PUT /api/v1/admin/projects/{project_ref}/install-policy-rules': 'putProjectInstallPolicyRules',
  'GET /api/v1/admin/projects/{project_ref}/install-policy-rules/reference': 'getInstallPolicyRulesReference',
  'GET /api/v1/admin/projects/{project_ref}/channels/{slug}/install-policy-rules': 'getChannelInstallPolicyRules',
  'PUT /api/v1/admin/projects/{project_ref}/channels/{slug}/install-policy-rules': 'putChannelInstallPolicyRules',
  'GET /api/v1/admin/projects/{project_ref}/languages': 'listLanguages',
  'POST /api/v1/admin/projects/{project_ref}/languages': 'createLanguage',
  'PATCH /api/v1/admin/projects/{project_ref}/languages/{code}': 'updateLanguage',
  'DELETE /api/v1/admin/projects/{project_ref}/languages/{code}': 'deleteLanguage',
  'GET /api/v1/admin/projects/{project_ref}/matrix': 'listMatrix',
  'POST /api/v1/admin/projects/{project_ref}/matrix': 'createMatrix',
  'PATCH /api/v1/admin/projects/{project_ref}/matrix/{os}/{arch}': 'updateMatrix',
  'GET /api/v1/admin/projects/{project_ref}/hw-revs': 'listHwRevs',
  'POST /api/v1/admin/projects/{project_ref}/hw-revs': 'createHwRev',
  'PATCH /api/v1/admin/projects/{project_ref}/hw-revs/{slug}': 'updateHwRev',
  'DELETE /api/v1/admin/projects/{project_ref}/hw-revs/{slug}': 'deleteHwRev',
  'GET /api/v1/admin/projects/{project_ref}/platforms/catalog': 'getPlatformCatalog',
  'GET /api/v1/admin/projects/{project_ref}/versions': 'listVersions',
  'GET /api/v1/admin/projects/{project_ref}/versions/{version}/exists': 'versionExists',
  'PUT /api/v1/admin/projects/{project_ref}/versions/{version}': 'putVersion',
  'GET /api/v1/admin/projects/{project_ref}/versions/{version}': 'getVersion',
  'PATCH /api/v1/admin/projects/{project_ref}/versions/{version}': 'patchVersion',
  'DELETE /api/v1/admin/projects/{project_ref}/versions/{version}': 'deleteVersion',
  'POST /api/v1/admin/projects/{project_ref}/versions/{version}/publish': 'publishVersion',
  'POST /api/v1/admin/projects/{project_ref}/versions/{version}/deprecate': 'deprecateVersion',
  'POST /api/v1/admin/projects/{project_ref}/versions/{version}/revoke': 'revokeVersion',
  'GET /api/v1/admin/projects/{project_ref}/versions/{version}/lines': 'listVersionLines',
  'POST /api/v1/admin/projects/{project_ref}/versions/{version}/lines': 'createVersionLine',
  'GET /api/v1/admin/projects/{project_ref}/versions/{version}/lines/{os}/{arch}': 'getVersionLine',
  'POST /api/v1/admin/projects/{project_ref}/versions/{version}/lines/{os}/{arch}/ready': 'readyVersionLine',
  'POST /api/v1/admin/projects/{project_ref}/versions/{version}/lines/{os}/{arch}/yank': 'yankVersionLine',
  'POST /api/v1/admin/projects/{project_ref}/versions/{version}/lines/{os}/{arch}/disable': 'disableVersionLine',
  'PUT /api/v1/admin/projects/{project_ref}/versions/{version}/lines/{os}/{arch}/artifacts': 'putLineArtifact',
  'POST /api/v1/admin/projects/{project_ref}/versions/{version}/lines/{os}/{arch}/artifacts': 'postLineArtifact',
  'POST /api/v1/admin/projects/{project_ref}/versions/{version}/lines/{os}/{arch}/artifacts/tus': 'createTusUpload',
  'HEAD /api/v1/admin/projects/{project_ref}/artifacts/tus/{upload_id}': 'headTusUpload',
  'PATCH /api/v1/admin/projects/{project_ref}/artifacts/tus/{upload_id}': 'patchTusUpload',
  'DELETE /api/v1/admin/projects/{project_ref}/artifacts/tus/{upload_id}': 'deleteTusUpload',
  'POST /api/v1/admin/projects/{project_ref}/versions/{version}/lines/{os}/{arch}/artifacts/presign': 'presignArtifactUpload',
  'POST /api/v1/admin/projects/{project_ref}/artifacts/cleanup': 'cleanupArtifacts',
  'POST /api/v1/admin/projects/{project_ref}/ci/releases': 'createCiRelease',
  'POST /api/v1/admin/projects/{project_ref}/versions/{version}/artifacts/bundle': 'uploadVersionBundle',
  'GET /api/v1/admin/projects/{project_ref}/versions/{version}/lines/{os}/{arch}/manifest': 'getManifest',
  'PUT /api/v1/admin/projects/{project_ref}/versions/{version}/lines/{os}/{arch}/manifest': 'putManifest',
  'POST /api/v1/admin/projects/{project_ref}/versions/{version}/lines/{os}/{arch}/build-archive': 'buildLineArchive',
  'GET /api/v1/admin/jobs/{job_id}': 'getAdminJob',
  'POST /api/v1/projects/{project_ref}/update/check': 'checkUpdate',
  'GET /api/v1/admin/projects/{project_ref}/announcements': 'listAnnouncements',
  'POST /api/v1/admin/projects/{project_ref}/announcements': 'createAnnouncement',
  'GET /api/v1/admin/projects/{project_ref}/announcements/{announcement_id}': 'getAnnouncement',
  'PATCH /api/v1/admin/projects/{project_ref}/announcements/{announcement_id}': 'patchAnnouncement',
  'DELETE /api/v1/admin/projects/{project_ref}/announcements/{announcement_id}': 'deleteAnnouncement',
  'PUT /api/v1/admin/projects/{project_ref}/announcements/reorder': 'reorderAnnouncements',
  'POST /api/v1/admin/projects/{project_ref}/media': 'uploadProjectMedia',
  'GET /api/v1/projects/{project_ref}/announcements': 'listClientAnnouncements',
  'GET /api/v1/projects/{project_ref}/media/{id}': 'getProjectMedia',
  'HEAD /api/v1/projects/{project_ref}/media/{id}': 'headProjectMedia',
  'GET /api/v1/projects/{project_ref}/changelog/{channel}/{os}/{arch}': 'getChangelog',
  'GET /api/v1/projects/{project_ref}/versions/{version}/integrity': 'getIntegrity',
  'POST /api/v1/projects/{project_ref}/telemetry/report': 'reportTelemetry',
  'GET /api/v1/health': 'getHealth',
  'GET /api/v1/openapi.json': 'getOpenAPISpec',
  'GET /api/v1/admin/build-info': 'getBuildInfo',
  'GET /api/v1/admin/projects/{project_ref}/audit': 'listAuditLogs',
  'DELETE /api/v1/admin/projects/{project_ref}/telemetry/devices/{device_hash}': 'deleteDeviceTelemetry',
  'POST /api/v1/admin/projects/{project_ref}/versions/{version}/artifacts/delta': 'createDeltaJob',
  'POST /api/v1/admin/projects/{project_ref}/versions/{version}/artifacts/reuse': 'reuseArtifact',
  'DELETE /api/v1/admin/projects/{project_ref}/versions/{version}/gray/allowlist': 'deleteVersionGrayAllowlist',
  'POST /api/v1/admin/projects/{project_ref}/versions/{version}/gray/allowlist': 'addVersionGrayAllowlist',
  'GET /api/v1/admin/projects/{project_ref}/versions/{version}/gray/allowlist': 'listVersionGrayAllowlist',
  'GET /api/v1/admin/projects/{project_ref}/versions/{version}/gray': 'getVersionGray',
  'PATCH /api/v1/admin/projects/{project_ref}/versions/{version}/gray': 'patchVersionGray',
  'POST /api/v1/admin/projects/{project_ref}/versions/{version}/gray/complete': 'completeVersionGray',
  'GET /api/v1/admin/projects/{project_ref}/versions/{version}/gray/clients': 'listVersionGrayClients',
  'GET /api/v1/admin/projects/{project_ref}/versions/{version}/gray/series': 'listVersionGraySeries',
  'GET /api/v1/admin/projects/{project_ref}/clients': 'listProjectClients',
  'GET /api/v1/admin/projects/{project_ref}/clients/stats': 'getProjectClientStats',
  'GET /api/v1/admin/projects/{project_ref}/clients/{client_id}': 'getProjectClient',
  'DELETE /api/v1/admin/projects/{project_ref}/clients/{client_id}': 'deleteProjectClient',
  'GET /api/v1/admin/projects/{project_ref}/members': 'listProjectMembers',
  'POST /api/v1/admin/projects/{project_ref}/members': 'createProjectMember',
  'PATCH /api/v1/admin/projects/{project_ref}/members/{admin_id}': 'updateProjectMember',
  'DELETE /api/v1/admin/projects/{project_ref}/members/{admin_id}': 'deleteProjectMember',
  'GET /api/v1/admin/geoip/databases': 'listGeoipDatabases',
  'POST /api/v1/admin/geoip/databases': 'uploadGeoipDatabase',
  'PATCH /api/v1/admin/geoip/databases/{id}': 'updateGeoipDatabase',
  'DELETE /api/v1/admin/geoip/databases/{id}': 'deleteGeoipDatabase',
  'GET /api/v1/admin/nodes': 'listNodes',
  'GET /api/v1/admin/projects/{project_ref}/node-sync': 'listProjectNodeSync',
  'GET /api/v1/admin/projects/{project_ref}/stats/series': 'getProjectStatsSeries',
  'POST /api/v1/projects/{project_ref}/clients/report': 'reportClient',
  'PATCH /api/v1/admin/projects/{project_ref}/versions/{version}/lines/{os}/{arch}': 'patchVersionLine',
  'GET /api/v1/admin/projects/{project_ref}/versions/{version}/line-defaults': 'getVersionLineDefaults',
  'GET /api/v1/projects/{project_ref}/channels': 'listClientChannels',
  'GET /api/v1/projects/{project_ref}/matrix': 'listClientMatrix',
  'GET /api/v1/projects/{project_ref}/languages': 'listClientLanguages',
  'POST /api/v1/admin/projects/{project_ref}/versions/{version}/promote': 'promoteVersion',
  'GET /api/v1/admin/projects/{project_ref}/webhook/deliveries': 'listWebhookDeliveries',
  'GET /api/v1/admin/projects/{project_ref}/store-listings': 'listStoreListings',
  'POST /api/v1/admin/projects/{project_ref}/store-listings': 'createStoreListing',
  'PATCH /api/v1/admin/projects/{project_ref}/store-listings/{listing_id}': 'updateStoreListing',
  'DELETE /api/v1/admin/projects/{project_ref}/store-listings/{listing_id}': 'deleteStoreListing',
  'GET /api/v1/projects/{project_ref}': 'getPublicProject',
  'OPTIONS /api/v1/projects/{project_ref}': 'optionsPublicProject',
  'GET /api/v1/projects/{project_ref}/packages/{ref}': 'downloadPackage',
  'HEAD /api/v1/projects/{project_ref}/packages/{ref}': 'headPackage',
  'POST /api/v1/projects/{project_ref}/update/diff': 'updateDiff',
  'POST /api/v1/projects/{project_ref}/update/pack': 'updatePack',
  'GET /api/v1/projects/{project_ref}/store/{protocol}/{listing_slug}': 'getStoreFeedListing',
  'POST /api/v1/projects/{project_ref}/store/{protocol}/{listing_slug}': 'postStoreFeedListing',
  'GET /api/v1/projects/{project_ref}/store/{protocol}/{listing_slug}/{doc}': 'getStoreFeed',
  'POST /api/v1/projects/{project_ref}/store/{protocol}/{listing_slug}/{doc}': 'postStoreFeed',
}

function injectOperationIds (spec) {
  let mapped = 0
  for (const [path, ops] of Object.entries(spec.paths ?? {})) {
    for (const method of Object.keys(ops)) {
      const op = ops[method]
      if (!op || typeof op !== 'object' || op.operationId) {
        continue
      }
      const name = NAMES[`${method.toUpperCase()} ${path}`]
      if (name) {
        op.operationId = name
        mapped++
      }
    }
  }
  return mapped
}

async function loadSpec (url, filePath, label) {
  if (url) {
    const res = await fetch(url)
    if (!res.ok) {
      throw new Error(`download failed: ${res.status} ${url}`)
    }
    const spec = await res.json()
    console.log(`[fetch-openapi] ${label}: downloaded from ${url}`)
    return spec
  }
  const spec = JSON.parse(await readFile(filePath, 'utf8'))
  console.log(`[fetch-openapi] ${label}: read ${filePath}`)
  return spec
}

async function writeSpec (spec, outFile, label) {
  const mapped = injectOperationIds(spec)
  console.log(`[fetch-openapi] ${label}: injected ${mapped} operationId(s)`)
  await mkdir(dirname(outFile), { recursive: true })
  await writeFile(outFile, JSON.stringify(spec, null, 2) + '\n', 'utf8')
  console.log(`[fetch-openapi] ${label}: wrote ${outFile}`)
}

const jobs = [
  {
    label: 'admin',
    url: process.env.KIRIVERS_OPENAPI_URL,
    file: resolve(root, 'internal/controller/openapi.admin.json'),
    out: resolve(root, 'frontend/.openapi/admin.json'),
  },
  {
    label: 'client',
    url: process.env.KIRIVERS_OPENAPI_CLIENT_URL,
    file: resolve(root, 'internal/controller/openapi.client.json'),
    out: resolve(root, 'frontend/.openapi/client.json'),
  },
]

for (const job of jobs) {
  const spec = await loadSpec(job.url, job.file, job.label)
  await writeSpec(spec, job.out, job.label)
}
