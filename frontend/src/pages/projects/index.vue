<!-- pages/projects/index.vue — compact project cards: latest version, count, 7-day active -->
<template>
  <div>
    <PageHeader :description="t('projects.description')" :title="t('projects.title')">
      <template #actions>
        <v-btn v-if="auth.isPlatformAdmin" color="primary" prepend-icon="mdi-plus" @click="openCreate">
          {{ t('projects.create') }}
        </v-btn>
      </template>
    </PageHeader>

    <ErrorAlert :error="resource.error.value" />

    <div v-if="resource.loading.value" class="d-flex justify-center py-12">
      <v-progress-circular color="primary" indeterminate />
    </div>

    <EmptyState v-else-if="resource.items.value.length === 0" icon="mdi-view-dashboard-outline" :text="t('common.empty')">
      <template #actions>
        <v-btn v-if="auth.isPlatformAdmin" color="primary" @click="openCreate">{{ t('projects.create') }}</v-btn>
      </template>
    </EmptyState>

    <template v-else>
      <v-row class="mb-2" dense>
        <v-col
          v-for="tile in fleetTiles"
          :key="tile.label"
          cols="12"
          sm="4"
        >
          <v-card :color="tile.color" variant="tonal">
            <v-card-text class="d-flex align-center ga-3 py-3">
              <v-avatar :color="tile.color" size="40" variant="flat">
                <v-icon :icon="tile.icon" />
              </v-avatar>

              <div>
                <div class="text-caption text-medium-emphasis">{{ tile.label }}</div>
                <div class="text-h6 text-truncate">{{ tile.value }}</div>
              </div>
            </v-card-text>
          </v-card>
        </v-col>
      </v-row>

      <v-text-field
        v-model="searchQuery"
        class="mb-4"
        clearable
        density="compact"
        hide-details
        :label="t('common.search')"
        :placeholder="t('projects.searchPlaceholder')"
        prepend-inner-icon="mdi-magnify"
        style="max-width: 360px"
      />

      <EmptyState
        v-if="filteredProjects.length === 0"
        icon="mdi-magnify"
        :text="t('projects.searchNoMatch')"
      />

      <v-row v-else>
        <v-col
          v-for="project in filteredProjects"
          :key="project.uuid"
          cols="12"
          lg="4"
          sm="6"
        >
          <v-card class="d-flex flex-column" hover>
            <v-card-item>
              <template #prepend>
                <v-avatar color="primary" variant="tonal">
                  <span class="font-weight-bold">{{ initials(project) }}</span>
                </v-avatar>
              </template>

              <v-card-title class="text-wrap">{{ project.name || project.slug }}</v-card-title>

              <v-card-subtitle class="d-flex flex-wrap align-center ga-1">
                <span>{{ project.slug }}</span>

                <v-chip label size="x-small" variant="tonal">
                  {{ t(`projects.compareEngine${project.compare_engine === 'integer' ? 'Integer' : 'Semver'}`) }}
                </v-chip>
              </v-card-subtitle>

              <template #append>
                <v-btn
                  color="error"
                  icon="mdi-delete-outline"
                  size="small"
                  variant="text"
                  @click.stop="askDelete(project)"
                />
              </template>
            </v-card-item>

            <v-card-text>
              <div class="d-flex flex-wrap align-center ga-2 mb-3">
                <span class="text-caption text-medium-emphasis">{{ t('projects.latestVersion') }}</span>

                <template v-if="project.stats?.latest_version">
                  <span class="font-weight-medium">{{ project.stats.latest_version.version }}</span>

                  <v-chip
                    v-if="project.stats.latest_version.channel"
                    label
                    size="x-small"
                    variant="tonal"
                  >
                    {{ project.stats.latest_version.channel }}
                  </v-chip>

                  <StatusChip
                    v-if="project.stats.latest_version.status"
                    kind="version"
                    :status="project.stats.latest_version.status"
                  />
                </template>

                <span v-else class="text-medium-emphasis">{{ t('projects.noVersion') }}</span>
              </div>

              <div class="d-flex flex-wrap ga-4">
                <div>
                  <div class="text-subtitle-1 font-weight-medium">{{ project.stats?.versions?.total ?? 0 }}</div>
                  <div class="text-caption text-medium-emphasis">{{ t('projects.metricVersions') }}</div>
                </div>

                <div>
                  <div class="text-subtitle-1 font-weight-medium">{{ project.stats?.active_7d ?? 0 }}</div>
                  <div class="text-caption text-medium-emphasis">{{ t('projects.active7d') }}</div>
                </div>
              </div>
            </v-card-text>

            <v-card-actions>
              <v-btn
                block
                color="primary"
                prepend-icon="mdi-arrow-right-thin-circle-outline"
                variant="tonal"
                @click="router.push(`/projects/${project.slug ?? ''}/overview`)"
              >
                {{ t('projects.openConsole') }}
              </v-btn>
            </v-card-actions>
          </v-card>
        </v-col>
      </v-row>
    </template>

    <v-dialog v-model="dialog" max-width="560">
      <v-card :title="t('projects.create')">
        <v-card-text>
          <ErrorAlert :error="formError" />

          <v-text-field
            v-model="displayName"
            :label="t('projects.name')"
          />

          <v-text-field
            v-model="slug"
            hint="^[a-zA-Z0-9_-]{3,64}$"
            :label="t('projects.slug')"
            persistent-hint
            :rules="[required, slugRule]"
          />

          <v-select
            v-model="compareEngine"
            :items="[
              { title: t('projects.compareEngineSemver'), value: 'semver' },
              { title: t('projects.compareEngineInteger'), value: 'integer' },
            ]"
            :label="t('projects.compareEngine')"
          />

          <v-alert class="mt-2" density="compact" type="info">
            {{ t('projects.engineLocked') }}
          </v-alert>

          <v-text-field
            v-model="ownerUsername"
            class="mt-4"
            :hint="t('projects.ownerUsernameHint')"
            :label="t('projects.ownerUsername')"
            persistent-hint
          />

          <v-text-field
            v-model="ownerPassword"
            :disabled="!ownerUsername.trim()"
            :hint="t('projects.ownerPasswordHint')"
            :label="t('projects.ownerPassword')"
            persistent-hint
            type="password"
          />
        </v-card-text>

        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="dialog = false">{{ t('common.cancel') }}</v-btn>
          <v-btn color="primary" :loading="saving" @click="save">{{ t('common.create') }}</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <ConfirmDialog
      v-model="deleteDialog"
      :confirm-label="t('projects.confirmName')"
      :confirm-word="deleteTarget?.slug"
      icon="mdi-delete-alert-outline"
      :text="deleteTarget ? t('projects.deleteText', { name: deleteTarget.name || deleteTarget.slug }) : ''"
      :title="t('projects.deleteTitle')"
      @confirm="doDelete"
    />
  </div>
</template>

<script lang="ts" setup>
  import type { Project } from '@/api/generated'
  import { computed, ref } from 'vue'
  import { useI18n } from 'vue-i18n'
  import { useRouter } from 'vue-router'
  import { isApiError } from '@/api/client'
  import { createProject, deleteProject, listProjects } from '@/api/generated'
  import ConfirmDialog from '@/components/ConfirmDialog.vue'
  import EmptyState from '@/components/EmptyState.vue'
  import ErrorAlert from '@/components/ErrorAlert.vue'
  import PageHeader from '@/components/PageHeader.vue'
  import StatusChip from '@/components/StatusChip.vue'
  import { useApiResource } from '@/composables/useApiResource'
  import { useAuthStore } from '@/stores/auth'
  import { useSnackbarStore } from '@/stores/snackbar'
  import { useUiStore } from '@/stores/ui'

  const { t } = useI18n()
  const router = useRouter()
  const snackbar = useSnackbarStore()
  const ui = useUiStore()
  const auth = useAuthStore()

  const resource = useApiResource<Project>(async () => {
    const { data } = await listProjects()
    return data?.projects ?? []
  })

  const dialog = ref(false)
  const displayName = ref('')
  const slug = ref('')
  const compareEngine = ref('semver')
  const ownerUsername = ref('')
  const ownerPassword = ref('')
  const saving = ref(false)
  const formError = ref<unknown>(null)

  const deleteDialog = ref(false)
  const deleteTarget = ref<Project | null>(null)
  const searchQuery = ref('')

  const filteredProjects = computed(() => {
    const query = (searchQuery.value ?? '').trim().toLowerCase()
    if (!query) return resource.items.value
    return resource.items.value.filter(project => {
      const projectName = (project.name ?? '').toLowerCase()
      const projectSlug = (project.slug ?? '').toLowerCase()
      return projectName.includes(query) || projectSlug.includes(query)
    })
  })

  const fleetTiles = computed(() => {
    let versions = 0
    let active = 0
    for (const project of resource.items.value) {
      versions += project.stats?.versions?.total ?? 0
      active += project.stats?.active_7d ?? 0
    }
    return [
      { color: 'primary', icon: 'mdi-view-dashboard-outline', label: t('projects.fleetProjects'), value: String(resource.items.value.length) },
      { color: 'info', icon: 'mdi-tag-multiple-outline', label: t('projects.fleetVersions'), value: String(versions) },
      { color: 'success', icon: 'mdi-cellphone-link', label: t('projects.fleetActive'), value: String(active) },
    ]
  })

  function required (value: string): boolean | string {
    return Boolean(value) || t('common.required')
  }

  function slugRule (value: string): boolean | string {
    return /^[\w-]{3,64}$/.test(value) || '^[a-zA-Z0-9_-]{3,64}$'
  }

  function openCreate (): void {
    displayName.value = ''
    slug.value = ''
    compareEngine.value = 'semver'
    ownerUsername.value = ''
    ownerPassword.value = ''
    formError.value = null
    dialog.value = true
  }

  async function save (): Promise<void> {
    saving.value = true
    formError.value = null
    try {
      const body: {
        slug: string
        compare_engine: 'semver' | 'integer'
        default_locale: string
        name?: string
        owner_username?: string
        owner_password?: string
        system_channel_names: { alpha: string, beta: string, stable: string }
      } = {
        slug: slug.value,
        compare_engine: compareEngine.value as 'semver' | 'integer',
        default_locale: ui.locale,
        system_channel_names: {
          alpha: t('channels.seedAlpha'),
          beta: t('channels.seedBeta'),
          stable: t('channels.seedStable'),
        },
      }
      const trimmedName = displayName.value.trim()
      if (trimmedName) body.name = trimmedName
      const owner = ownerUsername.value.trim()
      if (owner) {
        body.owner_username = owner
        const password = ownerPassword.value.trim()
        if (password) body.owner_password = password
      }
      await createProject({ body })
      snackbar.show(t('projects.create') + ' ✓')
      dialog.value = false
      await resource.refresh()
    } catch (error) {
      formError.value = error
    } finally {
      saving.value = false
    }
  }

  function askDelete (project: Project): void {
    deleteTarget.value = project
    deleteDialog.value = true
  }

  async function doDelete (): Promise<void> {
    if (!deleteTarget.value) return
    try {
      await deleteProject({ path: { project_ref: deleteTarget.value.slug ?? deleteTarget.value.uuid ?? '' } })
      snackbar.show(t('common.delete') + ' ✓')
      await resource.refresh()
    } catch (error) {
      snackbar.show(errText(error), 'error', 5000)
    }
  }

  function errText (err: unknown): string {
    return isApiError(err) ? `${err.code}: ${err.message}` : String(err)
  }

  function initials (project: Project): string {
    const source = (project.name || project.slug || '').trim()
    return source.slice(0, 2).toUpperCase()
  }
</script>
