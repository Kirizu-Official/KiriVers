<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'

const props = defineProps<{ specUrl: string }>()
const host = ref<HTMLElement | null>(null)
let api: { destroy?: () => void } | undefined
let observer: MutationObserver | undefined
let lastDark: boolean | undefined

function isDark(): boolean {
  return document.documentElement.classList.contains('dark')
}

async function mount() {
  if (!host.value) return
  api?.destroy?.()
  api = undefined
  await import('@scalar/api-reference/style.css')
  const mod = await import('@scalar/api-reference')
  const create = (mod as { createApiReference?: Function }).createApiReference
  if (typeof create !== 'function') {
    host.value.textContent = 'Unable to load API reference component.'
    return
  }
  api = create(host.value, {
    url: props.specUrl,
    layout: 'modern',
    darkMode: isDark(),
    hideDarkModeToggle: true,
    defaultOpenAllTags: false,
  }) as { destroy?: () => void }
}

onMounted(async () => {
  lastDark = isDark()
  await mount()
  observer = new MutationObserver(() => {
    const next = isDark()
    if (next === lastDark) return
    lastDark = next
    void mount()
  })
  observer.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] })
})

onBeforeUnmount(() => {
  observer?.disconnect()
  api?.destroy?.()
})
</script>

<template>
  <div class="scalar-standalone">
    <div ref="host" class="scalar-standalone__host" />
  </div>
</template>

<style scoped>
.scalar-standalone {
  height: 100vh;
  width: 100%;
  margin: 0;
  overflow: auto;
}

.scalar-standalone__host {
  min-height: 100vh;
}
</style>
