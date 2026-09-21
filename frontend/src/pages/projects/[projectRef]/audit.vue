<!-- pages/projects/[projectRef]/audit.vue — 审计日志 -->
<template>
  <div>
    <PageHeader :description="t('audit.description')" :title="t('audit.title')" />

    <ErrorAlert :error="resource.error.value" />
    <EmptyState v-if="!resource.loading.value && resource.items.value.length === 0" icon="mdi-text-box-search-outline" :text="t('common.empty')" />

    <v-card v-else>
      <v-data-table
        :headers="headers"
        hide-default-footer
        :items="resource.items.value"
        items-key="id"
        :loading="resource.loading.value"
      >
        <template #item.actor_fingerprint="{ item }">
          {{ item.actor_fingerprint ?? '—' }}
        </template>

        <template #item.action="{ item }">
          <v-chip :color="actionColor(item.action)" label size="small" variant="tonal">
            {{ item.action }}
          </v-chip>
        </template>

        <template #item.target="{ item }">
          {{ item.resource_type ? `${item.resource_type}:${item.resource_id ?? ''}` : (item.resource_id ?? '—') }}
        </template>

        <template #item.detail="{ item }">
          <code v-if="item.detail" class="text-caption">{{ JSON.stringify(item.detail) }}</code>
          <span v-else class="text-medium-emphasis">—</span>
        </template>
      </v-data-table>
    </v-card>
  </div>
</template>

<script lang="ts" setup>
  import type { AuditListOutput } from '@/api/generated'
  import { computed } from 'vue'
  import { useI18n } from 'vue-i18n'
  import { useRoute } from 'vue-router'
  import { listAuditLogs } from '@/api/generated'
  import EmptyState from '@/components/EmptyState.vue'
  import ErrorAlert from '@/components/ErrorAlert.vue'
  import PageHeader from '@/components/PageHeader.vue'
  import { useApiResource } from '@/composables/useApiResource'

  type AuditEvent = NonNullable<AuditListOutput['events']>[number]

  const { t } = useI18n()
  const route = useRoute('/projects/[projectRef]/audit')

  const projectRef = computed(() => route.params.projectRef)
  const resource = useApiResource<AuditEvent>(async () => {
    const { data } = await listAuditLogs({
      path: { project_ref: projectRef.value },
      query: { limit: 100 },
    })
    return data?.events ?? []
  })

  const headers = computed(() => [
    { title: t('audit.time'), key: 'created_at', width: 190 },
    { title: t('audit.actor'), key: 'actor_fingerprint', width: 140 },
    { title: t('audit.action'), key: 'action', width: 200 },
    { title: t('audit.target'), key: 'target' },
    { title: t('audit.details'), key: 'detail' },
  ])

  function actionColor (action = ''): string {
    if (action.includes('delete') || action.includes('revoke')) return 'error'
    if (action.includes('create') || action.includes('publish')) return 'success'
    return 'info'
  }
</script>
