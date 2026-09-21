<!-- CopyField.vue — 只读等宽字段 + 复制按钮（Token 明文/哈希/UUID） -->
<template>
  <div class="copy-field d-flex align-center">
    <code class="flex-grow-1 text-body-2 text-truncate">{{ value }}</code>
    <v-btn icon="mdi-content-copy" size="x-small" variant="text" @click="copy" />
  </div>
</template>

<script lang="ts" setup>
  import { useI18n } from 'vue-i18n'
  import { useSnackbarStore } from '@/stores/snackbar'

  defineProps<{ value: string | null | undefined }>()
  const { t } = useI18n()
  const snackbar = useSnackbarStore()

  async function copy (event: MouseEvent): Promise<void> {
    const host = (event.currentTarget as HTMLElement).closest('.copy-field')
    const text = host?.querySelector('code')?.textContent ?? ''
    await navigator.clipboard.writeText(text)
    snackbar.show(t('common.copied'))
  }
</script>

<style scoped>
.copy-field code {
  padding: 4px 8px;
  border-radius: 6px;
  background: rgba(var(--v-theme-on-surface), 0.06);
}
</style>
