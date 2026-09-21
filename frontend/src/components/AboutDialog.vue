<!-- AboutDialog.vue — "关于"对话框：分区展示前端（编译期注入）与后端（build-info 端点）编译信息 -->
<template>
  <v-dialog v-model="model" max-width="520">
    <v-card>
      <v-card-title class="d-flex align-center">
        <v-icon class="mr-3" icon="mdi-information-outline" />
        {{ t('about.title') }}
      </v-card-title>

      <v-card-text>
        <div class="mb-1 text-subtitle-1 font-weight-medium">{{ t('about.frontend') }}</div>

        <div v-for="row in frontendRows" :key="row.label" class="align-center d-flex ga-4 py-1">
          <span class="flex-shrink-0 text-caption text-medium-emphasis w-25">{{ row.label }}</span>

          <span class="text-body-2 text-break">{{ row.value }}</span>
        </div>

        <v-divider class="my-3" />

        <div class="mb-1 text-subtitle-1 font-weight-medium">{{ t('about.backend') }}</div>

        <div v-for="row in backendRows" :key="row.label" class="align-center d-flex ga-4 py-1">
          <span class="flex-shrink-0 text-caption text-medium-emphasis w-25">{{ row.label }}</span>

          <span class="text-body-2 text-break">{{ row.value }}</span>
        </div>
      </v-card-text>

      <v-card-actions>
        <v-spacer />
        <v-btn variant="text" @click="model = false">{{ t('common.close') }}</v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>

<script lang="ts" setup>
  import { computed } from 'vue'
  import { useI18n } from 'vue-i18n'
  import { useBuildInfo } from '@/composables/useBuildInfo'

  const model = defineModel<boolean>({ default: false })

  const { t } = useI18n()
  const build = useBuildInfo()

  /** 一行「标签 / 值」；值一律是可直接展示的字符串，不留空串。 */
  interface InfoRow {
    label: string
    value: string
  }

  /** 文本字段不可得（null / 空串）时的占位，例如 Docker 构建拿不到 commit。 */
  function text (raw?: string | null): string {
    return raw && raw.trim() ? raw : t('common.none')
  }

  /**
   * 构建时间展示：仓库没有日期库，沿用 overview.vue 的 ISO 切片写法（UTC 视角，
   * 形如 `2026-09-21 03:12:44 UTC`）。回退值（`unknown` 等）解析不出时间戳时原样
   * 显示，绝不用当前时刻伪造编译时间。
   */
  function time (raw?: string | null): string {
    if (!raw) {
      return t('common.none')
    }
    const ms = Date.parse(raw)
    if (Number.isNaN(ms)) {
      return raw
    }
    return `${new Date(ms).toISOString().slice(0, 19).replace('T', ' ')} UTC`
  }

  const frontendRows = computed<InfoRow[]>(() => [
    { label: t('about.version'), value: text(build.frontend.version) },
    { label: t('about.commit'), value: text(build.frontend.commit) },
    { label: t('about.buildTime'), value: time(build.frontend.buildTime) },
  ])

  /**
   * 后端分区：请求失败（pending 会话 401 / DB 不可用 / 网络中断）时六行统一显示
   * 不可用占位（R5）——不报错、不弹 snackbar。数据来自 useBuildInfo 的单次缓存。
   *
   * 首次请求在途时不能声称"不可用"：那是把还没发生结论的读取说成失败，
   * 所以同一批行改挂加载占位。
   */
  const backendRows = computed<InfoRow[]>(() => {
    const info = build.backend.value
    const missing = build.loading.value ? t('common.loading') : t('about.unavailable')
    return [
      { label: t('about.version'), value: info ? text(info.version) : missing },
      { label: t('about.commit'), value: info ? text(info.commit) : missing },
      { label: t('about.buildTime'), value: info ? time(info.build_time) : missing },
      { label: t('about.goVersion'), value: info ? text(info.go_version) : missing },
      { label: t('about.platform'), value: info ? text(info.platform) : missing },
      { label: t('about.cgoEnabled'), value: info ? t(info.cgo_enabled ? 'common.enabled' : 'common.disabled') : missing },
    ]
  })
</script>
