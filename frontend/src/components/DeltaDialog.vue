<!-- DeltaDialog.vue — 差量生成：算法 + 源版本 → 202 任务 -->
<template>
  <v-dialog v-model="model" max-width="560">
    <v-card :title="`${t('artifacts.generateDelta')} · ${os}/${arch}`">
      <v-card-text>
        <ErrorAlert :error="error" />
        <v-select v-model="algo" :items="['hdiffpatch', 'bsdiff', 'xdelta3']" :label="t('matrix.deltaAlgo')" />

        <v-select
          v-model="source"
          :hint="t('artifacts.deltaHint')"
          :items="sourceOptions"
          :label="t('artifacts.sourceVersion')"
          persistent-hint
        />
      </v-card-text>

      <v-card-actions>
        <v-spacer />
        <v-btn variant="text" @click="model = false">{{ t('common.cancel') }}</v-btn>

        <v-btn color="primary" :disabled="!source" :loading="submitting" @click="submit">
          {{ t('artifacts.generateDelta') }}
        </v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>

<script lang="ts" setup>
  import type { Version } from '@/api/generated'
  import { computed, onMounted, ref, watch } from 'vue'
  import { useI18n } from 'vue-i18n'
  import { createDeltaJob, listVersions } from '@/api/generated'
  import ErrorAlert from '@/components/ErrorAlert.vue'
  import { useJobsStore } from '@/stores/jobs'
  import { useSnackbarStore } from '@/stores/snackbar'

  const props = defineProps<{ projectRef: string, version: string, os: string, arch: string }>()
  const model = defineModel<boolean>({ default: false })
  const emit = defineEmits<{ changed: [] }>()

  const { t } = useI18n()
  const jobs = useJobsStore()
  const snackbar = useSnackbarStore()

  const algo = ref('hdiffpatch')
  const source = ref<string | null>(null)
  const versions = ref<Version[]>([])
  const submitting = ref(false)
  const error = ref<unknown>(null)

  const sourceOptions = computed(() =>
    versions.value.flatMap(v => {
      const ref = v.version_semver || (v.version_integer == null ? v.id : String(v.version_integer))
      if (!ref || ref === props.version) {
        return []
      }
      return [{ title: ref, value: ref }]
    }),
  )

  onMounted(async () => {
    try {
      const { data } = await listVersions({ path: { project_ref: props.projectRef } })
      versions.value = data?.versions ?? []
    } catch {
      versions.value = []
    }
  })

  watch(model, open => {
    if (open) {
      error.value = null
      source.value = null
    }
  })

  async function submit (): Promise<void> {
    if (!source.value) return
    submitting.value = true
    error.value = null
    try {
      const { data } = await createDeltaJob({
        path: { project_ref: props.projectRef, version: props.version },
        body: {
          source_version: source.value,
          os: props.os,
          arch: props.arch,
          algo: algo.value as 'hdiffpatch' | 'bsdiff' | 'xdelta3',
        },
      })
      const jobId = data?.job_id
      if (!jobId) return
      jobs.track(jobId, `delta ${props.os}/${props.arch}`, 'delta', props.projectRef)
      snackbar.show(t('release.submitHint'))
      model.value = false
      emit('changed')
    } catch (error_) {
      error.value = error_
    } finally {
      submitting.value = false
    }
  }
</script>
