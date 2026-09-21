<!-- DateTimeField.vue — Vuetify 日期（+ 可选 24h 时间）合成字段；v-model 为 RFC3339 或空串 -->
<template>
  <v-row dense>
    <v-col cols="12" :md="includeTime ? 7 : 12">
      <v-date-input
        v-model="date"
        clearable
        :disabled="disabled"
        hide-actions
        :hint="hint"
        :label="label"
        :persistent-hint="Boolean(hint)"
      />
    </v-col>

    <v-col v-if="includeTime" cols="12" md="5">
      <v-menu v-model="timeOpen" :close-on-content-click="false">
        <template #activator="{ props: menuProps }">
          <v-text-field
            v-bind="menuProps"
            :disabled="disabled || !date"
            :label="t('editor.time')"
            :model-value="timeLabel"
            prepend-inner-icon="mdi-clock-outline"
            readonly
          />
        </template>

        <v-card>
          <v-time-picker
            v-model="time"
            format="24hr"
          />
        </v-card>
      </v-menu>
    </v-col>
  </v-row>
</template>

<script lang="ts" setup>
  import { computed, nextTick, ref, watch } from 'vue'
  import { useI18n } from 'vue-i18n'

  const props = withDefaults(defineProps<{
    label: string
    includeTime?: boolean
    disabled?: boolean
    hint?: string
  }>(), {
    includeTime: true,
    disabled: false,
    hint: '',
  })

  const model = defineModel<string>({ default: '' })
  const { t } = useI18n()
  const timeOpen = ref(false)
  const date = ref<Date | null>(null)
  const time = ref<string | null>(null)
  let syncing = false

  const timeLabel = computed(() => time.value ?? '')

  function pad (n: number): string {
    return String(n).padStart(2, '0')
  }

  function toRFC3339 (value: Date): string {
    return value.toISOString().replace(/\.\d{3}Z$/, 'Z')
  }

  function calendarUTCMidnight (value: Date): string {
    return `${value.getFullYear()}-${pad(value.getMonth() + 1)}-${pad(value.getDate())}T00:00:00Z`
  }

  function asDate (value: unknown): Date | null {
    if (value == null || value === '') {
      return null
    }
    const next = value instanceof Date ? value : new Date(String(value))
    return Number.isNaN(next.getTime()) ? null : next
  }

  function parseModel (iso: string): void {
    const trimmed = iso.trim()
    if (!trimmed) {
      date.value = null
      time.value = null
      return
    }
    const next = asDate(trimmed)
    if (!next) {
      date.value = null
      time.value = null
      return
    }
    date.value = next
    time.value = `${pad(next.getHours())}:${pad(next.getMinutes())}`
  }

  function emitFromParts (): void {
    if (syncing) {
      return
    }
    if (!date.value) {
      if (model.value !== '') {
        model.value = ''
      }
      return
    }
    const next = new Date(date.value)
    let iso: string
    if (props.includeTime) {
      const [hours, minutes] = (time.value ?? '00:00').split(':').map(Number)
      next.setHours(hours || 0, minutes || 0, 0, 0)
      iso = toRFC3339(next)
    } else {
      iso = calendarUTCMidnight(next)
    }
    if (model.value !== iso) {
      model.value = iso
    }
  }

  parseModel(model.value)

  watch(model, value => {
    syncing = true
    parseModel(value)
    void nextTick(() => {
      syncing = false
    })
  })

  watch([date, time], () => {
    emitFromParts()
  })
</script>
