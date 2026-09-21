<!-- PageHeader.vue — 页面标题 + 描述 + 右侧动作插槽 -->
<template>
  <div class="d-flex flex-wrap align-center ga-4 mb-6">
    <div class="flex-grow-1">
      <div class="text-h5 font-weight-bold">{{ title }}</div>
      <div v-if="description" class="text-body-2 text-medium-emphasis mt-1">{{ description }}</div>
    </div>

    <div class="d-flex ga-2">
      <slot name="actions" />
    </div>
  </div>
</template>

<script lang="ts" setup>
  defineProps<{ title: string, description?: string }>()
</script>
