<!-- UploadProgress.vue — 百分比、已传/总字节、平滑速度。 -->
<template>
  <div v-if="active">
    <v-progress-linear
      class="mt-2"
      color="primary"
      height="8"
      :indeterminate="progress <= 0 && total <= 0"
      :model-value="progress"
      rounded
    />

    <div class="text-caption text-medium-emphasis mt-1">
      {{ caption }}
    </div>
  </div>
</template>

<script lang="ts" setup>
  import { computed } from 'vue'
  import { formatBytes, formatSpeed } from '@/utils/bytes'

  const props = withDefaults(defineProps<{
    active: boolean
    progress: number
    loaded?: number
    total?: number
    bytesPerSec?: number
    extra?: string
  }>(), {
    loaded: 0,
    total: 0,
    bytesPerSec: 0,
    extra: '',
  })

  const caption = computed(() => {
    const parts = [`${Math.max(0, Math.min(100, Math.round(props.progress)))}%`]
    if (props.total > 0) {
      parts.push(`${formatBytes(props.loaded)} / ${formatBytes(props.total)}`)
    } else if (props.loaded > 0) {
      parts.push(formatBytes(props.loaded))
    }
    parts.push(formatSpeed(props.bytesPerSec))
    if (props.extra) {
      parts.push(props.extra)
    }
    return parts.join(' · ')
  })
</script>
