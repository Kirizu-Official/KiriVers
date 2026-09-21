<!-- MarkdownEditor.vue — Vditor ir 封装：v-model 为存储用 Markdown（含 ${site_url}） -->
<template>
  <div class="markdown-editor" :style="{ '--md-min-height': `${minHeight}px` }">
    <div ref="host" />

    <UploadProgress
      :active="uploadActive"
      :bytes-per-sec="uploadBytesPerSec"
      :loaded="uploadLoaded"
      :progress="uploadProgress"
      :total="uploadTotal"
    />
  </div>
</template>

<script lang="ts" setup>
  import Vditor from 'vditor'
  import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
  import { useI18n } from 'vue-i18n'
  import { isApiError } from '@/api/client'
  import { uploadProjectMedia } from '@/api/generated'
  import UploadProgress from '@/components/UploadProgress.vue'
  import { createByteProgressTracker } from '@/composables/useUpload'
  import { DARK_THEME } from '@/plugins/vuetify'
  import { useUiStore } from '@/stores/ui'
  import 'vditor/dist/index.css'

  const SITE_URL = '${site_url}'

  const props = withDefaults(defineProps<{
    modelValue: string
    disabled?: boolean
    placeholder?: string
    minHeight?: number
    projectRef: string
  }>(), {
    disabled: false,
    placeholder: '',
    minHeight: 280,
  })

  const emit = defineEmits<{ 'update:modelValue': [value: string] }>()

  const { t } = useI18n()
  const ui = useUiStore()
  const host = ref<HTMLDivElement | null>(null)
  const uploadActive = ref(false)
  const uploadProgress = ref(0)
  const uploadLoaded = ref(0)
  const uploadTotal = ref(0)
  const uploadBytesPerSec = ref(0)
  const uploadTracker = createByteProgressTracker()

  const cdn = computed(() => {
    const base = String(import.meta.env.BASE_URL ?? '/').replace(/\/$/, '')
    return `${base}/vditor`
  })

  const vditorLang = computed((): 'zh_CN' | 'en_US' => ui.locale === 'en' ? 'en_US' : 'zh_CN')
  const isDark = computed(() => ui.resolvedName() === DARK_THEME)

  let vditor: Vditor | null = null
  let generation = 0
  let applying = false
  let pendingValue: string | null = null
  let ready = false

  function previewOrigin (): string {
    return window.location.origin.replace(/\/$/, '')
  }

  function expand (markdown: string): string {
    return markdown.replaceAll(SITE_URL, previewOrigin())
  }

  function collapse (markdown: string): string {
    const origin = previewOrigin()
    return markdown.replaceAll(origin, SITE_URL).replaceAll('%24%7Bsite_url%7D', SITE_URL)
  }

  function expandHtml (html: string): string {
    const origin = previewOrigin()
    return html.replaceAll(SITE_URL, origin).replaceAll('%24%7Bsite_url%7D', origin)
  }

  function applyTheme (instance: Vditor): void {
    const dark = isDark.value
    instance.setTheme(
      dark ? 'dark' : 'classic',
      dark ? 'dark' : 'light',
      'github',
      `${cdn.value}/dist/css/content-theme`,
    )
  }

  function applyDisabled (instance: Vditor): void {
    if (props.disabled) {
      instance.disabled()
      return
    }
    instance.enable()
  }

  function emitCollapsed (markdown: string): void {
    // eslint-disable-next-line vue/custom-event-name-casing
    emit('update:modelValue', collapse(markdown))
  }

  function isImageFile (name: string): boolean {
    return /\.(?:png|jpe?g|gif|webp|bmp|avif)$/i.test(name)
  }

  async function uploadFiles (files: File[]): Promise<string | null> {
    if (!props.projectRef) {
      return t('editor.uploadFailed')
    }
    uploadTracker.resetSpeed()
    uploadActive.value = true
    uploadProgress.value = 0
    uploadLoaded.value = 0
    uploadTotal.value = files.reduce((sum, file) => sum + file.size, 0)
    uploadBytesPerSec.value = 0
    try {
      const { data } = await uploadProjectMedia({
        path: { project_ref: props.projectRef },
        body: { 'file[]': files },
        throwOnError: true,
        onUploadProgress: event => {
          const loaded = event.loaded ?? 0
          const total = event.total && event.total > 0 ? event.total : uploadTotal.value
          uploadLoaded.value = loaded
          uploadTotal.value = total
          uploadBytesPerSec.value = uploadTracker.sample(loaded)
          uploadProgress.value = total > 0 ? Math.round((loaded / total) * 100) : 0
        },
      })
      if ((data?.code ?? 1) !== 0 || !data?.data?.succMap) {
        return data?.msg || t('editor.uploadFailed')
      }
      const parts: string[] = []
      for (const [name, url] of Object.entries(data.data.succMap)) {
        if (!url) {
          continue
        }
        const href = expand(url)
        parts.push(isImageFile(name) ? `![${name}](${href})` : `[${name}](${href})`)
      }
      if (parts.length > 0) {
        vditor?.insertMD(`${parts.join('\n')}\n`)
        if (vditor) {
          emitCollapsed(vditor.getValue())
        }
      }
      const errFiles = data.data.errFiles ?? []
      if (errFiles.length > 0 && parts.length === 0) {
        return t('editor.uploadFailed')
      }
      uploadProgress.value = 100
      return null
    } catch (error) {
      if (isApiError(error) && error.message) {
        return error.message
      }
      return t('editor.uploadFailed')
    } finally {
      uploadActive.value = false
    }
  }

  function destroyEditor (): void {
    generation += 1
    ready = false
    vditor?.destroy()
    vditor = null
  }

  function createEditor (): void {
    if (!host.value) {
      return
    }
    destroyEditor()
    const myGen = generation
    const instance = new Vditor(host.value, {
      after () {
        if (myGen !== generation || instance !== vditor) {
          return
        }
        applying = true
        const initial = pendingValue ?? props.modelValue ?? ''
        pendingValue = null
        instance.setValue(expand(initial), true)
        applyTheme(instance)
        applyDisabled(instance)
        applying = false
        ready = true
      },
      cache: { enable: false },
      cdn: cdn.value,
      hint: { emojiPath: `${cdn.value}/dist/images/emoji` },
      height: 'auto',
      icon: 'material',
      input (value: string) {
        if (applying || myGen !== generation) {
          return
        }
        emitCollapsed(value)
      },
      lang: vditorLang.value,
      minHeight: props.minHeight,
      mode: 'ir',
      placeholder: props.placeholder,
      preview: {
        theme: {
          current: isDark.value ? 'dark' : 'light',
          path: `${cdn.value}/dist/css/content-theme`,
        },
        transform: expandHtml,
      },
      theme: isDark.value ? 'dark' : 'classic',
      toolbar: [
        'headings', 'bold', 'italic', 'strike', '|',
        'line', 'quote', 'list', 'ordered-list', 'check', 'outdent', 'indent', '|',
        'code', 'inline-code', 'link', 'table', '|',
        'upload', '|',
        'undo', 'redo', '|',
        'fullscreen', 'edit-mode',
      ],
      upload: {
        fieldName: 'file[]',
        filename: name => name,
        format (_files, responseText) {
          return responseText
        },
        handler: files => uploadFiles(files) as Promise<string> | Promise<null>,
        max: 10 * 1024 * 1024,
        multiple: true,
      },
    })
    vditor = instance
  }

  function flush (): string {
    let stored = props.modelValue ?? ''
    if (vditor) {
      stored = collapse(vditor.getValue())
    } else if (pendingValue != null) {
      stored = pendingValue
    }
    // eslint-disable-next-line vue/custom-event-name-casing
    emit('update:modelValue', stored)
    return stored
  }

  defineExpose({ flush })

  onMounted(createEditor)
  onBeforeUnmount(destroyEditor)

  watch(() => props.modelValue, next => {
    if (!ready || !vditor) {
      pendingValue = next ?? ''
      return
    }
    if (collapse(vditor.getValue()) === (next ?? '')) {
      return
    }
    applying = true
    vditor.setValue(expand(next ?? ''), true)
    applying = false
  })

  watch(() => props.disabled, () => {
    if (vditor) {
      applyDisabled(vditor)
    }
  })

  watch(isDark, () => {
    if (vditor) {
      applyTheme(vditor)
    }
  })

  watch(vditorLang, () => {
    createEditor()
  })
</script>

<style scoped>
.markdown-editor :deep(.vditor) {
  border-radius: 8px;
}

.markdown-editor :deep(.vditor-ir pre.vditor-reset) {
  min-height: var(--md-min-height);
}
</style>
