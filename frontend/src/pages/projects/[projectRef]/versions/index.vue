<!-- pages/projects/[projectRef]/versions/index.vue — 版本列表 + 创建 + exists 快查 -->
<template>
  <div>
    <PageHeader :title="t('versions.title')">
      <template #actions>
        <v-btn color="primary" prepend-icon="mdi-plus" @click="openCreate">{{ t('versions.create') }}</v-btn>
      </template>
    </PageHeader>

    <div class="d-flex flex-wrap align-center ga-3 mb-4">
      <v-select
        v-model="filter.channel"
        clearable
        density="compact"
        hide-details
        :items="channelOptions"
        :label="t('versions.channel')"
        style="max-width: 180px"
      />

      <v-select
        v-model="filter.status"
        clearable
        density="compact"
        hide-details
        :items="[
          { title: t('versions.statusDraft'), value: 'draft' },
          { title: t('versions.statusPublished'), value: 'published' },
          { title: t('versions.statusDeprecated'), value: 'deprecated' },
          { title: t('versions.statusRevoked'), value: 'revoked' },
        ]"
        :label="t('versions.status')"
        style="max-width: 180px"
      />

      <v-select
        v-model="filter.pair"
        clearable
        density="compact"
        hide-details
        :items="pairOptions"
        :label="t('versions.matrixPair')"
        style="max-width: 240px"
      />

      <v-text-field
        v-model="existsQuery"
        density="compact"
        hide-details
        :label="t('versions.existsCheck')"
        style="max-width: 260px"
        @keyup.enter="checkExists"
      >
        <template #append-inner>
          <v-icon icon="mdi-magnify" size="18" style="cursor: pointer" @click="checkExists" />
        </template>
      </v-text-field>

      <v-chip v-if="existsResult !== null" :color="existsResult ? 'success' : 'grey'" label>
        {{ existsResult ? t('versions.existsYes') : t('versions.existsNo') }}
        <template v-if="existsDetail">{{ existsDetail }}</template>
      </v-chip>
    </div>

    <ErrorAlert :error="resource.error.value" />

    <EmptyState v-if="!resource.loading.value && resource.items.value.length === 0" icon="mdi-tag-multiple-outline" :text="t('common.empty')">
      <template #actions>
        <v-btn color="primary" @click="openCreate">{{ t('versions.create') }}</v-btn>
      </template>
    </EmptyState>

    <v-card v-else>
      <v-data-table :headers="headers" :items="resource.items.value" items-key="id" :loading="resource.loading.value">
        <template #item.version="{ item }">
          <RouterLink class="font-weight-bold text-decoration-none" :to="`/projects/${projectRef}/versions/${displayVersion(item)}`">
            {{ displayVersion(item) }}
          </RouterLink>

          <v-chip
            v-if="item.is_lts"
            class="ml-2"
            color="primary"
            size="x-small"
            variant="tonal"
          >LTS</v-chip>

          <v-chip
            v-if="item.is_critical"
            class="ml-2"
            color="error"
            size="x-small"
            variant="tonal"
          >critical</v-chip>
        </template>

        <template #item.channel="{ item }">
          <v-chip size="small" variant="tonal">{{ item.channel }}</v-chip>
        </template>

        <template #item.status="{ item }">
          <StatusChip kind="version" :status="item.status ?? ''" />
        </template>

        <template #item.gray="{ item }">
          <v-chip
            v-if="item.status !== 'draft' && !item.gray_completed_at"
            color="warning"
            size="small"
            variant="tonal"
          >
            {{ t('versions.grayActive') }} {{ item.actual_percent ?? 0 }}%
          </v-chip>

          <span v-else-if="item.gray_completed_at" class="text-medium-emphasis">{{ t('gray.fullPush') }}</span>
          <span v-else class="text-medium-emphasis">—</span>
        </template>

        <template #item.created_at="{ item }">
          <span class="text-body-2 text-medium-emphasis">{{ item.created_at }}</span>
        </template>

        <template #item.actions="{ item }">
          <v-btn prepend-icon="mdi-text-box-edit-outline" size="small" variant="text" @click="openChangelog(item)">
            {{ t('versions.changelog') }}
          </v-btn>

          <v-btn
            prepend-icon="mdi-package-variant-closed"
            size="small"
            variant="text"
            @click="router.push(`/projects/${projectRef}/versions/${displayVersion(item)}`)"
          >
            {{ t('versions.manageArtifacts') }}
          </v-btn>

          <v-btn
            prepend-icon="mdi-chart-timeline-variant"
            size="small"
            variant="text"
            @click="router.push(`/projects/${projectRef}/versions/${displayVersion(item)}/gray`)"
          >
            {{ t('gray.manage') }}
          </v-btn>
        </template>
      </v-data-table>
    </v-card>

    <v-dialog v-model="dialog" max-width="720">
      <v-card :title="t('versions.create')">
        <v-card-text>
          <ErrorAlert :error="formError" />
          <div class="text-caption text-medium-emphasis mb-2">{{ t('versions.createHint') }}</div>

          <v-row dense>
            <v-col cols="12" md="6">
              <v-text-field
                v-model="form.semver"
                :hint="t('versions.semverHint')"
                :label="t('versions.semver')"
                persistent-hint
                placeholder="1.2.3"
              />
            </v-col>

            <v-col cols="12" md="6">
              <v-text-field
                v-model="form.integer"
                :hint="t('versions.integerHint')"
                :label="t('versions.integer')"
                persistent-hint
                placeholder="102"
                type="number"
              />
            </v-col>

            <v-col cols="12" md="6">
              <v-select
                v-model="form.channel"
                :hint="t('versions.channelHint')"
                :items="channelOptions"
                :label="t('versions.channel')"
                persistent-hint
              />
            </v-col>

            <v-col cols="12" md="6">
              <v-text-field
                v-model="form.git_tag"
                :hint="t('versions.gitTagHint')"
                :label="t('versions.gitTag')"
                persistent-hint
              />
            </v-col>

            <v-col cols="12" md="6">
              <v-text-field
                v-model="form.git_commit"
                :hint="t('versions.gitCommitHint')"
                :label="t('versions.gitCommit')"
                persistent-hint
              />
            </v-col>

            <v-col cols="12" md="6">
              <v-text-field
                v-model="form.min_source_version"
                :hint="t('versions.minSourceHint')"
                :label="t('versions.minSourceVersion')"
                persistent-hint
              />
            </v-col>
          </v-row>

          <div class="text-subtitle-2 mt-4 mb-2">{{ t('versions.changelog') }}</div>

          <LocaleTabs
            ref="createLocaleTabsRef"
            v-model="form.changelog"
            :languages="langResource.items.value"
            :project-ref="projectRef"
            :rows="6"
          />

          <v-row class="mt-2" dense>
            <v-col cols="12" md="4">
              <v-switch
                v-model="form.is_lts"
                color="primary"
                density="compact"
                :hint="t('versions.isLtsHint')"
                :label="t('versions.isLts')"
                persistent-hint
              />
            </v-col>

            <v-col cols="12" md="4">
              <v-switch
                v-model="form.is_critical"
                color="primary"
                density="compact"
                :hint="t('versions.isCriticalHint')"
                :label="t('versions.isCritical')"
                persistent-hint
                @update:model-value="onCriticalToggle"
              />
            </v-col>

            <v-col cols="12" md="4">
              <v-slider
                v-model="form.gray_start_percent"
                density="compact"
                :disabled="form.is_critical"
                :hint="t('gray.startHint')"
                :label="t('gray.start')"
                :max="100"
                :min="0"
                persistent-hint
                thumb-label
              />
            </v-col>

            <v-col cols="12" md="4">
              <v-slider
                v-model="form.gray_step_percent"
                density="compact"
                :disabled="form.is_critical"
                :hint="t('gray.stepHint')"
                :label="t('gray.step')"
                :max="100"
                :min="0"
                persistent-hint
                thumb-label
              />
            </v-col>

            <v-col cols="12" md="4">
              <v-text-field
                v-model.number="form.gray_interval_seconds"
                density="compact"
                :disabled="form.is_critical"
                :hint="t('gray.intervalHint')"
                :label="t('gray.interval')"
                persistent-hint
                type="number"
              />
            </v-col>
          </v-row>
        </v-card-text>

        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="dialog = false">{{ t('common.cancel') }}</v-btn>
          <v-btn color="primary" :loading="saving" @click="save">{{ t('common.create') }}</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <v-dialog v-model="changelogDialog" max-width="720">
      <v-card :title="t('versions.changelog')">
        <v-card-text>
          <ErrorAlert :error="changelogError" />

          <div v-if="changelogLoading" class="d-flex justify-center py-8">
            <v-progress-circular color="primary" indeterminate />
          </div>

          <template v-else>
            <LocaleTabs
              ref="changelogLocaleTabsRef"
              v-model="changelogDraft"
              :languages="langResource.items.value"
              :project-ref="projectRef"
              :rows="8"
            />
          </template>
        </v-card-text>

        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="changelogDialog = false">{{ t('common.close') }}</v-btn>

          <v-btn
            color="primary"
            :disabled="changelogLoading || !changelogTarget"
            :loading="changelogSaving"
            @click="saveChangelog"
          >
            {{ t('common.save') }}
          </v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>
  </div>
</template>

<script lang="ts" setup>
  import type { Version } from '@/api/generated'
  import { computed, reactive, ref, watch } from 'vue'
  import { useI18n } from 'vue-i18n'
  import { useRoute, useRouter } from 'vue-router'
  import {
    getVersion,
    listChannels,
    listMatrix,
    listVersions,
    patchVersion,
    putVersion,
    versionExists,
  } from '@/api/generated'
  import { changelogDraftToI18n, flattenChangelogMap } from '@/components/changelogMap'
  import EmptyState from '@/components/EmptyState.vue'
  import ErrorAlert from '@/components/ErrorAlert.vue'
  import LocaleTabs from '@/components/LocaleTabs.vue'
  import PageHeader from '@/components/PageHeader.vue'
  import StatusChip from '@/components/StatusChip.vue'
  import { useApiResource } from '@/composables/useApiResource'
  import { useProjectLanguages } from '@/composables/useProjectLanguages'
  import { useSnackbarStore } from '@/stores/snackbar'

  const { t } = useI18n()
  const route = useRoute('/projects/[projectRef]/versions/')
  const router = useRouter()
  const snackbar = useSnackbarStore()

  const projectRef = computed(() => route.params.projectRef)
  const langResource = useProjectLanguages(projectRef)
  const filter = reactive({ channel: '', status: '', pair: '' })

  const resource = useApiResource<Version>(async () => {
    const pair = parsePair(filter.pair)
    const { data } = await listVersions({
      path: { project_ref: projectRef.value },
      query: {
        channel: filter.channel || undefined,
        status: filter.status || undefined,
        os: pair?.os,
        arch: pair?.arch,
      },
    })
    return data?.versions ?? []
  })

  watch(() => [filter.channel, filter.status, filter.pair], () => {
    void resource.refresh()
  })

  const channelOptions = ref<string[]>(['stable', 'beta', 'alpha'])
  const pairOptions = ref<Array<{ title: string, value: string }>>([])
  void listChannels({ path: { project_ref: projectRef.value } }).then(({ data }) => {
    channelOptions.value = (data?.channels ?? []).flatMap(c => c.slug ? [c.slug] : [])
  }).catch(() => undefined)
  void listMatrix({ path: { project_ref: projectRef.value } }).then(({ data }) => {
    pairOptions.value = (data?.matrix ?? []).flatMap(row => {
      if (!row.os || !row.arch) return []
      return [{ title: `${row.os} / ${row.arch}`, value: `${row.os}|${row.arch}` }]
    })
  }).catch(() => undefined)

  const headers = computed(() => [
    { title: t('versions.version'), key: 'version' },
    { title: t('versions.channel'), key: 'channel' },
    { title: t('versions.status'), key: 'status' },
    { title: t('gray.title'), key: 'gray' },
    { title: t('versions.publishTime'), key: 'publish_time' },
    { title: t('projects.createdAt'), key: 'created_at' },
    { title: t('common.actions'), key: 'actions', sortable: false, align: 'end' as const },
  ])

  const existsQuery = ref('')
  const existsResult = ref<boolean | null>(null)
  const existsDetail = ref('')

  function parsePair (value: string): { os: string, arch: string } | undefined {
    const [os, arch] = value.split('|')
    if (!os || !arch) return undefined
    return { os, arch }
  }

  async function checkExists (): Promise<void> {
    if (!existsQuery.value) return
    try {
      const { data: result } = await versionExists({
        path: { project_ref: projectRef.value, version: existsQuery.value },
      })
      existsResult.value = Boolean(result?.exists)
      existsDetail.value = result?.exists ? ` · ${result.channel ?? ''} · ${result.status ?? ''}` : ''
    } catch {
      existsResult.value = null
      existsDetail.value = ''
    }
  }

  function displayVersion (item: Version): string {
    return item.version_semver ?? String(item.version_integer ?? item.id?.slice(0, 8) ?? '')
  }

  const dialog = ref(false)
  const saving = ref(false)
  const formError = ref<unknown>(null)
  const createLocaleTabsRef = ref<{ flush: () => Record<string, string> } | null>(null)
  const changelogLocaleTabsRef = ref<{ flush: () => Record<string, string> } | null>(null)
  const form = reactive({
    semver: '',
    integer: '',
    channel: 'stable',
    git_tag: '',
    git_commit: '',
    min_source_version: '',
    changelog: {} as Record<string, string> | null,
    is_lts: false,
    is_critical: false,
    gray_start_percent: 30,
    gray_step_percent: 10,
    gray_interval_seconds: 3600,
  })

  function onCriticalToggle (value: boolean | null): void {
    if (value) {
      form.gray_start_percent = 100
      return
    }
    form.gray_start_percent = 30
    form.gray_step_percent = 10
  }

  function openCreate (): void {
    form.semver = ''
    form.integer = ''
    form.channel = 'stable'
    form.git_tag = ''
    form.git_commit = ''
    form.min_source_version = ''
    form.changelog = {}
    form.is_lts = false
    form.is_critical = false
    form.gray_start_percent = 30
    form.gray_step_percent = 10
    form.gray_interval_seconds = 3600
    formError.value = null
    dialog.value = true
  }

  async function save (): Promise<void> {
    const identity = form.semver || form.integer
    if (!identity) {
      formError.value = new Error(t('versions.identityMissing'))
      return
    }
    saving.value = true
    formError.value = null
    try {
      const changelog = createLocaleTabsRef.value?.flush() ?? form.changelog
      form.changelog = changelog
      const { data: created } = await putVersion({
        path: { project_ref: projectRef.value, version: identity },
        body: {
          channel: form.channel,
          version_integer: form.integer ? Number(form.integer) : undefined,
          version_semver: form.semver || undefined,
          changelog_i18n: changelogDraftToI18n(changelog),
          git_tag: form.git_tag || undefined,
          git_commit: form.git_commit || undefined,
          min_source_version: form.min_source_version || undefined,
          is_lts: form.is_lts,
          is_critical: form.is_critical,
          gray_start_percent: Math.round(form.gray_start_percent),
          gray_step_percent: Math.round(form.gray_step_percent),
          gray_interval_seconds: Math.round(Number(form.gray_interval_seconds)) || 3600,
        },
      })
      snackbar.show(t('versions.create') + ' ✓')
      dialog.value = false
      await router.push(`/projects/${projectRef.value}/versions/${displayVersion(created ?? {})}`)
    } catch (error) {
      formError.value = error
    } finally {
      saving.value = false
    }
  }

  const changelogDialog = ref(false)
  const changelogLoading = ref(false)
  const changelogSaving = ref(false)
  const changelogError = ref<unknown>(null)
  const changelogTarget = ref<Version | null>(null)
  const changelogDraft = ref<Record<string, string> | null>({})

  async function openChangelog (item: Version): Promise<void> {
    changelogTarget.value = item
    changelogDraft.value = {}
    changelogError.value = null
    changelogDialog.value = true
    changelogLoading.value = true
    try {
      const { data } = await getVersion({
        path: { project_ref: projectRef.value, version: displayVersion(item) },
      })
      changelogTarget.value = data ?? item
      changelogDraft.value = flattenChangelogMap(data?.changelog)
    } catch (error) {
      changelogError.value = error
    } finally {
      changelogLoading.value = false
    }
  }

  async function saveChangelog (): Promise<void> {
    if (!changelogTarget.value) return
    changelogSaving.value = true
    changelogError.value = null
    try {
      const changelog = changelogLocaleTabsRef.value?.flush() ?? changelogDraft.value
      changelogDraft.value = changelog
      await patchVersion({
        path: { project_ref: projectRef.value, version: displayVersion(changelogTarget.value) },
        body: { changelog_i18n: changelogDraftToI18n(changelog) },
      })
      snackbar.show(t('common.save') + ' ✓')
      changelogDialog.value = false
      await resource.refresh()
    } catch (error) {
      changelogError.value = error
    } finally {
      changelogSaving.value = false
    }
  }
</script>
