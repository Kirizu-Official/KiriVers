<!-- EmptyState.vue — 空数据占位 -->
<template>
  <div class="d-flex flex-column align-center justify-center py-12 text-medium-emphasis">
    <v-icon class="mb-4 opacity-60" :icon="icon" size="48" />
    <div class="text-body-1">{{ text }}</div>

    <div class="mt-4">
      <slot name="actions" />
    </div>
  </div>
</template>

<script lang="ts" setup>
  withDefaults(defineProps<{ text: string, icon?: string }>(), { icon: 'mdi-inbox-outline' })
</script>
