/**
 * composables/useBuildInfo.ts — 前后端编译信息（模块级单例 composable）。
 *
 * 为什么不是 Pinia：`.trellis/spec/frontend/state-management.md` 明令 `stores/app.ts`
 * 不得加字段，且编译信息是只读快照（design §8 的取舍：代价是不进 devtools 时间旅行）。
 *
 * 两个来源：
 * - `frontend`：Vite `define` 在编译期内联的 `__BUILD_INFO__`（见 vite.config.mts /
 *   env.d.ts），同步可得、不需要请求；
 * - `backend`：`GET /api/v1/admin/build-info`，仅完整管理会话可读，因此必须走鉴权后
 *   的生成方法（不硬编码路径）。
 *
 * 请求次数：ensureLoaded() 幂等（`requested` 标记），由 DefaultShell 挂载时调用一次，
 * footer 与"关于"对话框只读同一份缓存 —— 切路由、重挂组件都不会再发请求（AC6）。
 *
 * 失败降级：pending 会话 401、DB 不可用导致 /api 404、网络错误等一律只置 `failed`，
 * 不弹 snackbar、不自动重试（R5）；界面按占位文案显示。
 */
import { shallowRef, type ShallowRef } from 'vue'
import { type BuildInfo, getBuildInfo } from '@/api/generated'

/** 前端编译期构建信息（与 `__BUILD_INFO__` 的字面量类型一一对应）。 */
export type FrontendBuildInfo = {
  readonly version: string
  /** null 表示构建环境拿不到 git（例如 Docker frontend stage 里没有 .git 且未注入）。 */
  readonly commit: string | null
  readonly buildTime: string
}

export interface BuildInfoView {
  /** 编译期内联值，读取即得。 */
  frontend: FrontendBuildInfo
  /** 后端回报值；未发起请求、请求中或请求失败时为 null。 */
  backend: ShallowRef<BuildInfo | null>
  loading: ShallowRef<boolean>
  /** 后端信息不可得（失败或空响应），界面据此显示占位。 */
  failed: ShallowRef<boolean>
  ensureLoaded: () => Promise<void>
}

/** define 注入的对象字面量：只取一次引用，避免在产物里重复内联同一份常量。 */
const frontend: FrontendBuildInfo = __BUILD_INFO__

const backend = shallowRef<BuildInfo | null>(null)
const loading = shallowRef(false)
const failed = shallowRef(false)

/** 幂等标记：进程内至多发起一次请求，失败也不重试（避免刷屏与重复拉取）。 */
let requested = false

async function ensureLoaded (): Promise<void> {
  if (requested) {
    return
  }

  loading.value = true
  try {
    const { data } = await getBuildInfo()
    if (data) {
      backend.value = data
      requested = true
    } else {
      backend.value = null
      failed.value = false
    }
  } catch {
    // ApiError 在此吞掉：编译信息缺失不影响控制台任何功能（R5）。
    backend.value = null
    failed.value = true
  } finally {
    loading.value = false
  }
}

export function useBuildInfo (): BuildInfoView {
  return { backend, failed, frontend, loading, ensureLoaded }
}
