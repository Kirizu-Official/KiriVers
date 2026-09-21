import type { Admin, AdminLoginOutput } from '@/api/generated'
import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
/**
 * stores/auth.ts
 *
 * 完整会话 vs 登录受限 token：
 * - accessToken 才算 isAuthenticated，写入 localStorage（kirivers.auth）
 * - pendingToken 仅用于强制绑定 / 第二因素，写入 sessionStorage，不计入已登录
 * - 401：完整会话下线；pending 上错误验证码保持向导，仅 invalid token 清 pending
 * 成功的管理请求按登录时的 idle TTL 滑动本地过期时间。
 */
import { registerAuthHooks } from '@/api/client'
import { adminLogin, adminLogout } from '@/api/generated'

const STORAGE_KEY = 'kirivers.auth'
const PENDING_KEY = 'kirivers.auth.pending'
/** 提前 30 秒视为过期，避免边界竞态。 */
const EXPIRY_SLACK_MS = 30_000

interface PersistedSession {
  accessToken: string | null
  expiresAt: number
  idleMs: number
  admin: Admin | null
}

interface PersistedPending {
  pendingToken: string | null
  stage: AdminLoginOutput['stage'] | null
  expiresAt: number
  totpSecret: string | null
  otpauthUrl: string | null
  recoveryCodes: string[]
  hasPasskey: boolean
}

function loadSession (): PersistedSession {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (raw) {
      const parsed = JSON.parse(raw) as Partial<PersistedSession>
      return {
        accessToken: typeof parsed.accessToken === 'string' ? parsed.accessToken : null,
        expiresAt: typeof parsed.expiresAt === 'number' ? parsed.expiresAt : 0,
        idleMs: typeof parsed.idleMs === 'number' ? parsed.idleMs : 0,
        admin: parsed.admin ?? null,
      }
    }
  } catch {
    // 忽略损坏数据
  }
  return { accessToken: null, expiresAt: 0, idleMs: 0, admin: null }
}

function loadPending (): PersistedPending {
  try {
    const raw = sessionStorage.getItem(PENDING_KEY)
    if (raw) {
      const parsed = JSON.parse(raw) as Partial<PersistedPending>
      return {
        pendingToken: typeof parsed.pendingToken === 'string' ? parsed.pendingToken : null,
        stage: parsed.stage ?? null,
        expiresAt: typeof parsed.expiresAt === 'number' ? parsed.expiresAt : 0,
        totpSecret: typeof parsed.totpSecret === 'string' ? parsed.totpSecret : null,
        otpauthUrl: typeof parsed.otpauthUrl === 'string' ? parsed.otpauthUrl : null,
        recoveryCodes: Array.isArray(parsed.recoveryCodes) ? parsed.recoveryCodes.filter((c): c is string => typeof c === 'string') : [],
        hasPasskey: parsed.hasPasskey === true,
      }
    }
  } catch {
    // 忽略损坏数据
  }
  return { pendingToken: null, stage: null, expiresAt: 0, totpSecret: null, otpauthUrl: null, recoveryCodes: [], hasPasskey: false }
}

function persist (session: PersistedSession): void {
  if (session.accessToken) {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(session))
  } else {
    localStorage.removeItem(STORAGE_KEY)
  }
}

function persistPending (pending: PersistedPending): void {
  if (pending.pendingToken) {
    sessionStorage.setItem(PENDING_KEY, JSON.stringify(pending))
  } else {
    sessionStorage.removeItem(PENDING_KEY)
  }
}

export const useAuthStore = defineStore('auth', () => {
  const initial = loadSession()
  const initialPending = loadPending()
  const accessToken = ref<string | null>(initial.accessToken)
  const expiresAt = ref(initial.expiresAt)
  const idleMs = ref(initial.idleMs)
  const admin = ref<Admin | null>(initial.admin)
  const pendingToken = ref<string | null>(initialPending.pendingToken)
  const pendingStage = ref<AdminLoginOutput['stage'] | null>(initialPending.stage)
  const pendingExpiresAt = ref(initialPending.expiresAt)
  const totpSecret = ref<string | null>(initialPending.totpSecret)
  const otpauthUrl = ref<string | null>(initialPending.otpauthUrl)
  const recoveryCodes = ref<string[]>(initialPending.recoveryCodes)
  const hasPasskey = ref(initialPending.hasPasskey)

  const isAuthenticated = computed(() => Boolean(accessToken.value) && expiresAt.value - EXPIRY_SLACK_MS > Date.now())
  const username = computed(() => admin.value?.username ?? '')
  const isPlatformAdmin = computed(() => admin.value?.is_platform_admin !== false)
  const hasPending = computed(() => Boolean(pendingToken.value) && pendingExpiresAt.value > Date.now())

  function snapshotPending (): PersistedPending {
    return {
      pendingToken: pendingToken.value,
      stage: pendingStage.value,
      expiresAt: pendingExpiresAt.value,
      totpSecret: totpSecret.value,
      otpauthUrl: otpauthUrl.value,
      recoveryCodes: recoveryCodes.value,
      hasPasskey: hasPasskey.value,
    }
  }

  function snapshotSession (): PersistedSession {
    return {
      accessToken: accessToken.value,
      expiresAt: expiresAt.value,
      idleMs: idleMs.value,
      admin: admin.value,
    }
  }

  function clearPending (): void {
    pendingToken.value = null
    pendingStage.value = null
    pendingExpiresAt.value = 0
    totpSecret.value = null
    otpauthUrl.value = null
    recoveryCodes.value = []
    hasPasskey.value = false
    persistPending({
      pendingToken: null,
      stage: null,
      expiresAt: 0,
      totpSecret: null,
      otpauthUrl: null,
      recoveryCodes: [],
      hasPasskey: false,
    })
  }

  function clear (): void {
    accessToken.value = null
    expiresAt.value = 0
    idleMs.value = 0
    admin.value = null
    persist({ accessToken: null, expiresAt: 0, idleMs: 0, admin: null })
    clearPending()
  }

  function applyAuthResult (data: AdminLoginOutput | undefined): AdminLoginOutput {
    if (!data) {
      throw new Error('empty login response')
    }
    if (data.status === 'complete' && data.access_token) {
      clearPending()
      accessToken.value = data.access_token
      idleMs.value = (data.expires_in ?? 0) * 1000
      expiresAt.value = Date.now() + idleMs.value
      admin.value = data.admin ?? null
      persist(snapshotSession())
      return data
    }
    if (data.pending_token) {
      pendingToken.value = data.pending_token
    }
    if (data.stage) {
      pendingStage.value = data.stage
    }
    if (typeof data.expires_in === 'number' && data.expires_in > 0) {
      pendingExpiresAt.value = Date.now() + data.expires_in * 1000
    }
    if (data.secret) {
      totpSecret.value = data.secret
      otpauthUrl.value = data.otpauth_url ?? null
    }
    if (data.recovery_codes?.length) {
      recoveryCodes.value = data.recovery_codes
    } else if (data.stage && data.stage !== 'ack_recovery') {
      recoveryCodes.value = []
    }
    hasPasskey.value = data.status === 'pending' && data.stage === 'second_factor' && data.has_passkey === true
    persistPending(snapshotPending())
    return data
  }

  async function login (username: string, password: string): Promise<AdminLoginOutput> {
    const { data } = await adminLogin({ body: { username, password } })
    return applyAuthResult(data)
  }

  async function logout (): Promise<void> {
    try {
      if (isAuthenticated.value) {
        await adminLogout()
      }
    } catch {
      // 本地仍须下线
    }
    clear()
  }

  function touchSession (): void {
    if (!accessToken.value || idleMs.value <= 0) {
      return
    }
    if (expiresAt.value - EXPIRY_SLACK_MS <= Date.now()) {
      return
    }
    expiresAt.value = Date.now() + idleMs.value
    persist(snapshotSession())
  }

  registerAuthHooks({
    getToken: url => {
      if (isAuthenticated.value) {
        return accessToken.value
      }
      if (!pendingToken.value || pendingExpiresAt.value <= Date.now()) {
        return null
      }
      // 受限 token 只用于 /auth/2fa；其它请求带上会 401 并打断绑定向导。
      if (url && !url.includes('/auth/2fa')) {
        return null
      }
      return pendingToken.value
    },
    onUnauthorized: error => {
      if (isAuthenticated.value) {
        clear()
        void import('@/router').then(({ default: router }) => {
          void router.push({ path: '/login', query: { expired: '1' } })
        })
        return
      }
      if (error?.message === 'invalid token') {
        clearPending()
      }
    },
    onSuccess: () => {
      touchSession()
    },
  })

  return {
    accessToken,
    expiresAt,
    admin,
    pendingToken,
    pendingStage,
    totpSecret,
    otpauthUrl,
    recoveryCodes,
    hasPasskey,
    isAuthenticated,
    hasPending,
    username,
    isPlatformAdmin,
    login,
    logout,
    clear,
    clearPending,
    applyAuthResult,
  }
})
