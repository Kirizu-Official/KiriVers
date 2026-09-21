<!-- UploadDialog.vue — 产物上传三通道（直传 / TUS 分片 / 预签名）+ 产物复用（POST /artifacts/reuse）+ 整线 zip -->
<template>
  <v-dialog v-model="model" max-width="680" persistent>
    <v-card :title="t('artifacts.uploadTo', { os, arch })">
      <v-card-text>
        <ErrorAlert :error="error" />

        <v-tabs v-model="mode" color="primary">
          <v-tab value="upload">{{ t('artifacts.upload') }}</v-tab>
          <v-tab value="reuse">{{ t('artifacts.reuseMode') }}</v-tab>
        </v-tabs>

        <v-window v-model="mode" class="mt-3">
          <!-- 上传模式：三通道 + 文件 + 整线 zip -->
          <v-window-item value="upload">
            <v-radio-group v-model="channel" density="compact" inline :label="t('artifacts.channel')">
              <v-radio :label="t('artifacts.channelDirect')" value="direct" />
              <v-radio :label="t('artifacts.channelTus')" value="tus" />
              <v-radio :label="t('artifacts.channelPresign')" value="presign" />
            </v-radio-group>

            <v-file-input v-model="file" :label="t('artifacts.file')" prepend-icon="" prepend-inner-icon="mdi-paperclip" />

            <v-divider class="my-3" />
            <div class="text-subtitle-2 mb-2">{{ t('artifacts.buildArchive') }}</div>

            <div class="d-flex align-center ga-2">
              <v-file-input
                v-model="zipFile"
                class="flex-grow-1"
                density="compact"
                :label="t('artifacts.bundleFile')"
                prepend-icon=""
                prepend-inner-icon="mdi-folder-zip-outline"
              />

              <v-btn
                color="accent"
                :disabled="!zipFile"
                :loading="zipLoading"
                variant="tonal"
                @click="uploadZip"
              >
                {{ t('common.confirm') }}
              </v-btn>
            </div>

            <v-divider class="my-3" />

            <v-row dense>
              <v-col v-if="channel === 'tus'" cols="12" md="6">
                <v-text-field v-model.number="chunkMiB" :label="t('artifacts.chunkSize')" type="number" />
              </v-col>

              <v-col cols="12">
                <v-switch
                  v-model="precomputeSha"
                  color="primary"
                  density="compact"
                  hide-details
                  :label="t('artifacts.sha256Precompute')"
                />
              </v-col>
            </v-row>

            <div v-if="upload.state.phase === 'hashing'" class="text-caption text-info mt-2">
              {{ t('artifacts.hashing') }}
            </div>

            <UploadProgress
              :active="upload.state.phase === 'uploading' || upload.state.phase === 'hashing'"
              :bytes-per-sec="upload.state.bytesPerSec"
              :extra="upload.state.phase === 'uploading' && upload.state.message ? t('artifacts.resumable', { offset: upload.state.message }) : ''"
              :loaded="upload.state.loaded"
              :progress="upload.state.progress"
              :total="upload.state.total"
            />

            <div v-if="upload.state.phase === 'done'" class="text-caption text-success mt-2">✓ 100%</div>

            <UploadProgress
              :active="zipLoading"
              :bytes-per-sec="zipProgress.bytesPerSec"
              :loaded="zipProgress.loaded"
              :progress="zipProgress.progress"
              :total="zipProgress.total"
            />
          </v-window-item>

          <!-- 复用模式：手输来源（artifact_id / sha256 二选一），服务端零拷贝 -->
          <v-window-item value="reuse">
            <v-row dense>
              <v-col cols="12" md="6">
                <v-select
                  v-model="sourceType"
                  density="compact"
                  :items="sourceTypeItems"
                  :label="t('artifacts.reuseSource')"
                />
              </v-col>

              <v-col cols="12">
                <v-text-field
                  v-model="sourceValue"
                  :hint="t('artifacts.reuseSourceHint')"
                  :label="sourceType === 'sha256' ? t('artifacts.reuseSourceSha256') : t('artifacts.reuseSourceId')"
                  persistent-hint
                />
              </v-col>
            </v-row>

            <v-divider class="my-3" />

            <v-progress-linear
              v-if="reusing"
              class="mt-1"
              color="primary"
              height="8"
              indeterminate
              rounded
            />
          </v-window-item>
        </v-window>

        <v-divider class="my-3" />

        <v-row dense>
          <v-col cols="12" md="6">
            <v-select v-model="hwRev" clearable :items="hwRevOptions" :label="t('artifacts.hwRev')" />
          </v-col>
        </v-row>
      </v-card-text>

      <v-card-actions>
        <v-spacer />

        <v-btn v-if="upload.isBusy.value" color="error" variant="text" @click="cancel">
          {{ t('common.cancel') }}
        </v-btn>

        <v-btn v-else variant="text" @click="model = false">{{ t('common.close') }}</v-btn>

        <v-btn
          v-if="!upload.isBusy.value"
          color="primary"
          :disabled="!canSubmit"
          :loading="reusing"
          @click="start"
        >
          {{ t('artifacts.start') }}
        </v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>

<script lang="ts" setup>
  import type { Artifact } from '@/api/generated'
  import { computed, onMounted, reactive, ref, watch } from 'vue'
  import { useI18n } from 'vue-i18n'
  import { buildLineArchive, listHwRevs, reuseArtifact } from '@/api/generated'
  import ErrorAlert from '@/components/ErrorAlert.vue'
  import UploadProgress from '@/components/UploadProgress.vue'
  import { createByteProgressTracker, useUpload } from '@/composables/useUpload'
  import { useSnackbarStore } from '@/stores/snackbar'

  const props = defineProps<{
    projectRef: string
    version: string
    os: string
    arch: string
  }>()

  // uploaded 携带新建产物（复用/上传/归档的返回值）与目标线，父页据此维护会话内产物表
  const emit = defineEmits<{ uploaded: [artifact: Artifact | null, os: string, arch: string] }>()
  const model = defineModel<boolean>({ default: false })

  const { t } = useI18n()
  const upload = useUpload()
  const snackbar = useSnackbarStore()

  const mode = ref<'upload' | 'reuse'>('upload')
  const channel = ref<'direct' | 'tus' | 'presign'>('direct')
  const file = ref<File | null>(null)
  const zipFile = ref<File | null>(null)
  const zipLoading = ref(false)
  const zipProgress = reactive({ progress: 0, loaded: 0, total: 0, bytesPerSec: 0 })
  const zipTracker = createByteProgressTracker()
  const hwRev = ref<string | null>(null)
  const chunkMiB = ref(8)
  const precomputeSha = ref(false)
  const sourceType = ref<'artifact_id' | 'sha256'>('artifact_id')
  const sourceValue = ref('')
  const reusing = ref(false)
  const error = ref<unknown>(null)

  const hwRevOptions = ref<string[]>([])
  const sourceTypeItems = computed(() => [
    { title: t('artifacts.reuseSourceId'), value: 'artifact_id' },
    { title: t('artifacts.reuseSourceSha256'), value: 'sha256' },
  ])

  const canSubmit = computed(() => (mode.value === 'reuse' ? Boolean(sourceValue.value.trim()) : Boolean(file.value)))

  onMounted(async () => {
    try {
      const { data: revs } = await listHwRevs({ path: { project_ref: props.projectRef } })
      hwRevOptions.value = (revs?.hw_revs ?? []).flatMap(r => r.slug ? [r.slug] : [])
    } catch {
      hwRevOptions.value = []
    }
  })

  watch(model, open => {
    if (open) {
      error.value = null
      upload.reset()
    }
  })

  async function start (): Promise<void> {
    error.value = null
    try {
      if (mode.value === 'reuse') {
        if (!sourceValue.value.trim()) return
        reusing.value = true
        // 复用提交：os/arch 取对话框打开时的线（与上传上下文一致，不可改）
        const { data } = await reuseArtifact({
          path: { project_ref: props.projectRef, version: props.version },
          body: {
            os: props.os,
            arch: props.arch,
            hw_rev: hwRev.value ?? undefined,
            source: sourceType.value === 'sha256'
              ? { sha256: sourceValue.value.trim() }
              : { artifact_id: sourceValue.value.trim() },
          },
        })
        const reused = data?.artifacts?.[0]
        if (!reused) {
          throw new Error('reuse: empty artifacts')
        }
        reusing.value = false
        snackbar.show(t('artifacts.reuseDone') + ' ✓')
        emit('uploaded', reused, props.os, props.arch)
        model.value = false
        sourceValue.value = ''
        return
      }
      if (!file.value) return
      const uploaded = await upload.run({
        channel: channel.value,
        file: file.value,
        projectRef: props.projectRef,
        version: props.version,
        os: props.os,
        arch: props.arch,
        hwRev: hwRev.value ?? undefined,
        chunkMiB: chunkMiB.value,
        precomputeSha: precomputeSha.value,
      })
      emit('uploaded', uploaded, props.os, props.arch)
      model.value = false
      file.value = null
    } catch (error_) {
      error.value = error_
    } finally {
      reusing.value = false
    }
  }

  async function uploadZip (): Promise<void> {
    if (!zipFile.value) return
    zipLoading.value = true
    error.value = null
    zipProgress.progress = 0
    zipProgress.loaded = 0
    zipProgress.total = zipFile.value.size
    zipProgress.bytesPerSec = 0
    zipTracker.resetSpeed()
    try {
      const { data: artifact } = await buildLineArchive({
        path: {
          project_ref: props.projectRef,
          version: props.version,
          os: props.os,
          arch: props.arch,
        },
        body: zipFile.value,
        onUploadProgress: event => {
          const loaded = event.loaded ?? 0
          const total = event.total ?? zipFile.value?.size ?? 0
          zipProgress.loaded = loaded
          zipProgress.total = total
          zipProgress.bytesPerSec = zipTracker.sample(loaded)
          zipProgress.progress = total ? Math.round((loaded / total) * 100) : 0
        },
      })
      emit('uploaded', artifact ?? null, props.os, props.arch)
      model.value = false
      zipFile.value = null
    } catch (error_) {
      error.value = error_
    } finally {
      zipLoading.value = false
    }
  }

  function cancel (): void {
    void upload.cancel({
      channel: channel.value,
      file: file.value ?? new File([], 'x'),
      projectRef: props.projectRef,
      version: props.version,
      os: props.os,
      arch: props.arch,
    })
  }
</script>
