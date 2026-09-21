<!-- GlobalSnackbar.vue — 全局提示条（stores/snackbar 队列，右上堆叠） -->
<template>
  <div class="snackbar-stack">
    <v-snackbar
      v-for="item in store.items"
      :key="item.id"
      :color="item.color"
      location="top right"
      :model-value="true"
      multi-line
      timeout="-1"
      variant="tonal"
    >
      {{ item.text }}
      <template #actions>
        <v-btn icon="mdi-close" variant="text" @click="store.dismiss(item.id)" />
      </template>
    </v-snackbar>
  </div>
</template>

<script lang="ts" setup>
  import { useSnackbarStore } from '@/stores/snackbar'

  const store = useSnackbarStore()
</script>

<style scoped>
.snackbar-stack {
  position: fixed;
  top: 0;
  right: 0;
  z-index: 1200;
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 16px;
  pointer-events: none;
}
</style>
