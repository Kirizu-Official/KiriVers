/**
 * main.ts
 *
 * Bootstraps Vuetify and other plugins then mounts the App.
 * 所有 /api 请求直连真实后端（dev 经 Vite 代理分流 admin/client 双平面）。
 */

/* eslint-disable perfectionist/sort-imports -- interceptors must load before plugins/stores */
import '@/api/client'
import { createApp } from 'vue'
import { registerPlugins } from '@/plugins'
import App from './App.vue'
/* eslint-enable perfectionist/sort-imports */

// Styles
import 'unfonts.css'
import './styles/tailwind.css'
import './styles/main.scss'

const app = createApp(App)
registerPlugins(app)
app.mount('#app')
