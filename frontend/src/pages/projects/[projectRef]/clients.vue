<!-- pages/projects/[projectRef]/clients.vue — 客户端：筛选、搜索、KPI、分布图 -->
<template>
  <div>
    <PageHeader :description="t('clients.description')" :title="t('clients.title')" />

    <ErrorAlert :error="error" />

    <v-row class="mb-2" dense>
      <v-col
        v-for="tile in kpiTiles"
        :key="tile.label"
        cols="12"
        sm="4"
      >
        <v-card variant="tonal">
          <v-card-text class="py-3">
            <div class="text-caption text-medium-emphasis">{{ tile.label }}</div>
            <div class="text-h6">{{ tile.value }}</div>
          </v-card-text>
        </v-card>
      </v-col>
    </v-row>

    <v-row class="mb-4">
      <v-col cols="12" lg="4" md="6">
        <v-card :title="t('charts.versionPie')">
          <v-card-text><EChart :option="versionPie" /></v-card-text>
        </v-card>
      </v-col>

      <v-col cols="12" lg="4" md="6">
        <v-card :title="t('charts.osArch')">
          <v-card-text><EChart :option="osBar" /></v-card-text>
        </v-card>
      </v-col>

      <v-col cols="12" lg="4" md="6">
        <v-card :title="t('charts.recency')">
          <v-card-text><EChart :option="recencyBar" /></v-card-text>
        </v-card>
      </v-col>
    </v-row>

    <div class="d-flex flex-wrap align-center ga-3 mb-4">
      <v-text-field
        v-model="filters.q"
        clearable
        density="compact"
        hide-details
        :label="t('common.search')"
        prepend-inner-icon="mdi-magnify"
        style="max-width: 280px"
        @keyup.enter="reload"
      />

      <v-text-field
        v-model="filters.os"
        clearable
        density="compact"
        hide-details
        :label="t('lines.os')"
        style="max-width: 140px"
        @keyup.enter="reload"
      />

      <v-text-field
        v-model="filters.arch"
        clearable
        density="compact"
        hide-details
        :label="t('lines.arch')"
        style="max-width: 140px"
        @keyup.enter="reload"
      />

      <v-text-field
        v-model="filters.version"
        clearable
        density="compact"
        hide-details
        :label="t('versions.version')"
        style="max-width: 160px"
        @keyup.enter="reload"
      />

      <v-select
        v-model="filters.activeWindow"
        clearable
        density="compact"
        hide-details
        :items="activeItems"
        :label="t('clients.activeSince')"
        style="max-width: 180px"
      />

      <v-btn prepend-icon="mdi-magnify" variant="tonal" @click="reload">{{ t('common.search') }}</v-btn>
    </div>

    <v-card>
      <v-data-table
        :headers="headers"
        :items="items"
        items-key="id"
        :items-length="total"
        :items-per-page="itemsPerPage"
        :loading="loading"
        :page="page"
        @update:items-per-page="onPerPage"
        @update:page="onPage"
      >
        <template #item.last_ip="{ item }">
          <span>{{ item.last_ip ?? '—' }}</span>
        </template>

        <template #item.country="{ item }">
          {{ geoLabel(item.geo_i18n?.country, ui.locale, item.country_code, t('charts.geoUnknown')) }}
        </template>

        <template #item.custom="{ item }">
          <code class="text-caption">{{ formatCustom(item.custom) }}</code>
        </template>

        <template #item.last_check_at="{ item }">
          <span class="text-body-2 text-medium-emphasis">{{ item.last_check_at ?? '—' }}</span>
        </template>

        <template #item.created_at="{ item }">
          <span class="text-body-2 text-medium-emphasis">{{ item.created_at ?? '—' }}</span>
        </template>

        <template #item.actions="{ item }">
          <v-btn prepend-icon="mdi-code-json" size="small" variant="text" @click="openJson(item)">
            {{ t('clients.viewJson') }}
          </v-btn>

          <v-btn
            color="error"
            prepend-icon="mdi-delete-outline"
            size="small"
            variant="text"
            @click="askDelete(item)"
          >
            {{ t('common.delete') }}
          </v-btn>
        </template>
      </v-data-table>
    </v-card>

    <v-dialog v-model="jsonOpen" max-width="720">
      <v-card :title="t('clients.viewJson')">
        <v-card-text>
          <pre class="text-caption">{{ jsonText }}</pre>
        </v-card-text>

        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="jsonOpen = false">{{ t('common.close') }}</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <ConfirmDialog
      v-model="deleteDialog"
      icon="mdi-delete-alert-outline"
      :text="deleteTarget ? t('clients.deleteText', { hash: deleteTarget.device_hash }) : ''"
      :title="t('clients.deleteTitle')"
      @confirm="doDelete"
    />
  </div>
</template>

<script lang="ts" setup>
  import type { Client, ClientBuckets } from '@/api/generated'
  import { computed, onMounted, reactive, ref, watch } from 'vue'
  import { useI18n } from 'vue-i18n'
  import { useRoute } from 'vue-router'
  import { deleteProjectClient, getProjectClient, getProjectClientStats, listProjectClients } from '@/api/generated'
  import ConfirmDialog from '@/components/ConfirmDialog.vue'
  import EChart from '@/components/EChart.vue'
  import ErrorAlert from '@/components/ErrorAlert.vue'
  import PageHeader from '@/components/PageHeader.vue'
  import { barOption, pieOption } from '@/composables/chartOptions'
  import { geoLabel } from '@/composables/geoLabel'
  import { useChartTheme } from '@/composables/useChartTheme'
  import { useUiStore } from '@/stores/ui'

  const { t } = useI18n()
  const route = useRoute('/projects/[projectRef]/clients')
  const projectRef = computed(() => route.params.projectRef)
  const chartTheme = useChartTheme()
  const ui = useUiStore()

  const filters = reactive({
    q: '',
    os: '',
    arch: '',
    version: '',
    activeWindow: '' as '' | '24h' | '7d',
  })
  const items = ref<Client[]>([])
  const total = ref(0)
  const loading = ref(false)
  const error = ref<unknown>(null)
  const buckets = ref<ClientBuckets | null>(null)
  const page = ref(1)
  const itemsPerPage = ref(25)

  const activeItems = computed(() => [
    { title: t('clients.active24h'), value: '24h' },
    { title: t('clients.active7d'), value: '7d' },
  ])

  const kpiTiles = computed(() => [
    { label: t('clients.kpiTotal'), value: String(buckets.value?.total ?? total.value) },
    { label: t('clients.kpiActive24h'), value: String(buckets.value?.active_24h ?? 0) },
    { label: t('clients.kpiActive7d'), value: String(buckets.value?.active_7d ?? 0) },
  ])

  const versionPie = computed(() => pieOption(chartTheme.value, buckets.value?.versions))
  const osBar = computed(() => barOption(chartTheme.value, [
    ...(buckets.value?.os ?? []),
    ...(buckets.value?.arch ?? []).map(row => ({ name: row.name ? `arch:${row.name}` : 'arch:—', count: row.count })),
  ]))
  const recencyBar = computed(() => barOption(chartTheme.value, buckets.value?.recency_hours))

  const headers = computed(() => [
    { title: t('clients.deviceHash'), key: 'device_hash' },
    { title: t('versions.version'), key: 'last_version' },
    { title: t('lines.os'), key: 'last_os' },
    { title: t('lines.arch'), key: 'last_arch' },
    { title: t('versions.channel'), key: 'last_channel' },
    { title: t('clients.lastIp'), key: 'last_ip' },
    { title: t('clients.country'), key: 'country' },
    { title: t('clients.lastCheck'), key: 'last_check_at' },
    { title: t('projects.createdAt'), key: 'created_at' },
    { title: t('clients.custom'), key: 'custom' },
    { title: t('common.actions'), key: 'actions', sortable: false, align: 'end' as const },
  ])

  const jsonOpen = ref(false)
  const jsonText = ref('')
  const deleteDialog = ref(false)
  const deleteTarget = ref<Client | null>(null)

  async function openJson (item: Client): Promise<void> {
    jsonOpen.value = true
    jsonText.value = JSON.stringify(item, null, 2)
    if (!item.id) return
    try {
      const { data } = await getProjectClient({
        path: { project_ref: projectRef.value, client_id: item.id },
      })
      jsonText.value = JSON.stringify(data ?? item, null, 2)
    } catch {
      // keep list-row JSON
    }
  }

  function askDelete (item: Client): void {
    deleteTarget.value = item
    deleteDialog.value = true
  }

  async function doDelete (): Promise<void> {
    if (!deleteTarget.value?.id) return
    try {
      await deleteProjectClient({
        path: { project_ref: projectRef.value, client_id: deleteTarget.value.id },
      })
      await loadList()
      await loadStats()
    } catch (error_) {
      error.value = error_
    }
  }

  function activeSince (): string | undefined {
    if (filters.activeWindow === '24h') {
      return new Date(Date.now() - 24 * 3600 * 1000).toISOString()
    }
    if (filters.activeWindow === '7d') {
      return new Date(Date.now() - 7 * 24 * 3600 * 1000).toISOString()
    }
    return undefined
  }

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

  async function loadList (): Promise<void> {
    loading.value = true
    error.value = null
    try {
      const { data } = await listProjectClients({
        path: { project_ref: projectRef.value },
        query: {
          q: filters.q.trim() || undefined,
          os: filters.os.trim() || undefined,
          arch: filters.arch.trim() || undefined,
          version: filters.version.trim() || undefined,
          active_since: activeSince(),
          limit: itemsPerPage.value,
          offset: (page.value - 1) * itemsPerPage.value,
        },
      })
      items.value = data?.clients ?? []
      total.value = data?.total ?? 0
    } catch (error_) {
      error.value = error_
    } finally {
      loading.value = false
    }
  }

  async function loadStats (): Promise<void> {
    try {
      const { data } = await getProjectClientStats({ path: { project_ref: projectRef.value } })
      buckets.value = data ?? null
    } catch {
      buckets.value = null
    }
  }

  function reload (): void {
    page.value = 1
    void loadList()
    void loadStats()
  }

  function onPage (next: number): void {
    page.value = next
    void loadList()
  }

  function onPerPage (next: number): void {
    itemsPerPage.value = next
    page.value = 1
    void loadList()
  }

  onMounted(() => {
    void loadList()
    void loadStats()
  })
  watch(projectRef, () => {
    reload()
  })
</script>
