<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'

const props = defineProps<{ code: string }>()
const el = ref<HTMLElement | null>(null)

async function render() {
  if (!el.value) return
  const mermaid = (await import('mermaid')).default
  mermaid.initialize({ startOnLoad: false, theme: 'neutral', securityLevel: 'strict' })
  const decoded = decodeURIComponent(props.code)
  const id = `mmd-${Math.random().toString(36).slice(2)}`
  const { svg } = await mermaid.render(id, decoded)
  el.value.innerHTML = svg
}

onMounted(render)
watch(() => props.code, render)
</script>

<template>
  <div ref="el" class="vp-mermaid" />
</template>
