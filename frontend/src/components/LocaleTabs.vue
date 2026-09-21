<!-- LocaleTabs.vue — 多语言 changelog 编辑：从项目语言列表选择 + 共享 v-model -->
<template>
  <div>
    <v-select
      class="mb-2"
      :hint="t('languages.editorHint')"
      item-title="title"
      item-value="code"
      :items="localeItems"
      :label="t('languages.locale')"
      :model-value="selected"
      persistent-hint
      @update:model-value="onSelectLocale"
    />

    <MarkdownEditor
      ref="editorRef"
      :disabled="disabled"
      :min-height="minHeight"
      :model-value="selected ? (modelValue?.[selected] ?? '') : ''"
      :placeholder="t('versions.changelogHint')"
      :project-ref="projectRef"
      @update:model-value="setLocale($event)"
    />
  </div>
</template>

<script lang="ts" setup>
  import { computed, ref, watch } from 'vue'
  import { useI18n } from 'vue-i18n'
  import MarkdownEditor from '@/components/MarkdownEditor.vue'
  import { languageLabel } from '@/composables/useProjectLanguages'

  const props = withDefaults(defineProps<{
    modelValue: Record<string, string> | null
    languages?: Array<{ code?: string, display_name?: string, is_default?: boolean }>
    rows?: number
    disabled?: boolean
    projectRef: string
  }>(), { rows: 10, disabled: false, languages: () => [] })

  const emit = defineEmits<{ 'update:modelValue': [value: Record<string, string>] }>()

  const { t } = useI18n()
  const selected = ref('')
  const editorRef = ref<{ flush: () => string } | null>(null)
  const minHeight = computed(() => Math.max(240, props.rows * 28))

  const localeItems = computed(() => (props.languages ?? []).map(row => ({
    title: languageLabel(row),
    code: row.code ?? '',
  })).filter(item => item.code))

  watch(() => props.languages, list => {
    const codes = (list ?? []).flatMap(row => row.code ? [row.code] : [])
    if (codes.length === 0) {
      selected.value = ''
      return
    }
    if (selected.value && codes.includes(selected.value)) {
      return
    }
    const def = (list ?? []).find(row => row.is_default)?.code
    selected.value = def && codes.includes(def) ? def : codes[0]
  }, { immediate: true })

  function applyMap (locale: string, value: string): Record<string, string> {
    const next: Record<string, string> = props.modelValue == null ? {} : { ...props.modelValue }
    if (value) {
      next[locale] = value
    } else {
      delete next[locale]
    }
    return next
  }

  function setLocale (value: string): void {
    if (!selected.value) return
    // eslint-disable-next-line vue/custom-event-name-casing
    emit('update:modelValue', applyMap(selected.value, value))
  }

  function onSelectLocale (code: unknown): void {
    if (selected.value) {
      const flushed = editorRef.value?.flush() ?? props.modelValue?.[selected.value] ?? ''
      // eslint-disable-next-line vue/custom-event-name-casing
      emit('update:modelValue', applyMap(selected.value, flushed))
    }
    selected.value = typeof code === 'string' ? code : ''
  }

  function flush (): Record<string, string> {
    if (!selected.value) {
      return props.modelValue == null ? {} : { ...props.modelValue }
    }
    const flushed = editorRef.value?.flush() ?? props.modelValue?.[selected.value] ?? ''
    const next = applyMap(selected.value, flushed)
    // eslint-disable-next-line vue/custom-event-name-casing
    emit('update:modelValue', next)
    return next
  }

  defineExpose({ flush })
</script>
