<!-- ErrorAlert.vue — 将 ApiError.code 映射为可读提示（errors.<CODE> i18n），兜底 message -->
<template>
  <v-alert v-if="error" class="mb-4" density="compact" type="error">
    <div class="font-weight-medium">{{ title }}</div>
    <div class="text-body-2">{{ text }}</div>
    <div v-if="detail" class="text-caption text-medium-emphasis mt-1">{{ detail }}</div>
  </v-alert>
</template>

<script lang="ts" setup>
  import { computed } from 'vue'
  import { useI18n } from 'vue-i18n'
  import { isApiError } from '@/api/client'

  const props = defineProps<{ error: unknown }>()
  const { t, te } = useI18n()

  const title = computed(() => (te('errors.title') ? t('errors.title') : 'Error'))
  const text = computed(() => {
    const err = props.error
    if (isApiError(err)) {
      const key = `errors.${err.code}`
      return te(key) ? t(key) : err.message
    }
    return err instanceof Error ? err.message : String(err ?? '')
  })
  const detail = computed(() => {
    const err = props.error
    if (isApiError(err) && err.details != null) {
      try {
        return JSON.stringify(err.details)
      } catch {
        return String(err.details)
      }
    }
    return ''
  })
</script>
