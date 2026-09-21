/**
 * router/index.ts
 *
 * unplugin-vue-router 自动路由 + 全局守卫：
 * - 未登录访问受保护页 → /login?redirect=…
 * - 已登录访问 guest 页（登录页）→ /projects
 * - / → /projects
 */
import { createRouter, createWebHistory } from 'vue-router'
import { routes } from 'vue-router/auto-routes'
import { useAuthStore } from '@/stores/auth'

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes,
})

router.beforeEach(to => {
  if (to.path === '/') {
    return { path: '/projects', replace: true }
  }
  const auth = useAuthStore()
  const isGuestPage = to.path === '/login'
  if (!auth.isAuthenticated && !isGuestPage) {
    return { path: '/login', query: { redirect: to.fullPath }, replace: true }
  }
  if (auth.isAuthenticated && isGuestPage) {
    return { path: '/projects', replace: true }
  }
  if (auth.isAuthenticated && ['/admins', '/geoip', '/nodes'].includes(to.path) && !auth.isPlatformAdmin) {
    return { path: '/projects', replace: true }
  }
  return true
})

export default router
