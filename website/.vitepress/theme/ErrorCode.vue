<script setup lang="ts">
import { computed } from 'vue'
import { useData } from 'vitepress'
import { errorCodeText } from './error-codes'

const props = defineProps<{ code: string }>()

const { lang } = useData()

const hint = computed(() => errorCodeText(props.code, lang.value) ?? '')
</script>

<template>
  <span class="kv-error-code" tabindex="0">
    <code>{{ code }}</code>
    <span v-if="hint" class="kv-error-code__pop" role="tooltip">{{ hint }}</span>
  </span>
</template>

<style scoped>
.kv-error-code {
  position: relative;
  display: inline-block;
  cursor: help;
  outline: none;
}

.kv-error-code:focus-visible code {
  box-shadow: 0 0 0 2px var(--vp-c-brand-1);
}

.kv-error-code__pop {
  position: absolute;
  z-index: 30;
  left: 0;
  bottom: calc(100% + 0.4rem);
  display: none;
  width: max-content;
  max-width: min(22rem, 80vw);
  padding: 0.45rem 0.65rem;
  border-radius: 6px;
  border: 1px solid var(--vp-c-divider);
  background: var(--vp-c-bg-elv);
  color: var(--vp-c-text-1);
  font-size: 0.8rem;
  font-family: var(--vp-font-family-base);
  font-weight: 400;
  line-height: 1.45;
  white-space: normal;
  box-shadow: var(--vp-shadow-2);
}

.kv-error-code:hover .kv-error-code__pop,
.kv-error-code:focus-within .kv-error-code__pop,
.kv-error-code:focus .kv-error-code__pop {
  display: block;
}
</style>
