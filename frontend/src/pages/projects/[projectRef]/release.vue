<!-- pages/projects/[projectRef]/release.vue — 批量发版：已有版本 + zip + 可选发布 + 任务中心 -->
<template>
  <div>
    <PageHeader :description="t('release.description')" :title="t('release.title')" />

    <v-row>
      <v-col cols="12" lg="7">
        <v-card :title="t('release.title')">
          <v-card-text>
            <ErrorAlert :error="bundleError" />

            <div class="d-flex flex-wrap align-center ga-2">
              <v-select
                v-model="existingVersion"
                density="compact"
                hide-details
                :items="versionOptions"
                :label="t('versions.version')"
                style="max-width: 240px"
              />

              <v-file-input
                v-model="existingBundle"
                density="compact"
                hide-details
                :label="t('release.bundleFile')"
                prepend-icon=""
                prepend-inner-icon="mdi-folder-zip-outline"
                style="max-width: 320px"
              />

              <v-switch
                v-model="existingPublish"
                color="primary"
                density="compact"
                hide-details
                :label="t('release.publish')"
              />

              <v-btn
                color="primary"
                :disabled="!existingVersion || !firstFile(existingBundle)"
                :loading="bundleSubmitting"
                variant="tonal"
                @click="submitExisting"
              >
                {{ t('release.start') }}
              </v-btn>
            </div>

            <UploadProgress
              :active="bundleSubmitting"
              :bytes-per-sec="bundleProgress.bytesPerSec"
              :loaded="bundleProgress.loaded"
              :progress="bundleProgress.progress"
              :total="bundleProgress.total"
            />

            <div class="text-caption text-medium-emphasis mt-3">{{ t('release.submitHint') }}</div>
          </v-card-text>
        </v-card>
      </v-col>

      <v-col cols="12" lg="5">
        <v-card prepend-icon="mdi-bell-outline" :title="t('release.taskCenter')">
          <v-card-text>
            <EmptyState v-if="jobs.jobs.length === 0" icon="mdi-bell-sleep-outline" :text="t('release.noJobs')" />

            <v-list v-else class="py-0" density="compact">
              <v-list-item v-for="job in jobs.jobs" :key="job.jobId" class="px-0">
                <template #prepend>
                  <v-icon :color="jobColor(job)" :icon="jobIcon(job)" />
                </template>

                <v-list-item-title>{{ job.label }}</v-list-item-title>

                <v-list-item-subtitle>
                  {{ statusText(job) }}
                  <template v-if="job.errorMessage"> — {{ job.errorMessage }}</template>
                </v-list-item-subtitle>

                <template #append>
                  <div style="width: 96px">
                    <v-progress-linear
                      :color="jobColor(job)"
                      height="6"
                      :model-value="job.progress"
                      rounded
                    />
                  </div>
                </template>
              </v-list-item>
            </v-list>

            <div class="text-caption text-medium-emphasis mt-3">{{ t('release.pollHint') }}</div>
          </v-card-text>
        </v-card>
      </v-col>
    </v-row>
  </div>
</template>

<script lang="ts" setup>
  import { computed, onMounted, reactive, ref } from 'vue'
  import { useI18n } from 'vue-i18n'
  import { useRoute } from 'vue-router'
  import { listVersions, uploadVersionBundle } from '@/api/generated'
  import EmptyState from '@/components/EmptyState.vue'
  import ErrorAlert from '@/components/ErrorAlert.vue'
  import PageHeader from '@/components/PageHeader.vue'
  import UploadProgress from '@/components/UploadProgress.vue'
  import { createByteProgressTracker } from '@/composables/useUpload'
  import { type TrackedJob, useJobsStore } from '@/stores/jobs'
  import { useSnackbarStore } from '@/stores/snackbar'

  const { t } = useI18n()
  const route = useRoute('/projects/[projectRef]/release')
  const jobs = useJobsStore()
  const snackbar = useSnackbarStore()

  const projectRef = computed(() => route.params.projectRef)

  const existingVersion = ref<string | null>(null)
  const existingBundle = ref<File | File[] | null>(null)
  const existingPublish = ref(false)
  const bundleSubmitting = ref(false)
  const bundleError = ref<unknown>(null)
  const bundleProgress = reactive({ progress: 0, loaded: 0, total: 0, bytesPerSec: 0 })
  const bundleTracker = createByteProgressTracker()
  const versionOptions = ref<Array<{ title: string, value: string }>>([])

  function firstFile (value: File | File[] | null | undefined): File | null {
    if (Array.isArray(value)) {
      return value[0] ?? null
    }
    return value ?? null
  }

  function jobIcon (job: TrackedJob): string {
    switch (job.status) {
      case 'succeeded': { return 'mdi-check-circle' }
      case 'failed': { return 'mdi-alert-circle' }
      case 'running': { return 'mdi-progress-clock' }
      default: { return 'mdi-timer-sand' }
    }
  }

  function jobColor (job: TrackedJob): string {
    switch (job.status) {
      case 'succeeded': { return 'success' }
      case 'failed': { return 'error' }
      case 'running': { return 'info' }
      default: { return 'info' }
    }
  }

  function statusText (job: TrackedJob): string {
    const map: Record<string, string> = {
      queued: t('release.jobQueued'),
      running: `${t('release.jobRunning')} ${job.progress}%`,
      succeeded: t('release.jobSucceeded'),
      failed: t('release.jobFailed'),
      unknown: '—',
    }
    return map[job.status ?? 'unknown'] ?? job.status
  }

  async function submitExisting (): Promise<void> {
    const file = firstFile(existingBundle.value)
    if (!existingVersion.value || !file) return
    bundleSubmitting.value = true
    bundleError.value = null
    bundleProgress.progress = 0
    bundleProgress.loaded = 0
    bundleProgress.total = file.size
    bundleProgress.bytesPerSec = 0
    bundleTracker.resetSpeed()
    try {
      const { data } = await uploadVersionBundle({
        path: { project_ref: projectRef.value, version: existingVersion.value },
        body: file,
        query: existingPublish.value ? { publish: 'true' } : undefined,
        onUploadProgress: event => {
          const loaded = event.loaded ?? 0
          const total = event.total ?? file.size
          bundleProgress.loaded = loaded
          bundleProgress.total = total
          bundleProgress.bytesPerSec = bundleTracker.sample(loaded)
          bundleProgress.progress = total ? Math.round((loaded / total) * 100) : 0
        },
      })
      const jobId = data?.job_id
      if (!jobId) return
      jobs.track(jobId, `bundle ${existingVersion.value}`, 'bundle', projectRef.value)
      snackbar.show(t('release.submitHint'))
      existingBundle.value = null
    } catch (error_) {
      bundleError.value = error_
    } finally {
      bundleSubmitting.value = false
    }
  }

  onMounted(async () => {
    try {
      const { data } = await listVersions({ path: { project_ref: projectRef.value } })
      versionOptions.value = (data?.versions ?? []).map(v => {
        const value = v.version_semver ?? String(v.version_integer ?? v.id ?? '')
        return { title: value, value }
      })
    } catch {
      versionOptions.value = []
    }
  })
</script>
