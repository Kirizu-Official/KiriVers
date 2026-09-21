<!-- DefaultShell.vue — 控制台外壳：顶栏 + 导航抽屉（全局/项目上下文） + 内容区 -->
<template>
  <v-app-bar border flat>
    <v-app-bar-nav-icon class="hidden-md-and-up" @click="drawer = !drawer" />

    <v-app-bar-title class="font-weight-bold">
      <span class="text-primary">{{ t('common.appName') }}</span>
    </v-app-bar-title>

    <template #append>
      <!-- 任务中心 -->
      <v-menu :close-on-content-click="false" location="bottom">
        <template #activator="{ props }">
          <v-btn v-bind="props" :color="jobs.runningCount > 0 ? 'primary' : undefined" icon="mdi-bell-outline">
            <v-badge v-if="jobs.runningCount > 0" :content="jobs.runningCount" floating>
              <v-icon icon="mdi-bell-outline" />
            </v-badge>

            <v-icon v-else icon="mdi-bell-outline" />
          </v-btn>
        </template>

        <v-card min-width="360">
          <v-card-title class="text-subtitle-1">{{ t('release.taskCenter') }}</v-card-title>

          <v-card-text v-if="jobs.jobs.length === 0" class="text-medium-emphasis">
            {{ t('release.noJobs') }}
          </v-card-text>

          <v-list v-else density="compact">
            <v-list-item v-for="job in jobs.jobs.slice(0, 8)" :key="job.jobId">
              <template #prepend>
                <v-icon :color="jobColor(job)" :icon="jobIcon(job)" />
              </template>

              <v-list-item-title>{{ job.label }}</v-list-item-title>

              <v-list-item-subtitle>
                {{ t(`release.job${capitalize(job.status)}`, job.status ?? '') }}
                <template v-if="job.status === 'running' || job.status === 'queued'"> · {{ job.progress }}%</template>
              </v-list-item-subtitle>
            </v-list-item>
          </v-list>

          <v-card-actions v-if="jobs.jobs.some(j => isFinished(j.status))">
            <v-btn size="small" variant="text" @click="jobs.clearFinished()">
              {{ t('common.close') }}
            </v-btn>
          </v-card-actions>
        </v-card>
      </v-menu>

      <!-- 语言切换 -->
      <v-menu location="bottom">
        <template #activator="{ props }">
          <v-btn v-bind="props" :aria-label="t('common.language')" icon="mdi-translate" />
        </template>

        <v-list density="compact">
          <v-list-item
            v-for="item in [['zh-CN', '简体中文'], ['en', 'English']]"
            :key="item[0]"
            :active="ui.locale === item[0]"
            :title="item[1]"
            @click="switchLocale(item[0])"
          />
        </v-list>
      </v-menu>

      <!-- 主题切换 -->
      <v-menu location="bottom">
        <template #activator="{ props }">
          <v-btn v-bind="props" :aria-label="t('common.theme')" :icon="modeIcon" />
        </template>

        <v-list density="compact">
          <v-list-item
            v-for="item in [['light', 'mdi-white-balance-sunny', '浅色'], ['dark', 'mdi-weather-night', '深色'], ['system', 'mdi-theme-light-dark', '跟随系统']] as const"
            :key="item[0]"
            :active="ui.mode === item[0]"
            :prepend-icon="item[1]"
            :title="item[2]"
            @click="ui.setMode(item[0])"
          />
        </v-list>
      </v-menu>

      <!-- 关于：前后端编译信息（身份类的用户菜单保持在最右） -->
      <v-btn :aria-label="t('nav.about')" icon="mdi-information-outline" @click="aboutOpen = true" />

      <!-- 用户菜单 -->
      <v-menu location="bottom">
        <template #activator="{ props }">
          <v-btn
            v-bind="props"
            class="ml-1"
            color="primary"
            prepend-icon="mdi-account-circle-outline"
            rounded="lg"
            variant="tonal"
          >
            {{ auth.username }}
          </v-btn>
        </template>

        <v-list density="compact">
          <v-list-item
            prepend-icon="mdi-shield-lock-outline"
            :title="t('nav.accountSecurity')"
            :to="{ path: '/security' }"
          />

          <v-list-item prepend-icon="mdi-logout" :title="t('nav.logout')" @click="logout" />
        </v-list>
      </v-menu>
    </template>
  </v-app-bar>

  <!-- 顶栏"关于"按钮打开的对话框；与 footer 共用 useBuildInfo 的单次请求缓存 -->
  <AboutDialog v-model="aboutOpen" />

  <v-navigation-drawer v-model="drawer" border>
    <v-list nav>
      <v-list-item
        v-if="auth.isPlatformAdmin"
        prepend-icon="mdi-shield-account-outline"
        :title="t('nav.admins')"
        :to="{ path: '/admins' }"
      />

      <v-list-item
        v-if="auth.isPlatformAdmin"
        prepend-icon="mdi-earth"
        :title="t('nav.geoip')"
        :to="{ path: '/geoip' }"
      />

      <v-list-item
        v-if="auth.isPlatformAdmin"
        prepend-icon="mdi-server-network"
        :title="t('nav.nodes')"
        :to="{ path: '/nodes' }"
      />

      <v-list-item
        prepend-icon="mdi-view-dashboard-outline"
        :title="t('nav.projects')"
        :to="{ path: '/projects' }"
      />
    </v-list>

    <template v-if="projectRef">
      <v-divider />

      <div class="px-4 py-2 text-caption text-medium-emphasis">
        {{ projectTitle }}
      </div>

      <v-list density="comfortable" nav>
        <v-list-item
          v-for="item in projectNav"
          :key="item.to"
          :active="activeProjectTo === item.to"
          exact
          :prepend-icon="item.icon"
          :title="item.title"
          :to="{ path: item.to }"
        />
      </v-list>
    </template>
  </v-navigation-drawer>

  <v-main class="d-flex flex-column">
    <div class="pa-4 pa-md-6">
      <slot />
    </div>

  </v-main>

  <v-footer app class="text-label-medium">
    {{ t('about.backend') }} {{ backendVersion }} · {{ t('about.frontend') }} {{ build.frontend.version }}
  </v-footer>
  <!-- footer 只给前后端版本号（其余编译字段留在"关于"对话框）；后端不可得时显示 common.none -->

</template>

<script lang="ts" setup>
  import { computed, onMounted, provide, ref, watch } from 'vue'
  import { useI18n } from 'vue-i18n'
  import { useRoute, useRouter } from 'vue-router'
  import { getProject } from '@/api/generated'
  import AboutDialog from '@/components/AboutDialog.vue'
  import { useBuildInfo } from '@/composables/useBuildInfo'
  import { matchProjectNav, projectNavItems } from '@/composables/useProjectNav'
  import i18n from '@/plugins/i18n'
  import { useAuthStore } from '@/stores/auth'
  import { type TrackedJob, useJobsStore } from '@/stores/jobs'
  import { useUiStore } from '@/stores/ui'

  const { t } = useI18n()
  const route = useRoute()
  const router = useRouter()
  const auth = useAuthStore()
  const ui = useUiStore()
  const jobs = useJobsStore()

  const drawer = ref(true)

  // 编译信息（R5/R6）：顶栏"关于"按钮与 footer 共用同一份缓存；ensureLoaded() 幂等，
  // 因此挂载时拉一次即可，路由切换或本组件重挂都不会再打这个端点。
  const build = useBuildInfo()
  const aboutOpen = ref(false)
  const backendVersion = computed(() => build.backend.value?.version ?? t('common.none'))

  onMounted(() => {
    void build.ensureLoaded()
  })

  const projectRef = computed(() => (route.params as { projectRef?: string }).projectRef)
  const projectTitle = ref('')

  function setProjectCaption (title: string): void {
    projectTitle.value = title
  }
  provide('setProjectCaption', setProjectCaption)

  watch(projectRef, async refValue => {
    if (!refValue) {
      projectTitle.value = ''
      return
    }
    projectTitle.value = refValue
    try {
      const { data } = await getProject({ path: { project_ref: refValue } })
      projectTitle.value = data?.name || refValue
    } catch {
      projectTitle.value = refValue
    }
  }, { immediate: true })

  const modeIcon = computed(() => (ui.mode === 'dark' ? 'mdi-weather-night' : (ui.mode === 'light' ? 'mdi-white-balance-sunny' : 'mdi-theme-light-dark')))

  const projectNav = computed(() => projectNavItems(projectRef.value ?? '', t))
  const activeProjectTo = computed(() => matchProjectNav(route.path, projectNav.value))

  // 非 md 断点默认收起抽屉
  watch(() => route.name, () => {
    if (window.innerWidth < 840) drawer.value = false
  })

  function jobIcon (job: TrackedJob): string {
    switch (job.status) {
      case 'succeeded': { return 'mdi-check-circle'
      }
      case 'failed': { return 'mdi-alert-circle'
      }
      case 'running': { return 'mdi-progress-clock'
      }
      default: { return 'mdi-timer-sand'
      }
    }
  }

  function jobColor (job: TrackedJob): string {
    switch (job.status) {
      case 'succeeded': { return 'success'
      }
      case 'failed': { return 'error'
      }
      default: { return 'info'
      }
    }
  }

  function isFinished (status: TrackedJob['status']): boolean {
    return status === 'succeeded' || status === 'failed'
  }

  function capitalize (value: string | undefined): string {
    return (value ?? '').charAt(0).toUpperCase() + (value ?? '').slice(1)
  }

  function switchLocale (next: string): void {
    ui.setLocale(next)
    i18n.global.locale.value = next as 'zh-CN' | 'en'
  }

  async function logout (): Promise<void> {
    await auth.logout()
    await router.push({ path: '/login' })
  }
</script>
