<!-- CheckPreviewDialog.vue — 客户端 check 预览（公开端点） -->
<template>
  <v-dialog v-model="model" max-width="720">
    <v-card :title="t('versions.checkPreview')">
      <v-card-text>
        <ErrorAlert :error="error" />

        <v-row dense>
          <v-col cols="12" md="6"><v-text-field v-model="currentVersion" :label="t('versions.currentVersion')" /></v-col>
          <v-col cols="12" md="3"><v-text-field v-model="os" label="OS" /></v-col>
          <v-col cols="12" md="3"><v-text-field v-model="arch" label="ARCH" /></v-col>

          <v-col cols="12" md="6">
            <v-select v-model="channel" :items="['stable', 'beta', 'alpha']" :label="t('versions.channel')" />
          </v-col>
        </v-row>

        <div v-if="result" class="mt-2">
          <div class="d-flex ga-2 mb-2">
            <v-chip color="success" label>{{ t('versions.statusPublished') }}</v-chip>
            <v-chip v-if="result.is_mandatory" color="error" label>mandatory</v-chip>
            <v-chip v-if="result.is_downgrade" color="warning" label>downgrade</v-chip>
            <v-chip color="info" label>{{ result.target_channel }}</v-chip>
          </div>

          <pre class="json-preview">{{ pretty }}</pre>
        </div>

        <v-alert v-else-if="noUpdate" class="mt-2" density="compact" type="info">
          {{ t('versions.noUpdate') }}
        </v-alert>
      </v-card-text>

      <v-card-actions>
        <v-spacer />
        <v-btn variant="text" @click="model = false">{{ t('common.close') }}</v-btn>
        <v-btn color="primary" :loading="loading" @click="run">{{ t('versions.checkPreview') }}</v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>

<script lang="ts" setup>
  import type { UpdateCheck200 } from '@/api/generated-client'
  import { computed, ref, watch } from 'vue'
  import { useI18n } from 'vue-i18n'
  import { checkUpdate } from '@/api/generated-client'
  import ErrorAlert from '@/components/ErrorAlert.vue'

  const props = defineProps<{ projectRef: string, suggestedVersion: string, suggestedOs: string, suggestedArch: string }>()
  const model = defineModel<boolean>({ default: false })

  const { t } = useI18n()
  const currentVersion = ref(props.suggestedVersion)
  const os = ref(props.suggestedOs)
  const arch = ref(props.suggestedArch)
  const channel = ref('stable')
  const result = ref<UpdateCheck200 | null>(null)
  const noUpdate = ref(false)
  const loading = ref(false)
  const error = ref<unknown>(null)

  const pretty = computed(() => JSON.stringify(result.value, null, 2))

  watch(model, open => {
    if (open) {
      currentVersion.value = props.suggestedVersion
      os.value = props.suggestedOs
      arch.value = props.suggestedArch
      result.value = null
      noUpdate.value = false
    }
  })

  async function run (): Promise<void> {
    loading.value = true
    error.value = null
    noUpdate.value = false
    try {
      const response = await checkUpdate({
        path: { project_ref: props.projectRef },
        body: {
          current_version: currentVersion.value,
          os: os.value,
          arch: arch.value,
          channel: channel.value,
        },
        validateStatus: status => (status >= 200 && status < 300) || status === 304,
      })
      const payload = response.data
      const empty = response.status === 204 || response.status === 304
        || payload == null
        || typeof payload !== 'object'
      if (empty) {
        result.value = null
        noUpdate.value = true
        return
      }
      result.value = payload
      noUpdate.value = false
    } catch (error_) {
      error.value = error_
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
