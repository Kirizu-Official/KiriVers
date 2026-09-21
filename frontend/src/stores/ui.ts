import { defineStore } from 'pinia'
/**
 * stores/ui.ts
 *
 * UI 偏好：主题三态（light/dark/system）与语言，持久化到
 * localStorage（kirivers.ui）。system 态监听系统配色变化，
 * 通过 useTheme 同步 Vuetify 全局主题名。
 */
import { ref } from 'vue'
import { useTheme } from 'vuetify'
import { DARK_THEME, LIGHT_THEME } from '@/plugins/vuetify'

export type ThemeMode = 'light' | 'dark' | 'system'

const STORAGE_KEY = 'kirivers.ui'
const MEDIA = '(prefers-color-scheme: dark)'

interface UiPrefs {
  mode: ThemeMode
  locale: string
}

function load (): UiPrefs {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (raw) {
      const parsed = JSON.parse(raw) as Partial<UiPrefs>
      return {
        mode: parsed.mode === 'light' || parsed.mode === 'dark' ? parsed.mode : 'system',
        locale: parsed.locale === 'en' ? 'en' : 'zh-CN',
      }
    }
  } catch {
    // 忽略损坏数据
  }
  return { mode: 'system', locale: 'zh-CN' }
}

export const useUiStore = defineStore('ui', () => {
  const initial = load()
  const mode = ref<ThemeMode>(initial.mode)
  const locale = ref(initial.locale)

  let theme: ReturnType<typeof useTheme> | null = null
  const media = window.matchMedia ? window.matchMedia(MEDIA) : null

  function resolvedName (): string {
    const dark = mode.value === 'dark' || (mode.value === 'system' && (media?.matches ?? false))
    return dark ? DARK_THEME : LIGHT_THEME
  }

  function apply (): void {
    theme ??= useTheme()
    const next = resolvedName()
    if (theme.name.value !== next) {
      void theme.change(next)
    }
  }

  function persist (): void {
    localStorage.setItem(STORAGE_KEY, JSON.stringify({ mode: mode.value, locale: locale.value }))
  }

  function bind (): void {
    apply()
    media?.addEventListener('change', () => {
      if (mode.value === 'system') {
        apply()
      }
    })
  }

  function setMode (next: ThemeMode): void {
    mode.value = next
    persist()
    apply()
  }

  function setLocale (next: string): void {
    locale.value = next
    persist()
  }

  return { mode, locale, bind, setMode, setLocale, resolvedName }
})
