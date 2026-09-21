<!-- StatusChip.vue — 版本/版本线状态徽章 -->
<template>
  <v-chip :color="color" label size="small">
    <v-icon :icon="icon" size="14" start />
    {{ label }}
  </v-chip>
</template>

<script lang="ts" setup>
  import { computed } from 'vue'
  import { useI18n } from 'vue-i18n'

  const props = defineProps<{ kind: 'version' | 'line' | 'announcement', status: string }>()
  const { t, te } = useI18n()

  const VERSIONS: Record<string, { color: string, icon: string, key: string }> = {
    draft: { color: 'grey', icon: 'mdi-pencil-outline', key: 'statusDraft' },
    published: { color: 'success', icon: 'mdi-check-circle-outline', key: 'statusPublished' },
    deprecated: { color: 'warning', icon: 'mdi-alert-circle-outline', key: 'statusDeprecated' },
    revoked: { color: 'error', icon: 'mdi-block-helper', key: 'statusRevoked' },
  }

  const LINES: Record<string, { color: string, icon: string, key: string }> = {
    uploading: { color: 'info', icon: 'mdi-tray-arrow-up', key: 'lineUploading' },
    processing: { color: 'info', icon: 'mdi-progress-clock', key: 'lineProcessing' },
    ready: { color: 'success', icon: 'mdi-check-circle-outline', key: 'lineReady' },
    yanked: { color: 'error', icon: 'mdi-arrow-down-bold-box-outline', key: 'lineYanked' },
    disabled: { color: 'grey', icon: 'mdi-pause-circle-outline', key: 'lineDisabled' },
    failed: { color: 'error', icon: 'mdi-alert-circle', key: 'lineFailed' },
  }

  const ANNOUNCEMENTS: Record<string, { color: string, icon: string, key: string }> = {
    draft: { color: 'grey', icon: 'mdi-pencil-outline', key: 'statusDraft' },
    published: { color: 'success', icon: 'mdi-check-circle-outline', key: 'statusPublished' },
    scheduled: { color: 'info', icon: 'mdi-clock-outline', key: 'statusScheduled' },
    expired: { color: 'warning', icon: 'mdi-calendar-remove-outline', key: 'statusExpired' },
  }

  const prefix = computed(() => {
    if (props.kind === 'line') return 'versions'
    if (props.kind === 'announcement') return 'announcements'
    return 'versions'
  })
  const table = computed(() => {
    if (props.kind === 'line') return LINES
    if (props.kind === 'announcement') return ANNOUNCEMENTS
    return VERSIONS
  })
  const meta = computed(() => table.value[props.status] ?? { color: 'grey', icon: 'mdi-help-circle-outline', key: props.status })
  const color = computed(() => meta.value.color)
  const icon = computed(() => meta.value.icon)
  const label = computed(() => {
    const key = `${prefix.value}.${meta.value.key}`
    return te(key) ? t(key) : meta.value.key
  })
</script>
