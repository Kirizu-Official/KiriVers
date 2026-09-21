/**
 * api/client.ts
 *
 * Axios 实例与全局请求/响应拦截器的唯一配置点：
 * - 复用 hey-api 管理平面生成客户端内部的 axios 实例（client.instance）；
 * - 客户端平面 SDK 通过 setConfig({ axios }) 共用同一实例；
 * - Bearer 注入（token 由 auth store 注册提供，避免循环依赖）；
 * - POST/PUT 自动携带 Idempotency-Key（app-init.md §11.4）；
 * - 非 2xx 归一化为 ApiError{status, code, message, details}，
 *   解析后端统一错误体 {error:{code,message,details}}；
 * - 401：完整会话视为下线；pending 2FA 的错误验证码不得清掉向导（仅 invalid token 才丢受限 token）。
 */
import { AxiosError } from 'axios'
import { client as clientPlane } from './generated-client/client.gen'
import { client } from './generated/client.gen'

type ErrorDetail = {
  code?: string
  message?: string
  details?: unknown
}

/** 归一化后的 API 错误。页面/store 捕获后用 e.code 查 i18n 提示。 */
export class ApiError extends Error {
  readonly status: number
  readonly code: string
  readonly details?: unknown

  constructor (status: number, code: string, message: string, details?: unknown) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
    this.details = details
  }
}

/** 判断任意抛出物是否为 ApiError。 */
export function isApiError (e: unknown): e is ApiError {
  return e instanceof ApiError
}

type AuthHooks = {
  getToken: (url?: string) => string | null
  onUnauthorized: (error?: ApiError) => void
  onSuccess?: () => void
}

const hooks: AuthHooks = {
  getToken: () => null,
  onUnauthorized: () => {},
}

/**
 * 注册认证钩子。由 auth store 在创建时调用一次，
 * 避免 client ↔ store 循环导入。
 */
export function registerAuthHooks (next: Partial<AuthHooks>): void {
  if (next.getToken) {
    hooks.getToken = next.getToken
  }
  if (next.onUnauthorized) {
    hooks.onUnauthorized = next.onUnauthorized
  }
  if (next.onSuccess) {
    hooks.onSuccess = next.onSuccess
  }
}

const baseURL = (import.meta.env.VITE_API_BASE_URL as string | undefined) ?? ''

client.setConfig({
  baseURL,
  throwOnError: true,
})

/** 管理平面与客户端平面共用的 axios 实例（生成客户端内部实例）。 */
export const axiosInstance = client.instance

axiosInstance.interceptors.request.use(config => {
  const token = hooks.getToken(config.url)
  if (token) {
    config.headers.set('Authorization', `Bearer ${token}`)
  }
  const method = (config.method ?? 'get').toLowerCase()
  if ((method === 'post' || method === 'put') && !config.headers.has('Idempotency-Key')) {
    config.headers.set('Idempotency-Key', crypto.randomUUID())
  }
  return config
})

function normalizeError (err: unknown): ApiError {
  if (err instanceof ApiError) {
    return err
  }
  if (err instanceof AxiosError) {
    const status = err.response?.status ?? 0
    const body = (err.response?.data ?? {}) as { error?: ErrorDetail }
    const detail = body?.error
    if (detail?.code) {
      return new ApiError(status, detail.code, detail.message ?? '', detail.details)
    }
    if (status === 0) {
      return new ApiError(0, 'NETWORK_ERROR', err.message || 'network error')
    }
    return new ApiError(status, 'HTTP_ERROR', err.message || `http ${status}`)
  }
  return new ApiError(0, 'UNKNOWN_ERROR', err instanceof Error ? err.message : String(err))
}

axiosInstance.interceptors.response.use(
  response => {
    hooks.onSuccess?.()
    return response
  },
  err => {
    const apiError = normalizeError(err)
    if (apiError.status === 401) {
      hooks.onUnauthorized(apiError)
    }
    return Promise.reject(apiError)
  },
)

clientPlane.setConfig({
  axios: client.instance,
  baseURL,
  throwOnError: true,
})
