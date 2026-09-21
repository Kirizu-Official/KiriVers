<!-- pages/projects/[projectRef]/overview.vue — 项目概览图表 -->
<template>
  <div>
    <v-row v-if="project">
      <v-col cols="12">
        <v-row dense>
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
      </v-col>

      <v-col cols="12" lg="4" md="6">
        <v-card :title="t('charts.versionPie')">
          <v-card-text>
            <EChart :option="versionPie" />
          </v-card-text>
        </v-card>
      </v-col>

      <v-col cols="12" lg="4" md="6">
        <v-card :title="t('charts.country')">
          <v-card-text>
            <EChart :option="countryPie" />
          </v-card-text>
        </v-card>
      </v-col>

      <v-col cols="12" lg="4" md="6">
        <v-card :title="t('charts.channelBar')">
          <v-card-text>
            <EChart :option="channelBar" />
          </v-card-text>
        </v-card>
      </v-col>

      <v-col cols="12" md="6">
        <v-card :title="t('charts.dailyClients')">
          <v-card-text>
            <EChart :option="dailyLine" />
          </v-card-text>
        </v-card>
      </v-col>

      <v-col cols="12">
        <v-card :title="t('charts.telemetry7d')">
          <v-card-text>
            <EChart :height="220" :option="telemetryBar" />
          </v-card-text>
        </v-card>
      </v-col>

      <v-col v-if="auth.isPlatformAdmin" cols="12">
        <v-card :title="t('nodes.syncTitle')">
          <v-card-text>
            <ErrorAlert :error="syncError" />

            <v-data-table
              :headers="syncHeaders"
              hide-default-footer
              :items="syncRows"
              items-key="id"
              :loading="syncLoading"
            >
              <template #item.node="{ item }">
                {{ item.display_name || item.node_id }}
              </template>

              <template #item.progress="{ item }">
                {{ formatBytes(item.bytes_done ?? 0) }} / {{ formatBytes(item.bytes_total ?? 0) }}
              </template>
            </v-data-table>
          </v-card-text>
        </v-card>
      </v-col>

    </v-row>
  </div>
</template>

<script lang="ts" setup>
  import type { ClientBuckets, ClientDailyPoint, NodeArtifactSync, Project } from '@/api/generated'
  import { computed, onMounted, ref, watch } from 'vue'
  import { useI18n } from 'vue-i18n'
  import { useRoute } from 'vue-router'
  import { getProject, getProjectClientStats, getProjectStatsSeries, listProjectNodeSync } from '@/api/generated'
  import EChart from '@/components/EChart.vue'
  import ErrorAlert from '@/components/ErrorAlert.vue'
  import { barOption, dualBarOption, lineSeriesOption, pieOption } from '@/composables/chartOptions'
  import { geoLabel } from '@/composables/geoLabel'
  import { useChartTheme } from '@/composables/useChartTheme'
  import { useAuthStore } from '@/stores/auth'
  import { useUiStore } from '@/stores/ui'
  import { formatBytes } from '@/utils/bytes'

  const { t } = useI18n()
  const route = useRoute('/projects/[projectRef]/overview')
  const ui = useUiStore()
  const auth = useAuthStore()
  const projectRef = computed(() => route.params.projectRef)
  const project = ref<Project | null>(null)
  const buckets = ref<ClientBuckets | null>(null)
  const series = ref<ClientDailyPoint[]>([])
  const chartTheme = useChartTheme()
  const syncRows = ref<NodeArtifactSync[]>([])
  const syncLoading = ref(false)
  const syncError = ref<unknown>(null)
  const syncHeaders = computed(() => [
    { title: t('nodes.node'), key: 'node' },
    { title: t('nodes.version'), key: 'version_id' },
    { title: t('nodes.line'), key: 'line_id' },
    { title: t('nodes.status'), key: 'status' },
    { title: t('nodes.progress'), key: 'progress', sortable: false },
  ])

  const kpiTiles = computed(() => {
    const stats = project.value?.stats
    const versions = stats?.versions
    return [
      { label: t('projects.metricChannels'), value: String(stats?.channels ?? 0) },
      { label: t('projects.metricMatrix'), value: String(stats?.matrix_rows ?? 0) },
      { label: t('projects.metricVersions'), value: String(versions?.total ?? 0) },
      { label: t('versions.statusPublished'), value: String(versions?.published ?? 0) },
      { label: t('projects.fleetActive'), value: String(stats?.active_7d ?? 0) },
      { label: t('projects.storageLabel'), value: formatBytes(stats?.storage_bytes ?? 0) },
      { label: t('projects.artifactsCount', { n: stats?.artifact_count ?? 0 }), value: String(stats?.artifact_count ?? 0) },
      { label: t('clients.kpiTotal'), value: String(buckets.value?.total ?? 0) },
    ]
  })

  const versionPie = computed(() => pieOption(chartTheme.value, buckets.value?.versions))
  const channelBar = computed(() => barOption(chartTheme.value, buckets.value?.channels))
  const countryPie = computed(() => pieOption(chartTheme.value, (buckets.value?.countries ?? []).map(row => ({
    name: geoLabel(row.names, ui.locale, row.code, t('charts.geoUnknown')),
    count: row.count,
  }))))
  const dailyLine = computed(() => lineSeriesOption(
    chartTheme.value,
    series.value.map(row => row.day ?? ''),
    [
      { name: t('charts.newDevices'), data: series.value.map(row => row.new_count ?? 0), color: chartTheme.value.primary },
      { name: t('charts.activeDevices'), data: series.value.map(row => row.active_count ?? 0), color: chartTheme.value.info },
    ],
  ))
  const installDays = computed(() => utcDayKeys(7))
  const seriesByDay = computed(() => {
    const map = new Map<string, ClientDailyPoint>()
    for (const row of series.value) {
      if (row.day) {
        map.set(row.day, row)
      }
    }
    return map
  })
  const telemetryBar = computed(() => dualBarOption(
    chartTheme.value,
    installDays.value,
    { name: t('projects.telemetryInstalled'), data: installDays.value.map(day => seriesByDay.value.get(day)?.installed_count ?? 0), color: chartTheme.value.success },
    { name: t('projects.telemetryFailed'), data: installDays.value.map(day => seriesByDay.value.get(day)?.failed_count ?? 0), color: chartTheme.value.error },
  ))

  function utcDayKeys (count: number): string[] {
    const days: string[] = []
    const now = new Date()
    for (let i = count - 1; i >= 0; i--) {
      const d = new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), now.getUTCDate() - i))
      days.push(d.toISOString().slice(0, 10))
    }
    return days
  }

  async function load (): Promise<void> {
    const { data } = await getProject({ path: { project_ref: projectRef.value } })
    project.value = data ?? null
    try {
      const [bucketRes, seriesRes] = await Promise.all([
        getProjectClientStats({ path: { project_ref: projectRef.value } }),
        getProjectStatsSeries({ path: { project_ref: projectRef.value } }),
      ])
      buckets.value = bucketRes.data ?? null
      series.value = seriesRes.data?.series ?? []
    } catch {
      buckets.value = null
      series.value = []
    }
    if (auth.isPlatformAdmin) {
      syncLoading.value = true
      syncError.value = null
      try {
        const { data } = await listProjectNodeSync({ path: { project_ref: projectRef.value } })
        syncRows.value = data?.sync ?? []
      } catch (error) {
        syncError.value = error
        syncRows.value = []
      } finally {
        syncLoading.value = false
      }
    } else {
      syncRows.value = []
    }
  }

  onMounted(() => {
    void load()
  })
  watch(projectRef, () => {
    void load()
  })
</script>
