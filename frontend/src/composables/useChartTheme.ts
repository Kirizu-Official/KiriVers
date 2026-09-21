/**
 * composables/useChartTheme.ts
 *
 * Resolve Vuetify MD3 CSS variables into rgb() colors ECharts can paint.
 * Recomputes when the global theme name changes (light / dark / system).
 */
import { computed } from 'vue'
import { useTheme } from 'vuetify'

export interface ChartTheme {
  primary: string
  secondary: string
  accent: string
  success: string
  warning: string
  error: string
  info: string
  text: string
  muted: string
  border: string
  tooltipBg: string
  palette: string[]
}

function rgbToken (style: CSSStyleDeclaration, token: string, fallback: string): string {
  const raw = style.getPropertyValue(`--v-theme-${token}`).trim()
  return raw ? `rgb(${raw})` : fallback
}

export function useChartTheme () {
  const theme = useTheme()

  return computed<ChartTheme>(() => {
    void theme.name.value
    const style = getComputedStyle(document.documentElement)
    const primary = rgbToken(style, 'primary', '#4F5BD5')
    const secondary = rgbToken(style, 'secondary', '#00897B')
    const accent = rgbToken(style, 'accent', '#7C4DFF')
    const success = rgbToken(style, 'success', '#2E7D32')
    const warning = rgbToken(style, 'warning', '#ED6C02')
    const error = rgbToken(style, 'error', '#B3261E')
    const info = rgbToken(style, 'info', '#1565C0')
    const text = rgbToken(style, 'on-surface', '#1A1D23')
    const muted = rgbToken(style, 'on-surface-variant', '#4A4D55')
    return {
      primary,
      secondary,
      accent,
      success,
      warning,
      error,
      info,
      text,
      muted,
      border: 'rgba(128,128,128,0.28)',
      tooltipBg: rgbToken(style, 'surface', '#FFFFFF'),
      palette: [primary, info, success, warning, error, secondary, accent],
    }
  })
}
