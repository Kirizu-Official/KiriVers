<!-- AnnouncementPreviewDialog.vue — 客户端公告 GET 预览（client 面，非管理端模拟） -->
<template>
  <v-dialog v-model="model" max-width="720">
    <v-card :title="t('announcements.previewTitle')">
      <v-card-text>
        <ErrorAlert :error="error" />
        <div class="text-body-2 text-medium-emphasis mb-4">{{ t('announcements.previewHint') }}</div>

        <v-row dense>
          <v-col cols="12" md="6">
            <v-combobox
              v-model="version"
              clearable
              :items="versionOptions"
              :label="t('announcements.version')"
            />
          </v-col>

          <v-col cols="12" md="3">
            <v-combobox
              v-model="os"
              clearable
              :items="osOptions"
              :label="t('announcements.os')"
            />
          </v-col>

          <v-col cols="12" md="3">
            <v-combobox
              v-model="arch"
              clearable
              :items="archOptions"
              :label="t('announcements.arch')"
            />
          </v-col>

          <v-col cols="12" md="6">
            <v-select
              v-model="locale"
              clearable
              item-title="title"
              item-value="code"
              :items="localeItems"
              :label="t('announcements.previewLocale')"
            />
          </v-col>
        </v-row>

        <div v-if="status !== null" class="mt-2">
          <v-chip class="mb-2" color="info" label>{{ t('announcements.previewStatus') }} {{ status }}</v-chip>
          <pre class="json-preview">{{ pretty }}</pre>
        </div>
      </v-card-text>

      <v-card-actions>
        <v-spacer />
        <v-btn variant="text" @click="model = false">{{ t('common.close') }}</v-btn>
        <v-btn color="primary" :loading="loading" @click="run">{{ t('announcements.previewRun') }}</v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>

<script lang="ts" setup>
  import { computed, ref, watch } from 'vue'
  import { useI18n } from 'vue-i18n'
  import { listMatrix, listVersions } from '@/api/generated'
  import { listClientAnnouncements } from '@/api/generated-client'
  import ErrorAlert from '@/components/ErrorAlert.vue'
  import { languageLabel } from '@/composables/useProjectLanguages'

  const props = withDefaults(defineProps<{
    projectRef: string
    languages?: Array<{ code?: string, display_name?: string, is_default?: boolean }>
  }>(), { languages: () => [] })
  const model = defineModel<boolean>({ default: false })

  const { t } = useI18n()
  const version = ref<string | null>(null)
  const os = ref<string | null>(null)
  const arch = ref<string | null>(null)
  const locale = ref<string | null>(null)
  const status = ref<number | null>(null)
  const payload = ref<unknown>(null)
  const loading = ref(false)
  const error = ref<unknown>(null)
  const versionOptions = ref<string[]>([])
  const osOptions = ref<string[]>([])
  const archOptions = ref<string[]>([])

  const pretty = computed(() => JSON.stringify(payload.value, null, 2))

  const localeItems = computed(() => (props.languages ?? []).map(row => ({
    title: languageLabel(row),
    code: row.code ?? '',
  })).filter(item => item.code))

  function comboValue (value: string | null): string | undefined {
    const trimmed = String(value ?? '').trim()
    return trimmed === '' ? undefined : trimmed
  }

  async function loadOptions (): Promise<void> {
    try {
      const [{ data: versions }, { data: matrix }] = await Promise.all([
        listVersions({ path: { project_ref: props.projectRef } }),
        listMatrix({ path: { project_ref: props.projectRef } }),
      ])
      versionOptions.value = (versions?.versions ?? []).map(item => (
        item.version_semver ?? String(item.version_integer ?? item.id ?? '')
      )).filter(Boolean)
      const osSet = new Set<string>()
      const archSet = new Set<string>()
      for (const row of matrix?.matrix ?? []) {
        if (row.os) osSet.add(row.os)
        if (row.arch) archSet.add(row.arch)
      }
      osOptions.value = Array.from(osSet)
      archOptions.value = Array.from(archSet)
    } catch {
      versionOptions.value = []
      osOptions.value = []
      archOptions.value = []
    }
  }

  watch(model, open => {
    if (open) {
      status.value = null
      payload.value = null
      error.value = null
      locale.value = null
      void loadOptions()
    }
  }, { immediate: true })

  async function run (): Promise<void> {
    loading.value = true
    error.value = null
    try {
      const response = await listClientAnnouncements({
        path: { project_ref: props.projectRef },
        query: {
          version: comboValue(version.value),
          os: comboValue(os.value),
          arch: comboValue(arch.value),
          locale: comboValue(locale.value),
        },
        validateStatus: s => (s >= 200 && s < 300) || s === 304,
        throwOnError: true,
      })
      status.value = response.status
      payload.value = response.data ?? null
    } catch (error_) {
      error.value = error_
      status.value = null
      payload.value = null
    } finally {
      loading.value = false
    }
  }
</script>

<style scoped>
.json-preview {
  max-height: 320px;
  overflow: auto;
  padding: 12px;
  border-radius: 8px;
  font-size: 12px;
  line-height: 1.5;
  background: rgba(var(--v-theme-on-surface), 0.06);
}
</style>
