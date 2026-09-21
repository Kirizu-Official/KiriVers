import DefaultTheme from 'vitepress/theme'
import type { Theme } from 'vitepress'
import Mermaid from './Mermaid.vue'
import ErrorCode from './ErrorCode.vue'
import ScalarStandalone from './ScalarStandalone.vue'
import './custom.css'

export default {
  extends: DefaultTheme,
  enhanceApp({ app }) {
    app.component('Mermaid', Mermaid)
    app.component('ErrorCode', ErrorCode)
    app.component('ScalarStandalone', ScalarStandalone)
  },
} satisfies Theme
