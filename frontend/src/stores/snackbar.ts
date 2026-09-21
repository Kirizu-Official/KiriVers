import { defineStore } from 'pinia'
/**
 * stores/snackbar.ts — 全局提示条队列。
 */
import { ref } from 'vue'

export interface SnackbarItem {
  id: number
  text: string
  color: 'success' | 'error' | 'info' | 'warning'
}

let seq = 0

export const useSnackbarStore = defineStore('snackbar', () => {
  const items = ref<SnackbarItem[]>([])

  function show (text: string, color: SnackbarItem['color'] = 'success', timeout = 3200): void {
    const id = ++seq
    items.value.push({ id, text, color })
    if (timeout > 0) {
      setTimeout(() => dismiss(id), timeout)
    }
  }

  function dismiss (id: number): void {
    items.value = items.value.filter(item => item.id !== id)
  }

  return { items, show, dismiss }
})
