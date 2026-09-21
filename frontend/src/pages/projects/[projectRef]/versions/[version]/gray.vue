<!-- pages/projects/[projectRef]/versions/[version]/gray.vue — 增量灰度：旋钮、白名单、是否已更新 -->
<template>
  <div>
    <ErrorAlert :error="error" />

    <v-row class="mb-2" dense>
      <v-col
        v-for="tile in kpiTiles"
        :key="tile.label"
        cols="6"
        md="3"
      >
        <v-card variant="tonal">
          <v-card-text class="py-3">
            <div class="text-caption text-medium-emphasis">{{ tile.label }}</div>
            <div class="text-h6">{{ tile.value }}</div>
          </v-card-text>
        </v-card>
      </v-col>
    </v-row>

    <v-card class="mb-4" :title="t('gray.knobs')">
      <v-card-text>
        <v-row dense>
          <v-col cols="12" md="4">
            <v-slider
              v-model="knobs.start"
              :disabled="Boolean(status?.is_critical)"
              :hint="t('gray.startHint')"
              :label="t('gray.start')"
              :max="100"
              :min="0"
              persistent-hint
              thumb-label
            />
          </v-col>

          <v-col cols="12" md="4">
            <v-slider
              v-model="knobs.step"
              :disabled="Boolean(status?.is_critical)"
              :hint="t('gray.stepHint')"
              :label="t('gray.step')"
              :max="100"
              :min="0"
              persistent-hint
              thumb-label
            />
          </v-col>

          <v-col cols="12" md="4">
            <v-text-field
              v-model.number="knobs.interval"
              :disabled="Boolean(status?.is_critical)"
              :hint="t('gray.intervalHint')"
              :label="t('gray.interval')"
              persistent-hint
              type="number"
            />
          </v-col>
        </v-row>

        <div class="d-flex flex-wrap ga-2 mt-2">
          <v-btn color="primary" :disabled="Boolean(status?.is_critical)" :loading="saving" @click="saveKnobs">
            {{ t('common.save') }}
          </v-btn>

          <v-btn
            color="success"
            :disabled="Boolean(status?.completed) || Boolean(status?.is_critical)"
            :loading="completing"
            variant="tonal"
            @click="completeNow"
          >
            {{ t('gray.complete') }}
          </v-btn>
        </div>

        <v-alert
          v-if="status?.is_critical"
          class="mt-4"
          density="compact"
          type="warning"
        >
          {{ t('gray.criticalForbidden') }}
        </v-alert>
      </v-card-text>
    </v-card>

    <v-row class="mb-4">
      <v-col cols="12" md="6">
        <v-card :title="t('charts.grayCoverage')">
          <v-card-text><EChart :option="coverageLine" /></v-card-text>
        </v-card>
      </v-col>

      <v-col cols="12" md="6">
        <v-card :title="t('charts.grayUpdated')">
          <v-card-text><EChart :option="updatedPie" /></v-card-text>
        </v-card>
      </v-col>

      <v-col cols="12" md="6">
        <v-card :title="t('charts.graySource')">
          <v-card-text><EChart :option="sourcePie" /></v-card-text>
        </v-card>
      </v-col>

      <v-col cols="12" md="6">
        <v-card :title="t('charts.os')">
          <v-card-text><EChart :option="osPie" /></v-card-text>
        </v-card>
      </v-col>
    </v-row>

    <v-card :title="t('gray.allowlist')">
      <template #append>
        <v-btn prepend-icon="mdi-account-plus" size="small" variant="tonal" @click="pickerOpen = true">
          {{ t('gray.addFromRegistry') }}
        </v-btn>
      </template>

      <v-card-text>
        <v-data-table :headers="headers" :items="grayClients" items-key="id" :loading="loading">
          <template #item.updated="{ item }">
            <v-chip :color="item.updated ? 'success' : 'warning'" size="small" variant="tonal">
              {{ item.updated ? t('common.yes') : t('common.no') }}
            </v-chip>
          </template>

          <template #item.source="{ item }">
            {{ item.source === 'manual' ? t('gray.sourceManual') : t('gray.sourceAuto') }}
          </template>

          <template #item.custom="{ item }">
            <code class="text-caption">{{ formatCustom(item.custom) }}</code>
          </template>

          <template #item.actions="{ item }">
            <v-btn color="error" size="small" variant="text" @click="removeOne(item.id)">
              {{ t('common.delete') }}
            </v-btn>
          </template>
        </v-data-table>
      </v-card-text>
    </v-card>

    <v-dialog v-model="pickerOpen" max-width="840">
      <v-card :title="t('gray.addFromRegistry')">
        <v-card-text>
          <v-text-field
            v-model="pickerQ"
            class="mb-3"
            clearable
            density="compact"
            hide-details
            :label="t('common.search')"
            prepend-inner-icon="mdi-magnify"
            @keyup.enter="loadPicker"
          />

          <v-data-table
            v-model="picked"
            :headers="pickerHeaders"
            item-value="id"
            :items="pickerItems"
            items-key="id"
            :loading="pickerLoading"
            show-select
          >
            <template #item.custom="{ item }">
              <code class="text-caption">{{ formatCustom(item.custom) }}</code>
            </template>
          </v-data-table>
        </v-card-text>

        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="pickerOpen = false">{{ t('common.cancel') }}</v-btn>

          <v-btn color="primary" :disabled="picked.length === 0" :loading="adding" @click="addPicked">
            {{ t('gray.addSelected') }}
          </v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>
  </div>
</template>

<script lang="ts" setup>
  import type { Client, GrayClientRow, GraySeriesPoint, GrayStatus } from '@/api/generated'
  import { computed, onMounted, reactive, ref, watch } from 'vue'
  import { useI18n } from 'vue-i18n'
  import { useRoute } from 'vue-router'
  import {
    addVersionGrayAllowlist,
    completeVersionGray,
    deleteVersionGrayAllowlist,
    getVersionGray,
    listProjectClients,
    listVersionGrayClients,
    listVersionGraySeries,
    patchVersionGray,
  } from '@/api/generated'
  import EChart from '@/components/EChart.vue'
  import ErrorAlert from '@/components/ErrorAlert.vue'
  import { lineSeriesOption, pieOption } from '@/composables/chartOptions'
  import { useChartTheme } from '@/composables/useChartTheme'
  import { useSnackbarStore } from '@/stores/snackbar'

  const { t } = useI18n()
  const route = useRoute('/projects/[projectRef]/versions/[version]/gray')
  const snackbar = useSnackbarStore()
  const chartTheme = useChartTheme()

  const projectRef = computed(() => route.params.projectRef)
  const versionParam = computed(() => route.params.version)

  const status = ref<GrayStatus | null>(null)
  const grayClients = ref<GrayClientRow[]>([])
  const series = ref<GraySeriesPoint[]>([])
  const loading = ref(false)
  const saving = ref(false)
  const completing = ref(false)
  const adding = ref(false)
  const error = ref<unknown>(null)
  const knobs = reactive({ start: 100, step: 0, interval: 3600 })

  const pickerOpen = ref(false)
  const pickerQ = ref('')
  const pickerItems = ref<Client[]>([])
  const pickerLoading = ref(false)
  const picked = ref<string[]>([])

  const kpiTiles = computed(() => [
    { label: t('gray.targetCoverage'), value: `${status.value?.target_percent ?? 0}%` },
    { label: t('gray.actualCoverage'), value: `${status.value?.actual_percent ?? 0}%` },
    { label: t('gray.allowlistedCount'), value: String(status.value?.allowlisted ?? 0) },
    { label: t('gray.updatedCount'), value: String(status.value?.updated ?? 0) },
  ])

  const coverageLine = computed(() => lineSeriesOption(
    chartTheme.value,
    series.value.map(row => row.taken_at ?? ''),
    [
      { name: t('gray.targetCoverage'), data: series.value.map(row => row.target_percent ?? 0), color: chartTheme.value.primary },
      {
        name: t('gray.actualCoverage'),
        data: series.value.map(row => {
          const n = row.n ?? 0
          if (n <= 0) return 100
          return Math.round(((row.allowlisted ?? 0) / n) * 100)
        }),
        color: chartTheme.value.success,
      },
    ],
  ))

  const updatedPie = computed(() => pieOption(chartTheme.value, [
    { name: t('gray.updatedYes'), count: grayClients.value.filter(row => row.updated).length },
    { name: t('gray.updatedNo'), count: grayClients.value.filter(row => !row.updated).length },
  ]))

  const sourcePie = computed(() => pieOption(chartTheme.value, [
    { name: t('gray.sourceAuto'), count: grayClients.value.filter(row => row.source === 'auto').length },
    { name: t('gray.sourceManual'), count: grayClients.value.filter(row => row.source === 'manual').length },
  ]))

  const osPie = computed(() => {
    const counts = new Map<string, number>()
    for (const row of grayClients.value) {
      const key = row.last_os || '—'
      counts.set(key, (counts.get(key) ?? 0) + 1)
    }
    return pieOption(chartTheme.value, Array.from(counts, ([name, count]) => ({ name, count })))
  })

  const headers = computed(() => [
    { title: t('clients.deviceHash'), key: 'device_hash' },
    { title: t('versions.version'), key: 'last_version' },
    { title: t('lines.os'), key: 'last_os' },
    { title: t('lines.arch'), key: 'last_arch' },
    { title: t('versions.channel'), key: 'last_channel' },
    { title: t('clients.lastIp'), key: 'last_ip' },
    { title: t('clients.lastCheck'), key: 'last_check_at' },
    { title: t('gray.updated'), key: 'updated' },
    { title: t('gray.source'), key: 'source' },
    { title: t('clients.custom'), key: 'custom' },
    { title: t('common.actions'), key: 'actions', sortable: false, align: 'end' as const },
  ])

  const pickerHeaders = computed(() => [
    { title: t('clients.deviceHash'), key: 'device_hash' },
    { title: t('versions.version'), key: 'last_version' },
    { title: t('lines.os'), key: 'last_os' },
    { title: t('lines.arch'), key: 'last_arch' },
    { title: t('clients.custom'), key: 'custom' },
  ])

  function formatCustom (value: unknown): string {
    if (value == null || (typeof value === 'object' && Object.keys(value as object).length === 0)) {
      return '—'
    }
    try {
      const text = JSON.stringify(value)
      return text.length > 80 ? `${text.slice(0, 80)}…` : text
    } catch {
      return '—'
    }
  }

  function applyStatus (next: GrayStatus | null): void {
    status.value = next
    knobs.start = next?.gray_start_percent ?? 100
    knobs.step = next?.gray_step_percent ?? 0
    knobs.interval = next?.gray_interval_seconds ?? 3600
  }

  async function loadAll (): Promise<void> {
    loading.value = true
    error.value = null
    const path = { project_ref: projectRef.value, version: versionParam.value }
    try {
      const [st, clients, snaps] = await Promise.all([
        getVersionGray({ path }),
        listVersionGrayClients({ path }),
        listVersionGraySeries({ path }),
      ])
      applyStatus(st.data ?? null)
      grayClients.value = clients.data?.clients ?? []
      series.value = snaps.data?.series ?? []
    } catch (error_) {
      error.value = error_
    } finally {
      loading.value = false
    }
  }

  async function saveKnobs (): Promise<void> {
    saving.value = true
    error.value = null
    try {
      const { data } = await patchVersionGray({
        path: { project_ref: projectRef.value, version: versionParam.value },
        body: {
          gray_start_percent: Math.round(knobs.start),
          gray_step_percent: Math.round(knobs.step),
          gray_interval_seconds: Math.round(Number(knobs.interval)) || 3600,
        },
      })
      applyStatus(data ?? null)
      snackbar.show(t('common.save') + ' ✓')
      await loadAll()
    } catch (error_) {
      error.value = error_
    } finally {
      saving.value = false
    }
  }

  async function completeNow (): Promise<void> {
    completing.value = true
    error.value = null
    try {
      const { data } = await completeVersionGray({
        path: { project_ref: projectRef.value, version: versionParam.value },
      })
      applyStatus(data ?? null)
      snackbar.show(t('gray.complete') + ' ✓')
      await loadAll()
    } catch (error_) {
      error.value = error_
    } finally {
      completing.value = false
    }
  }

  async function loadPicker (): Promise<void> {
    pickerLoading.value = true
    try {
      const { data } = await listProjectClients({
        path: { project_ref: projectRef.value },
        query: { q: pickerQ.value.trim() || undefined, limit: 100 },
      })
      pickerItems.value = data?.clients ?? []
    } catch (error_) {
      error.value = error_
    } finally {
      pickerLoading.value = false
    }
  }

  async function addPicked (): Promise<void> {
    if (picked.value.length === 0) return
    adding.value = true
    error.value = null
    try {
      await addVersionGrayAllowlist({
        path: { project_ref: projectRef.value, version: versionParam.value },
        body: { client_ids: picked.value },
      })
      snackbar.show(t('gray.addSelected') + ' ✓')
      pickerOpen.value = false
      picked.value = []
      await loadAll()
    } catch (error_) {
      error.value = error_
    } finally {
      adding.value = false
    }
  }

  async function removeOne (id: string | undefined): Promise<void> {
    if (!id) return
    error.value = null
    try {
      await deleteVersionGrayAllowlist({
        path: { project_ref: projectRef.value, version: versionParam.value },
        body: { client_ids: [id] },
      })
      await loadAll()
    } catch (error_) {
      error.value = error_
    }
  }

  watch(pickerOpen, open => {
    if (open) {
      picked.value = []
      void loadPicker()
    }
  })

  onMounted(() => {
    void loadAll()
  })
  watch(versionParam, () => {
    void loadAll()
  })
</script>
