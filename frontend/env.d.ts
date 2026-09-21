/// <reference types="vite/client" />
/// <reference types="vite-plugin-vue-layouts-next/client" />
/// <reference types="unplugin-vue-router/client" />

interface ImportMetaEnv {
  /** 真实后端地址；缺省同源（dev 走 Vite 代理分流 admin/client 双平面） */
  readonly VITE_API_BASE_URL?: string
}

/**
 * 前端编译期构建信息：由 vite.config.mts 的 `define.__BUILD_INFO__` 内联为字面量，
 * 与后端 internal/buildinfo 共用 KIRIVERS_VERSION / KIRIVERS_BUILD_COMMIT /
 * KIRIVERS_BUILD_TIME 三个 env。
 *
 * commit 为 null 表示构建环境拿不到 git（例如 Docker frontend stage 无 .git 且未注入）。
 * buildTime 通常是 RFC3339 字符串，但注入方（老产物/手工构建）可能给出任意文本，
 * 消费侧不得假设它一定能被 Date.parse。
 */
declare const __BUILD_INFO__: {
  readonly version: string
  readonly commit: string | null
  readonly buildTime: string
}
