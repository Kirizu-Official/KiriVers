<!-- ConfirmDialog.vue — 危险操作确认；confirmWord 非空时要求输入匹配 -->
<template>
  <v-dialog v-model="model" max-width="480">
    <v-card>
      <v-card-title class="d-flex align-center">
        <v-icon class="mr-3" color="error" :icon="icon" />
        {{ title }}
      </v-card-title>

      <v-card-text>
        <div>{{ text }}</div>

        <v-text-field
          v-if="confirmWord"
          v-model="typed"
          class="mt-4"
          density="compact"
          :label="confirmLabel ?? confirmWord"
          persistent-hint
          variant="outlined"
        />
      </v-card-text>

      <v-card-actions>
        <v-spacer />
        <v-btn variant="text" @click="model = false">{{ t('common.cancel') }}</v-btn>

        <v-btn
          color="error"
          :disabled="Boolean(confirmWord) && typed !== confirmWord"
          @click="confirm"
        >
          {{ confirmText ?? t('common.confirm') }}
        </v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>

<script lang="ts" setup>
  import { ref, watch } from 'vue'
  import { useI18n } from 'vue-i18n'

  defineProps<{
    title: string
    text: string
    icon?: string
    confirmText?: string
    /** 需要输入匹配的确认词（如对象名/版本号） */
    confirmWord?: string
    confirmLabel?: string
  }>()

  const model = defineModel<boolean>({ default: false })
  const emit = defineEmits<{ confirm: [] }>()

  const { t } = useI18n()
  const typed = ref('')

  watch(model, open => {
    if (open) typed.value = ''
  })

  function confirm (): void {
    model.value = false
    emit('confirm')
  }
</script>
