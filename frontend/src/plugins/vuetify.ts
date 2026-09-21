/**
 * plugins/vuetify.ts
 *
 * Vuetify 4 主题与全局组件默认值：品牌靛蓝主色、MD3 语气色、
 * 亮/暗两套表面色；VCard/VBtn/VTextField 等统一圆角与密度。
 * Framework documentation: https://vuetifyjs.com
 */
import { createVuetify } from 'vuetify'
import '@mdi/font/css/materialdesignicons.css'
import 'vuetify/styles'

export const LIGHT_THEME = 'kirivers-light'
export const DARK_THEME = 'kirivers-dark'

export const LIGHT_COLORS = {
  dark: false,
  colors: {
    'primary': '#4F5BD5',
    'primary-darken-1': '#3B47B0',
    'secondary': '#00897B',
    'accent': '#7C4DFF',
    'background': '#F6F7FB',
    'surface': '#FFFFFF',
    'surface-bright': '#FFFFFF',
    'surface-light': '#EEF0F6',
    'surface-variant': '#EEF0F6',
    'on-surface-variant': '#4A4D55',
    'error': '#B3261E',
    'success': '#2E7D32',
    'info': '#1565C0',
    'warning': '#ED6C02',
  },
}

export const DARK_COLORS = {
  dark: true,
  colors: {
    'primary': '#9FA8FF',
    'primary-darken-1': '#7C86F0',
    'secondary': '#4DB6AC',
    'accent': '#B388FF',
    'background': '#0F1115',
    'surface': '#1A1D23',
    'surface-bright': '#232730',
    'surface-light': '#1A1D23',
    'surface-variant': '#232730',
    'on-surface-variant': '#B8BCC7',
    'error': '#F2B8B5',
    'success': '#81C784',
    'info': '#64B5F6',
    'warning': '#FFB74D',
  },
}

export default createVuetify({
  theme: {
    defaultTheme: 'system',
    utilities: false,
    themes: {
      [LIGHT_THEME]: LIGHT_COLORS,
      [DARK_THEME]: DARK_COLORS,
    },
  },
  display: {
    mobileBreakpoint: 'md',
    thresholds: {
      xs: 0,
      sm: 600,
      md: 840,
      lg: 1145,
      xl: 1545,
      xxl: 2138,
    },
  },
  defaults: {
    VBtn: { rounded: 'lg' },
    VCard: { rounded: 'lg', elevation: 1 },
    VTextField: { variant: 'outlined', density: 'comfortable' },
    VSelect: { variant: 'outlined', density: 'comfortable' },
    VAutocomplete: { variant: 'outlined', density: 'comfortable' },
    VCombobox: { variant: 'outlined', density: 'comfortable' },
    VTextarea: { variant: 'outlined', density: 'comfortable' },
    VDialog: { maxWidth: 560 },
    VDataTable: { density: 'comfortable' },
    VChip: { label: true, size: 'small' },
    VAlert: { variant: 'tonal', rounded: 'lg' },
    VTabs: { density: 'comfortable' },
    VSwitch: { color: 'primary', inset: true },
    VSlider: { color: 'primary' },
    VRadioGroup: { color: 'primary' },
    VCheckbox: { color: 'primary' },
  },
})
