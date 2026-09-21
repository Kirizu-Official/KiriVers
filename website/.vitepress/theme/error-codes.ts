/** Hover copy for <ErrorCode>. Keep in lockstep with website/docs/faq/errors.md (zh + en). */

export type ErrorCodeCopy = { zh: string; en: string }

export const errorCodes: Record<string, ErrorCodeCopy> = {
  PROJECT_NOT_FOUND: {
    zh: '项目 UUID/slug/别名未命中或别名过期',
    en: 'Project identity missed or alias expired',
  },
  VERSION_NOT_FOUND: {
    zh: '版本不存在',
    en: 'Version missing',
  },
  VERSION_LINE_NOT_FOUND: {
    zh: '该版本没有此 os/arch 线',
    en: 'No line for that os/arch',
  },
  VERSION_NOT_VISIBLE: {
    zh: '版本对客户端不可见（草稿、未预热或灰度未命中）',
    en: 'Version not visible to the client (draft, packs not ready, or gray miss)',
  },
  VERSION_REVOKED: {
    zh: '版本已吊销且无安全回落',
    en: 'Revoked with no safe target',
  },
  NO_SAFE_TARGET: {
    zh: '没有安全目标版本',
    en: 'No safe target',
  },
  INTERMEDIATE_UNAVAILABLE: {
    zh: '中继版本缺失、未就绪、成环或 hop 超过 8 次',
    en: 'Min-source hop missing, not ready, cyclic, or more than 8 hops',
  },
  MIN_OS_NOT_MET: {
    zh: '客户端系统/API 低于当前版本线下限',
    en: 'OS/API below the current line floor',
  },
  CHANNEL_CONFLICT: {
    zh: '幂等创建时渠道与已有版本不一致',
    en: 'Idempotent create channel mismatch',
  },
  UNAUTHORIZED: {
    zh: '未认证或会话过期',
    en: 'Missing/invalid credentials',
  },
  FORBIDDEN: {
    zh: '权限不足（含项目成员访问 /admins、/geoip、/nodes）',
    en: 'Insufficient permission (including project-only access to /admins, /geoip, /nodes)',
  },
  PRECONDITION_FAILED: {
    zh: 'If-Match / ETag 失败（不是 POST check）',
    en: 'If-Match / ETag; not POST check',
  },
  HW_REV_INCOMPATIBLE: {
    zh: '硬件代号不在兼容范围',
    en: 'hw_rev out of range',
  },
  RATE_LIMITED: {
    zh: '过频；响应带 Retry-After',
    en: 'Too many requests; honor Retry-After',
  },
  CHANGELOG_QUERY_INVALID: {
    zh: 'changelog 查询非法',
    en: 'Illegal changelog query',
  },
  NOT_FOUND: {
    zh: '通用缺失（含未知包哈希）',
    en: 'Generic missing resource',
  },
  NOT_READY: {
    zh: '服务或 Passkey 未就绪。webauthn_rp_id 或 webauthn_origins 任一为空时 Passkey 接口返回本码（HTTP 503）',
    en: 'Service or Passkey not ready. Either empty webauthn_rp_id or webauthn_origins makes Passkey routes return this code (HTTP 503)',
  },
  INTERNAL_ERROR: {
    zh: '服务器内部错误',
    en: 'Internal error',
  },
  INVALID_REQUEST: {
    zh: '请求体/字段非法（含非 .mmdb、超上限、未知矩阵对、重复 listing）',
    en: 'Illegal body/fields (including non-.mmdb, caps, unknown matrix pair, duplicate listing)',
  },
  INVALID_QUERY_PARAM: {
    zh: '查询参数非法（含公告 version、未知 changelog 渠道）',
    en: 'Illegal query (including announcement version and unknown changelog channel)',
  },
  ENGINE_MISMATCH: {
    zh: '比较引擎与版本标识不匹配',
    en: 'Engine vs identity mismatch',
  },
  ARTIFACT_REQUIRED: {
    zh: '发布被拒绝：没有任何就绪的版本线',
    en: 'Publish rejected: no ready version line',
  },
  CHANNEL_SUFFIX_MISMATCH: {
    zh: 'SemVer 预发布后缀与渠道不一致；stable 不能用预发布后缀',
    en: 'Prerelease suffix vs channel; stable cannot use a prerelease suffix',
  },
  VERSION_ALREADY_EXISTS: {
    zh: '同项目版本号占用',
    en: 'Duplicate version in project',
  },
  GRAY_NOT_ALLOWED_ON_CRITICAL: {
    zh: '关键版本禁止灰度',
    en: 'Gray on critical version',
  },
  PACKAGE_TYPE_IMMUTABLE: {
    zh: '该平台已有已发布版本线后不能改产物形态',
    en: 'package_type locked after a published line exists',
  },
  COMPARE_ENGINE_IMMUTABLE: {
    zh: '已发布后不能改比较引擎',
    en: 'compare_engine locked after first publish',
  },
  INVALID_PATH: {
    zh: '非法路径',
    en: 'Illegal path',
  },
  ARTIFACT_IMMUTABLE: {
    zh: '已发布产物不可覆盖（先 yank 版本线）',
    en: 'Published artifact immutable (yank the line first)',
  },
  HW_REV_UNKNOWN: {
    zh: '未登记硬件代号',
    en: 'Unregistered hw_rev',
  },
  DELTA_ALGO_UNSUPPORTED: {
    zh: '不支持的差量算法',
    en: 'Unknown delta algo',
  },
  DELTA_SAME_VERSION: {
    zh: '源与目标版本相同',
    en: 'Delta source equals target',
  },
  TOTP_RATE_LIMITED: {
    zh: '同一 30s 周期 TOTP 次数用尽',
    en: 'TOTP attempts exhausted in one 30s period',
  },
  UPLOAD_INCOMPLETE: {
    zh: '上传未完成，不能发布或标记就绪',
    en: 'Upload incomplete for this operation',
  },
  CHANNEL_NOT_FOUND: {
    zh: '渠道不存在',
    en: 'Channel missing',
  },
  CHECKSUM_MISMATCH: {
    zh: '声明 SHA-256 与服务端复算不符',
    en: 'Declared SHA-256 mismatch',
  },
  JOB_NOT_FOUND: {
    zh: '任务不存在',
    en: 'Job missing',
  },
  JOB_FAILED: {
    zh: '异步任务失败',
    en: 'Async job failed',
  },
  AUTO_PUBLISH_PENDING: {
    zh: 'auto_publish_when 尚未全部就绪',
    en: 'auto_publish_when is not fully ready yet',
  },
  ZIP_LAYOUT_INVALID: {
    zh: 'zip 无法解析为 os/arch/…（缺段、未知标识或含路径穿越）',
    en: 'Zip layout is not os/arch/… (missing segments, unknown ids or traversal)',
  },
  ADMIN_NOT_FOUND: {
    zh: '管理员不存在',
    en: 'Admin missing',
  },
  USERNAME_TAKEN: {
    zh: '用户名占用',
    en: 'Username taken',
  },
  LAST_ADMIN: {
    zh: '不能删除最后一名管理员',
    en: 'Cannot delete last admin',
  },
  LAST_OWNER: {
    zh: '不能移除最后一名拥有者',
    en: 'Cannot remove last owner',
  },
  MEMBER_NOT_FOUND: {
    zh: '项目成员不存在',
    en: 'Member missing',
  },
  CLIENT_NOT_FOUND: {
    zh: '客户端名册行不存在',
    en: 'Roster client missing',
  },
  GEOIP_NOT_FOUND: {
    zh: 'GeoIP 库不存在',
    en: 'GeoIP database missing',
  },
  LANGUAGE_TAKEN: {
    zh: '语言代码已存在',
    en: 'Language code taken',
  },
  LANGUAGE_NOT_FOUND: {
    zh: '语言不存在',
    en: 'Language missing',
  },
  SYSTEM_CHANNEL: {
    zh: '系统渠道 alpha/beta/stable 不可删除',
    en: 'System channels alpha/beta/stable cannot be deleted',
  },
}

export function errorCodeText(code: string, lang: string): string | undefined {
  const entry = errorCodes[code]
  if (!entry) return undefined
  return lang.toLowerCase().startsWith('zh') ? entry.zh : entry.en
}
