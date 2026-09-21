import { execFileSync } from 'node:child_process'
import { cpSync, existsSync, mkdirSync, writeFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath, URL } from 'node:url'
import tailwindcss from '@tailwindcss/vite'
import Vue from '@vitejs/plugin-vue'
import Fonts from 'unplugin-fonts/vite'
import { defineConfig, type Plugin } from 'vite'
import Vuetify, { transformAssetUrls } from 'vite-plugin-vuetify'
import VueRouter from 'vue-router/vite'

const root = dirname(fileURLToPath(import.meta.url))

/**
 * 尽力取当前工作树的短 SHA，用于开发态本地 `yarn build` / `yarn dev` 的 commit 回退。
 * 任何失败（无 git 可执行、非 git 工作树、Docker frontend stage 里没有 .git ——
 * .dockerignore 排除了 .git）都返回 null，绝不让构建报错；展示端按占位处理。
 */
function gitShortCommit (): string | null {
  try {
    return execFileSync('git', ['rev-parse', '--short', 'HEAD'], {
      cwd: root,
      encoding: 'utf8',
      stdio: ['ignore', 'pipe', 'ignore'],
    }).trim() || null
  } catch {
    return null
  }
}

/**
 * 前端编译期构建信息（design §5）：与后端 internal/buildinfo 共用同一组 env ——
 * KIRIVERS_VERSION / KIRIVERS_BUILD_COMMIT / KIRIVERS_BUILD_TIME，由
 * dev/build/kirivers_build/build.py、dev/build/Dockerfile、CI 各 job 写入。
 * 回退：版本 'dev'、commit 走 gitShortCommit()、构建时间取本次构建时刻
 * （`yarn dev` 下即 dev server 启动时刻，开发态语义可接受）。
 * 用 `define` 而非 `import.meta.env.VITE_*`：不依赖 Vite 对 process.env 前缀的透传
 * 行为差异，且类型收口在一个常量对象上（声明见 env.d.ts）。
 * 空串按未注入处理（`env: KIRIVERS_VERSION=` 在 CI 里是可能的）。
 */
const buildInfo = {
  version: process.env.KIRIVERS_VERSION || 'dev',
  commit: process.env.KIRIVERS_BUILD_COMMIT || gitShortCommit(),
  buildTime: process.env.KIRIVERS_BUILD_TIME || new Date().toISOString(),
}

/** Copy npm vditor/dist → vditor/dist so options.cdn can stay same-origin (`${cdn}/dist/...`). */
function copyVditorDist (destRoot: string): void {
  const src = resolve(root, 'node_modules/vditor/dist')
  const dest = resolve(destRoot, 'vditor', 'dist')
  if (!existsSync(src)) {
    throw new Error(`vditor dist missing at ${src}; run yarn add vditor`)
  }
  mkdirSync(resolve(destRoot, 'vditor'), { recursive: true })
  cpSync(src, dest, { recursive: true })
}

function vditorCopyPlugin (): Plugin {
  return {
    name: 'vditor-copy',
    buildStart () {
      copyVditorDist(resolve(root, 'public'))
    },
    closeBundle () {
      copyVditorDist(resolve(root, 'dist'))
      // emptyOutDir 会删掉 dist/.gitkeep；占位文件必须保留，否则未再次 yarn build 时 go:embed 失败。
      writeFileSync(resolve(root, 'dist/.gitkeep'), '')
    },
  }
}

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [vditorCopyPlugin(), VueRouter({ dts: 'src/typed-router.d.ts' }), tailwindcss(), Vue({
    template: { transformAssetUrls },
  }), // https://github.com/vuetifyjs/vuetify-loader/tree/master/packages/vite-plugin#readme
  Vuetify({
    autoImport: true,
    styles: {
      configFile: 'src/styles/settings.scss',
    },
  }), Fonts({
    fontsource: {
      families: [
        {
          name: 'Roboto Mono',
          weights: [400, 700],
        },
        {
          name: 'Roboto',
          weights: [100, 300, 400, 500, 700, 900],
          styles: ['normal', 'italic'],
        },
      ],
    },
  })],
  // 'process.env': {} 是既有兜底（保持不动）；__BUILD_INFO__ 显式 JSON.stringify，
  // 让注入值成为编译期字面量常量，产物里不再有任何运行期 env 读取。
  define: { 'process.env': {}, '__BUILD_INFO__': JSON.stringify(buildInfo) },
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('src', import.meta.url)),
    },
    extensions: [
      '.js',
      '.json',
      '.jsx',
      '.mjs',
      '.ts',
      '.tsx',
      '.vue',
    ],
  },
  server: {
    port: 3000,
    // 开发代理（生产为同源反代分流）：管理平面 /api/v1/admin/**（含探活）
    // 在 configs/admin.yaml addr（:8081），可用 KIRIVERS_API_URL 覆盖；
    // 客户端平面 /api/v1/projects/**（check/integrity 预览等公开端点）
    // 在 configs/client.yaml addr（:8080），可用 KIRIVERS_CLIENT_API_URL 覆盖。
    // 注意：Vite 代理按 key 顺序匹配，更具体的 /api/v1/projects 必须放在通用 /api 之前。
    proxy: {
      '/api/v1/projects': {
        target: process.env.KIRIVERS_CLIENT_API_URL ?? 'http://127.0.0.1:8080',
        changeOrigin: true,
      },
      '/api': {
        target: process.env.KIRIVERS_API_URL ?? 'http://127.0.0.1:8081',
        changeOrigin: true,
      },
    },
  },
})
