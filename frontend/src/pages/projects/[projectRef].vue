<!-- pages/projects/[projectRef].vue — 项目上下文父页：项目头 + tabs + 子路由 -->
<template>
  <div>
    <div v-if="loading" class="d-flex justify-center py-12">
      <v-progress-circular color="primary" indeterminate />
    </div>

    <template v-else-if="project">
      <div class="d-flex flex-wrap align-center ga-3 mb-1">
        <v-btn icon="mdi-arrow-left" size="small" variant="text" @click="router.push('/projects')" />
        <div class="text-h5 font-weight-bold">{{ project.name || project.slug }}</div>

        <v-chip color="primary" label size="small">
          {{ t(`projects.compareEngine${project.compare_engine === 'integer' ? 'Integer' : 'Semver'}`) }}
        </v-chip>
      </div>

      <div class="text-body-2 text-medium-emphasis mb-1">{{ project.slug }}</div>
      <CopyField class="mb-2" :value="project.uuid" />

      <v-tabs class="mb-6" color="primary" density="comfortable" :model-value="tab">
        <v-tab v-for="item in tabs" :key="item.to" :to="item.to" :value="item.to">
          {{ item.title }}
        </v-tab>
      </v-tabs>

      <RouterView />
    </template>

    <ErrorAlert v-else :error="error" />
  </div>
</template>

<script lang="ts" setup>
  import type { Project } from '@/api/generated'
  import { computed, inject, onMounted, provide, ref, watch } from 'vue'
  import { useI18n } from 'vue-i18n'
  import { useRoute, useRouter } from 'vue-router'
  import { getProject } from '@/api/generated'
  import CopyField from '@/components/CopyField.vue'
  import ErrorAlert from '@/components/ErrorAlert.vue'
  import { matchProjectNav, projectNavItems } from '@/composables/useProjectNav'

  const { t } = useI18n()
  const route = useRoute('/projects/[projectRef]')
  const router = useRouter()

  const project = ref<Project | null>(null)
  const loading = ref(true)
  const error = ref<unknown>(null)
  const setProjectCaption = inject<(title: string) => void>('setProjectCaption', () => {})

  const projectRef = computed(() => route.params.projectRef as string)
  provide('projectRecord', project)

  watch(project, p => {
    if (p) setProjectCaption(p.name || p.slug || projectRef.value)
  })

  const tabs = computed(() => projectNavItems(projectRef.value, t).map(item => ({ to: item.to, title: item.title })))
  const tab = computed(() => matchProjectNav(route.path, tabs.value))

  async function load (): Promise<void> {
    loading.value = true
    error.value = null
    try {
      const { data } = await getProject({ path: { project_ref: projectRef.value } })
      project.value = data ?? null
    } catch (error_) {
      error.value = error_
    } finally {
      loading.value = false
    }
  }

  onMounted(() => {
    redirectIfBareProject()
    void load()
  })
  watch(projectRef, () => {
    redirectIfBareProject()
    void load()
  })
  watch(() => route.name, () => {
    redirectIfBareProject()
  })

  function redirectIfBareProject (): void {
    if (route.name === '/projects/[projectRef]') {
      void router.replace(`/projects/${projectRef.value}/overview`)
    }
  }
</script>
