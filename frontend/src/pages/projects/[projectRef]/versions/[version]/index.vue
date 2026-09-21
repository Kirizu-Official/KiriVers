<!-- pages/projects/[projectRef]/versions/[version]/index.vue — 产物列表 -->
<template>
  <div>
    <v-row class="mb-4">
      <v-col cols="12">
        <v-card :title="t('charts.artifactSize')">
          <v-card-text>
            <EChart :height="240" :option="sizeBar" />
          </v-card-text>
        </v-card>
      </v-col>
    </v-row>

    <v-card :title="t('artifacts.artifactList')">
      <template #append>
        <v-btn
          color="primary"
          prepend-icon="mdi-plus"
          size="small"
          variant="tonal"
          @click="openCreateLine"
        >
          {{ t('versions.createLine') }}
        </v-btn>
      </template>

      <v-card-text>
        <ErrorAlert :error="pageError" />
        <EmptyState v-if="!loading && lines.length === 0" icon="mdi-package-variant-closed" :text="t('common.empty')" />

        <v-data-table
          v-else
          :headers="headers"
          :items="lines"
          items-key="id"
          :loading="loading"
        >
          <template #item.os="{ item }">
            <code>{{ item.os }}</code>
          </template>

          <template #item.arch="{ item }">
            <code>{{ item.arch }}</code>
          </template>

          <template #item.status="{ item }">
            <div class="d-flex ga-1 align-center flex-wrap">
              <StatusChip kind="line" :status="item.status ?? ''" />

              <v-chip
                v-if="item.packs_ready_at"
                color="success"
                size="small"
                variant="tonal"
              >
                {{ t('versions.packsReady') }}
              </v-chip>

              <v-chip
                v-else-if="item.status === 'ready'"
                color="warning"
                size="small"
                variant="tonal"
              >
                {{ t('versions.packsPending') }}
              </v-chip>
            </div>
          </template>

          <template #item.file_name="{ item }">
            {{ primaryArtifact(item)?.file_name ?? '—' }}
          </template>

          <template #item.size="{ item }">
            {{ formatBytes(primaryArtifact(item)?.size ?? 0) }}
          </template>

          <template #item.sha256="{ item }">
            <CopyField v-if="primaryArtifact(item)?.sha256" :value="primaryArtifact(item)?.sha256" />
            <span v-else>—</span>
          </template>

          <template #item.artifact_count="{ item }">
            {{ (item.artifacts ?? []).length }}
          </template>

          <template #item.updated_at="{ item }">
            <span class="text-body-2 text-medium-emphasis">{{ item.updated_at }}</span>
          </template>

          <template #item.actions="{ item }">
            <v-btn
              v-if="item.status !== 'ready'"
              color="success"
              size="small"
              variant="text"
              @click="lineAct(item, 'ready')"
            >
              {{ t('versions.ready') }}
            </v-btn>

            <v-btn
              v-if="item.status === 'ready'"
              color="warning"
              size="small"
              variant="text"
              @click="lineAct(item, 'yank')"
            >
              {{ t('versions.yank') }}
            </v-btn>

            <v-btn
              v-if="item.status === 'ready'"
              size="small"
              variant="text"
              @click="lineAct(item, 'disable')"
            >
              {{ t('versions.disable') }}
            </v-btn>

            <v-btn size="small" variant="text" @click="openUpload(item)">{{ t('artifacts.upload') }}</v-btn>
            <v-btn size="small" variant="text" @click="openManifest(item)">{{ t('artifacts.manifest') }}</v-btn>
            <v-btn size="small" variant="text" @click="openDelta(item)">{{ t('artifacts.delta') }}</v-btn>
          </template>
        </v-data-table>
      </v-card-text>
    </v-card>

    <v-dialog v-model="lineDialog" max-width="560">
      <v-card :title="t('versions.createLine')">
        <v-card-text>
          <ErrorAlert :error="lineFormError" />

          <v-select
            v-model="lineForm.pair"
            :hint="t('versions.matrixPairHint')"
            :items="pairOptions"
            :label="t('versions.matrixPair')"
            persistent-hint
            :rules="[requiredPair]"
          />

          <v-text-field
            v-model="lineForm.min_os"
            :hint="t('lines.minOsHint')"
            :label="t('lines.minOs')"
            persistent-hint
          />

          <v-text-field
            v-model.number="lineForm.min_api_level"
            :hint="t('lines.minApiLevelHint')"
            :label="t('lines.minApiLevel')"
            persistent-hint
            type="number"
          />

          <v-file-input
            v-model="lineFile"
            :label="t('artifacts.file')"
            prepend-icon=""
            prepend-inner-icon="mdi-paperclip"
          />

          <UploadProgress
            :active="upload.isBusy.value"
            :bytes-per-sec="upload.state.bytesPerSec"
            :loaded="upload.state.loaded"
            :progress="upload.state.progress"
            :total="upload.state.total"
          />
        </v-card-text>

        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="lineDialog = false">{{ t('common.cancel') }}</v-btn>
          <v-btn color="primary" :loading="lineSaving || upload.isBusy.value" @click="createLine">{{ t('common.create') }}</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <UploadDialog
      v-if="uploadLine"
      v-model="uploadOpen"
      :arch="uploadLine.arch ?? ''"
      :os="uploadLine.os ?? ''"
      :project-ref="projectRef"
      :version="versionParam"
      @uploaded="onUploaded"
    />

    <ManifestDialog
      v-if="manifestLine"
      v-model="manifestOpen"
      :arch="manifestLine.arch ?? ''"
      :os="manifestLine.os ?? ''"
      :project-ref="projectRef"
      :version="versionParam"
      @changed="refreshLines"
    />

    <DeltaDialog
      v-if="deltaLine"
      v-model="deltaOpen"
      :arch="deltaLine.arch ?? ''"
      :os="deltaLine.os ?? ''"
      :project-ref="projectRef"
      :version="versionParam"
      @changed="refreshLines"
    />
  </div>
</template>

<script lang="ts" setup>
  import type { Artifact, VersionLine } from '@/api/generated'
  import { computed, onMounted, reactive, ref, watch } from 'vue'
  import { useI18n } from 'vue-i18n'
  import { useRoute } from 'vue-router'
  import {
    createVersionLine,
    disableVersionLine,
    getVersionLineDefaults,
    listMatrix,
    listVersionLines,
    readyVersionLine,
    yankVersionLine,
  } from '@/api/generated'
  import CopyField from '@/components/CopyField.vue'
  import DeltaDialog from '@/components/DeltaDialog.vue'
  import EChart from '@/components/EChart.vue'
  import EmptyState from '@/components/EmptyState.vue'
  import ErrorAlert from '@/components/ErrorAlert.vue'
  import ManifestDialog from '@/components/ManifestDialog.vue'
  import StatusChip from '@/components/StatusChip.vue'
  import UploadDialog from '@/components/UploadDialog.vue'
  import UploadProgress from '@/components/UploadProgress.vue'
  import { barOption } from '@/composables/chartOptions'
  import { useChartTheme } from '@/composables/useChartTheme'
  import { useUpload } from '@/composables/useUpload'
  import { useSnackbarStore } from '@/stores/snackbar'
  import { formatBytes } from '@/utils/bytes'

  const { t } = useI18n()
  const route = useRoute('/projects/[projectRef]/versions/[version]/')
  const snackbar = useSnackbarStore()
  const chartTheme = useChartTheme()
  const upload = useUpload()

  const projectRef = computed(() => route.params.projectRef)
  const versionParam = computed(() => route.params.version)

  const lines = ref<VersionLine[]>([])
  const loading = ref(false)
  const pageError = ref<unknown>(null)
  const pairOptions = ref<Array<{ title: string, value: string }>>([])

  const lineDialog = ref(false)
  const lineSaving = ref(false)
  const lineFormError = ref<unknown>(null)
  const lineForm = reactive({ pair: '', min_os: '', min_api_level: undefined as number | undefined })
  const lineFile = ref<File | File[] | null>(null)

  const uploadOpen = ref(false)
  const manifestOpen = ref(false)
  const deltaOpen = ref(false)
  const uploadLine = ref<VersionLine | null>(null)
  const manifestLine = ref<VersionLine | null>(null)
  const deltaLine = ref<VersionLine | null>(null)

  const headers = computed(() => [
    { title: t('lines.os'), key: 'os' },
    { title: t('lines.arch'), key: 'arch' },
    { title: t('versions.status'), key: 'status' },
    { title: t('artifacts.fileName'), key: 'file_name' },
    { title: t('artifacts.size'), key: 'size' },
    { title: t('artifacts.sha256'), key: 'sha256' },
    { title: t('artifacts.count'), key: 'artifact_count' },
    { title: t('common.updated'), key: 'updated_at' },
    { title: t('common.actions'), key: 'actions', sortable: false, align: 'end' as const },
  ])

  const sizeBar = computed(() => barOption(
    chartTheme.value,
    lines.value.map(line => ({
      name: `${line.os ?? '?'}/${line.arch ?? '?'}`,
      count: primaryArtifact(line)?.size ?? 0,
    })),
  ))

  function primaryArtifact (line: VersionLine): Artifact | undefined {
    const arts = line.artifacts ?? []
    return arts.find(row => row.kind === 'full') ?? arts.find(row => row.kind === 'file') ?? arts[0]
  }

  function firstFile (value: File | File[] | null | undefined): File | null {
    if (Array.isArray(value)) return value[0] ?? null
    return value ?? null
  }

  function parsePair (value: string): { os: string, arch: string } | undefined {
    const [os, arch] = value.split('|')
    if (!os || !arch) return undefined
    return { os, arch }
  }

  function requiredPair (value: string): boolean | string {
    return Boolean(parsePair(value)) || t('common.required')
  }

  function openCreateLine (): void {
    lineForm.pair = pairOptions.value[0]?.value ?? ''
    lineForm.min_os = ''
    lineForm.min_api_level = undefined
    lineFile.value = null
    lineFormError.value = null
    lineDialog.value = true
  }

  async function refreshLines (): Promise<void> {
    loading.value = true
    pageError.value = null
    try {
      const { data } = await listVersionLines({
        path: { project_ref: projectRef.value, version: versionParam.value },
      })
      lines.value = data?.lines ?? []
    } catch (error) {
      pageError.value = error
    } finally {
      loading.value = false
    }
  }

  function onUploaded (_artifact: Artifact | null): void {
    void refreshLines()
  }

  function openUpload (line: VersionLine): void {
    uploadLine.value = line
    uploadOpen.value = true
  }

  function openManifest (line: VersionLine): void {
    manifestLine.value = line
    manifestOpen.value = true
  }

  function openDelta (line: VersionLine): void {
    deltaLine.value = line
    deltaOpen.value = true
  }

  async function lineAct (line: VersionLine, action: 'ready' | 'yank' | 'disable'): Promise<void> {
    pageError.value = null
    try {
      const path = {
        project_ref: projectRef.value,
        version: versionParam.value,
        os: line.os ?? '',
        arch: line.arch ?? '',
      }
      if (action === 'ready') await readyVersionLine({ path })
      if (action === 'yank') await yankVersionLine({ path })
      if (action === 'disable') await disableVersionLine({ path })
      snackbar.show(t(`versions.line${action.charAt(0).toUpperCase()}${action.slice(1)}`) + ' ✓')
      await refreshLines()
    } catch (error) {
      pageError.value = error
    }
  }

  async function createLine (): Promise<void> {
    const pair = parsePair(lineForm.pair)
    if (!pair) {
      lineFormError.value = new Error(t('common.required'))
      return
    }
    lineSaving.value = true
    lineFormError.value = null
    try {
      await createVersionLine({
        path: { project_ref: projectRef.value, version: versionParam.value },
        body: {
          os: pair.os,
          arch: pair.arch,
          min_os: lineForm.min_os || undefined,
          min_api_level: typeof lineForm.min_api_level === 'number' ? lineForm.min_api_level : undefined,
        },
      })
      const file = firstFile(lineFile.value)
      if (file) {
        await upload.run({
          channel: 'direct',
          file,
          projectRef: projectRef.value,
          version: versionParam.value,
          os: pair.os,
          arch: pair.arch,
        })
      }
      lineDialog.value = false
      await refreshLines()
    } catch (error) {
      lineFormError.value = error
    } finally {
      lineSaving.value = false
    }
  }

  watch(() => ({ pair: lineForm.pair, open: lineDialog.value }), async ({ pair, open }) => {
    if (!open) return
    const parsed = parsePair(pair)
    if (!parsed) return
    try {
      const { data } = await getVersionLineDefaults({
        path: { project_ref: projectRef.value, version: versionParam.value },
        query: parsed,
      })
      lineForm.min_os = data?.min_os ?? ''
      lineForm.min_api_level = data?.min_api_level ?? undefined
    } catch {
      // keep user input
    }
  })

  onMounted(async () => {
    await refreshLines()
    try {
      const { data } = await listMatrix({ path: { project_ref: projectRef.value } })
      pairOptions.value = (data?.matrix ?? []).flatMap(row => {
        if (!row.os || !row.arch) return []
        return [{ title: `${row.os} / ${row.arch}`, value: `${row.os}|${row.arch}` }]
      })
    } catch {
      pairOptions.value = []
    }
  })
</script>
