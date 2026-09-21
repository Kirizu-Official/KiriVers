<!-- IntegrityDialog.vue — 客户端完整性元数据预览（client 面 integrity 端点，§8） -->
<template>
  <v-dialog v-model="model" max-width="720">
    <v-card :title="t('integrity.title')">
      <v-card-text>
        <ErrorAlert :error="error" />

        <v-row dense>
          <v-col cols="12" md="4">
            <v-combobox
              v-model="os"
              clearable
              density="compact"
              :items="osOptions"
              :label="t('lines.os')"
            />
          </v-col>

          <v-col cols="12" md="4">
            <v-combobox
              v-model="arch"
              clearable
              density="compact"
              :items="archOptions"
              :label="t('lines.arch')"
            />
          </v-col>
        </v-row>

        <div v-if="result != null" class="mt-2">
          <pre class="json-preview">{{ pretty }}</pre>
        </div>
      </v-card-text>

      <v-card-actions>
        <v-spacer />
        <v-btn variant="text" @click="model = false">{{ t('common.close') }}</v-btn>

        <v-btn color="primary" :disabled="!os || !arch" :loading="loading" @click="run">
          {{ t('integrity.preview') }}
        </v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>

<script lang="ts" setup>
  import { computed, ref, watch } from 'vue'
  import { useI18n } from 'vue-i18n'
  import { getIntegrity } from '@/api/generated-client'
  import ErrorAlert from '@/components/ErrorAlert.vue'

  const props = defineProps<{
    projectRef: string
    version: string
    suggestedOs: string
    suggestedArch: string
    /** os/arch 下拉选项：来自当前版本的版本线（可手输） */
    osOptions: string[]
    archOptions: string[]
  }>()
  const model = defineModel<boolean>({ default: false })

  const { t } = useI18n()
  const os = ref<string | null>(props.suggestedOs)
  const arch = ref<string | null>(props.suggestedArch)
  const result = ref<unknown>(null)
  const loading = ref(false)
  const error = ref<unknown>(null)

  const pretty = computed(() => JSON.stringify(result.value, null, 2))

  watch(model, open => {
    if (open) {
      os.value = props.suggestedOs
      arch.value = props.suggestedArch
      result.value = null
      error.value = null
    }
  })

  async function run (): Promise<void> {
    if (!os.value || !arch.value) return
    loading.value = true
    error.value = null
    try {
      const { data } = await getIntegrity({
        path: { project_ref: props.projectRef, version: props.version },
        query: {
          os: String(os.value).trim(),
          arch: String(arch.value).trim(),
        },
      })
      result.value = data
    } catch (error_) {
      error.value = error_
      result.value = null
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
