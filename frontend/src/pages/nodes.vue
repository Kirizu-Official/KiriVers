<!-- pages/nodes.vue — 平台节点列表：UUID、显示名、心跳、CPU/内存 -->
<template>
  <div>
    <PageHeader :description="t('nodes.description')" :title="t('nodes.title')">
      <template #actions>
        <v-chip :color="clusterActive ? 'primary' : 'secondary'" variant="tonal">
          {{ clusterActive ? t('nodes.modeCluster') : t('nodes.modeSingleton') }}
        </v-chip>
      </template>
    </PageHeader>

    <ErrorAlert :error="resource.error.value" />

    <EmptyState v-if="!resource.loading.value && resource.items.value.length === 0" icon="mdi-server-network" :text="t('common.empty')" />

    <v-card v-else>
      <v-data-table
        :headers="headers"
        hide-default-footer
        :items="resource.items.value"
        items-key="id"
        :loading="resource.loading.value"
      >
        <template #item.display_name="{ item }">
          <div class="d-flex align-center ga-2">
            <span>{{ item.display_name || item.id }}</span>

            <v-chip v-if="item.is_current" color="primary" size="x-small" variant="flat">
              {{ t('nodes.current') }}
            </v-chip>
          </div>
        </template>

        <template #item.offline="{ item }">
          <v-chip :color="item.offline ? 'error' : 'success'" size="x-small" variant="tonal">
            {{ item.offline ? t('nodes.offline') : t('nodes.online') }}
          </v-chip>
        </template>

        <template #item.cpu_percent="{ item }">
          {{ formatCpu(item.cpu_percent) }}
        </template>

        <template #item.memory="{ item }">
          {{ formatBytes(item.mem_used_bytes ?? 0) }} / {{ formatBytes(item.mem_total_bytes ?? 0) }}
        </template>
      </v-data-table>
    </v-card>
  </div>
</template>

<script lang="ts" setup>
  import type { ClusterNode } from '@/api/generated'
  import { computed, ref } from 'vue'
  import { useI18n } from 'vue-i18n'
  import { listNodes } from '@/api/generated'
  import EmptyState from '@/components/EmptyState.vue'
  import ErrorAlert from '@/components/ErrorAlert.vue'
  import PageHeader from '@/components/PageHeader.vue'
  import { useApiResource } from '@/composables/useApiResource'
  import { formatBytes } from '@/utils/bytes'

  const { t } = useI18n()
  const clusterActive = ref(false)

  const resource = useApiResource<ClusterNode>(async () => {
    const { data } = await listNodes()
    clusterActive.value = Boolean(data?.cluster_active)
    return data?.nodes ?? []
  })

  const headers = computed(() => [
    { title: t('nodes.displayName'), key: 'display_name' },
    { title: t('nodes.id'), key: 'id' },
    { title: t('nodes.status'), key: 'offline' },
    { title: t('nodes.lastSeen'), key: 'last_seen_at' },
    { title: t('nodes.cpu'), key: 'cpu_percent' },
    { title: t('nodes.memory'), key: 'memory', sortable: false },
  ])

  function formatCpu (value: number | undefined): string {
    return `${(value ?? 0).toFixed(1)}%`
  }
</script>
