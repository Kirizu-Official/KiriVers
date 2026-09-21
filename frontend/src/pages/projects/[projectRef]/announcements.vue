<!-- pages/projects/[projectRef]/announcements.vue — 项目公告 CRUD + 客户端 API 测试 -->
<template>
  <div>
    <PageHeader :description="t('announcements.description')" :title="t('announcements.title')">
      <template #actions>
        <v-btn prepend-icon="mdi-api" variant="tonal" @click="previewOpen = true">
          {{ t('announcements.testClient') }}
        </v-btn>

        <v-btn color="primary" prepend-icon="mdi-plus" @click="openCreate">{{ t('announcements.create') }}</v-btn>
      </template>
    </PageHeader>

    <ErrorAlert :error="resource.error.value" />

    <EmptyState v-if="!resource.loading.value && resource.items.value.length === 0" icon="mdi-bullhorn-outline" :text="t('announcements.empty')">
      <template #actions>
        <v-btn color="primary" @click="openCreate">{{ t('announcements.create') }}</v-btn>
      </template>
    </EmptyState>

    <v-card v-else>
      <v-data-table
        :headers="headers"
        hide-default-footer
        :items="resource.items.value"
        items-key="id"
        :loading="resource.loading.value"
      >
        <template #item.title="{ item }">
          <span class="font-weight-medium">{{ displayTitle(item) }}</span>
        </template>

        <template #item.language="{ item }">
          {{ languageLabelFor(item.language) }}
        </template>

        <template #item.scope="{ item }">
          {{ scopeLabel(item) }}
        </template>

        <template #item.status="{ item }">
          <StatusChip kind="announcement" :status="displayStatus(item)" />
        </template>

        <template #item.window="{ item }">
          <span class="text-body-2">{{ windowLabel(item) }}</span>
        </template>

        <template #item.actions="{ item }">
          <v-btn
            :aria-label="t('announcements.moveUp')"
            :disabled="isFirst(item)"
            icon="mdi-arrow-up"
            size="small"
            variant="text"
            @click="move(item, -1)"
          />

          <v-btn
            :aria-label="t('announcements.moveDown')"
            :disabled="isLast(item)"
            icon="mdi-arrow-down"
            size="small"
            variant="text"
            @click="move(item, 1)"
          />

          <v-btn prepend-icon="mdi-pencil-outline" size="small" variant="text" @click="openEdit(item)">
            {{ t('common.edit') }}
          </v-btn>

          <v-btn
            color="error"
            prepend-icon="mdi-delete-outline"
            size="small"
            variant="text"
            @click="askDelete(item)"
          >
            {{ t('common.delete') }}
          </v-btn>
        </template>
      </v-data-table>
    </v-card>

    <v-dialog v-model="dialog" max-width="960">
      <v-card :title="editing ? t('common.edit') : t('announcements.create')">
        <v-card-text>
          <ErrorAlert :error="formError" />

          <v-select
            v-model="form.scope"
            :items="scopeItems"
            :label="t('announcements.scope')"
            @update:model-value="onScopeChange"
          />

          <v-select
            v-if="needsVersion"
            v-model="form.versionId"
            item-title="title"
            item-value="value"
            :items="versionOptions"
            :label="t('announcements.version')"
            :rules="[required]"
          />

          <v-select
            v-if="form.scope === 'os' || form.scope === 'version_os'"
            v-model="form.os"
            :items="osItems"
            :label="t('announcements.os')"
            :rules="[required]"
          />

          <v-select
            v-if="form.scope === 'arch' || form.scope === 'version_arch'"
            v-model="form.arch"
            :items="archItems"
            :label="t('announcements.arch')"
            :rules="[required]"
          />

          <v-select
            v-if="form.scope === 'platform'"
            v-model="form.matrixPair"
            :items="matrixPairItems"
            :label="t('announcements.scopePlatform')"
            :rules="[required]"
          />

          <v-select
            class="mt-2"
            :hint="t('languages.editorHint')"
            item-title="title"
            item-value="code"
            :items="localeItems"
            :label="t('languages.locale')"
            :model-value="form.language"
            persistent-hint
            :rules="[required]"
            @update:model-value="onSelectLanguage"
          />

          <v-text-field v-model="form.title" class="mt-3" :label="t('announcements.titleField')" :rules="[required]" />
          <v-text-field v-model="form.subtitle" :label="t('announcements.subtitle')" />

          <MarkdownEditor
            ref="editorRef"
            v-model="form.content"
            class="mb-2"
            :disabled="Boolean(editing) && !bodyReady"
            :min-height="240"
            :placeholder="t('announcements.markdown')"
            :project-ref="projectRef"
          />

          <v-select
            v-model="form.status"
            :items="statusItems"
            :label="t('announcements.status')"
          />

          <DateTimeField
            v-if="form.status === 'scheduled'"
            v-model="form.startsAt"
            :label="t('announcements.startsAt')"
          />

          <DateTimeField v-model="form.endsAt" :label="t('announcements.endsAt')" />
        </v-card-text>

        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="dialog = false">{{ t('common.cancel') }}</v-btn>
          <v-btn color="primary" :disabled="Boolean(editing) && !bodyReady" :loading="saving" @click="save">{{ t('common.save') }}</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <ConfirmDialog
      v-model="deleteDialog"
      icon="mdi-delete-alert-outline"
      :text="deleteTarget ? t('announcements.deleteText', { title: displayTitle(deleteTarget) }) : ''"
      :title="t('announcements.deleteTitle')"
      @confirm="doDelete"
    />

    <AnnouncementPreviewDialog v-model="previewOpen" :languages="langResource.items.value" :project-ref="projectRef" />
  </div>
</template>

<script lang="ts" setup>
  import type { AdminAnnouncement, PlatformMatrix } from '@/api/generated'
  import { computed, onMounted, reactive, ref, watch } from 'vue'
  import { useI18n } from 'vue-i18n'
  import { useRoute } from 'vue-router'
  import {
    createAnnouncement,
    deleteAnnouncement,
    getAnnouncement,
    listAnnouncements,
    listMatrix,
    listVersions,
    patchAnnouncement,
    reorderAnnouncements,
  } from '@/api/generated'
  import AnnouncementPreviewDialog from '@/components/AnnouncementPreviewDialog.vue'
  import ConfirmDialog from '@/components/ConfirmDialog.vue'
  import DateTimeField from '@/components/DateTimeField.vue'
  import EmptyState from '@/components/EmptyState.vue'
  import ErrorAlert from '@/components/ErrorAlert.vue'
  import MarkdownEditor from '@/components/MarkdownEditor.vue'
  import PageHeader from '@/components/PageHeader.vue'
  import StatusChip from '@/components/StatusChip.vue'
  import { useApiResource } from '@/composables/useApiResource'
  import { languageLabel, useProjectLanguages } from '@/composables/useProjectLanguages'
  import { useSnackbarStore } from '@/stores/snackbar'

  type ScopeKind = 'project' | 'version' | 'os' | 'arch' | 'version_os' | 'version_arch' | 'platform'
  type VersionOption = { title: string, value: string }

  const { t } = useI18n()
  const route = useRoute('/projects/[projectRef]/announcements')
  const snackbar = useSnackbarStore()
  const projectRef = computed(() => route.params.projectRef)
  const langResource = useProjectLanguages(projectRef)
  const resource = useApiResource<AdminAnnouncement>(async () => {
    const { data } = await listAnnouncements({ path: { project_ref: projectRef.value } })
    return data?.announcements ?? []
  })

  const headers = computed(() => [
    { title: t('announcements.titleField'), key: 'title' },
    { title: t('languages.locale'), key: 'language' },
    { title: t('announcements.scope'), key: 'scope' },
    { title: t('announcements.status'), key: 'status' },
    { title: t('announcements.startsAt'), key: 'window' },
    { title: t('common.actions'), key: 'actions', sortable: false, align: 'end' as const },
  ])

  const scopeItems = computed(() => [
    { title: t('announcements.scopeProject'), value: 'project' },
    { title: t('announcements.scopeVersion'), value: 'version' },
    { title: t('announcements.scopeOs'), value: 'os' },
    { title: t('announcements.scopeArch'), value: 'arch' },
    { title: t('announcements.scopeVersionOs'), value: 'version_os' },
    { title: t('announcements.scopeVersionArch'), value: 'version_arch' },
    { title: t('announcements.scopePlatform'), value: 'platform' },
  ])
  const statusItems = computed(() => [
    { title: t('announcements.statusDraft'), value: 'draft' },
    { title: t('announcements.statusScheduled'), value: 'scheduled' },
    { title: t('announcements.statusPublished'), value: 'published' },
  ])

  const dialog = ref(false)
  const previewOpen = ref(false)
  const editing = ref<AdminAnnouncement | null>(null)
  const saving = ref(false)
  const formError = ref<unknown>(null)
  const deleteDialog = ref(false)
  const deleteTarget = ref<AdminAnnouncement | null>(null)
  const versionOptions = ref<VersionOption[]>([])
  const matrixRows = ref<PlatformMatrix[]>([])
  const editorRef = ref<{ flush: () => string } | null>(null)
  const bodyReady = ref(true)
  const osItems = computed(() => Array.from(new Set(matrixRows.value.flatMap(row => row.os ? [row.os] : []))))
  const archItems = computed(() => Array.from(new Set(matrixRows.value.flatMap(row => row.arch ? [row.arch] : []))))
  const matrixPairItems = computed(() => matrixRows.value.flatMap(row => {
    if (!row.os || !row.arch) return []
    return [{ title: `${row.os} / ${row.arch}`, value: `${row.os}|${row.arch}` }]
  }))

  const localeItems = computed(() => langResource.items.value.map(row => ({
    title: languageLabel(row),
    code: row.code ?? '',
  })).filter(item => item.code))

  const form = reactive({
    scope: 'project' as ScopeKind,
    versionId: '',
    os: '',
    arch: '',
    matrixPair: '',
    language: '',
    title: '',
    subtitle: '',
    content: '',
    status: 'draft',
    startsAt: '',
    endsAt: '',
  })
  const needsVersion = computed(() => ['version', 'version_os', 'version_arch', 'platform'].includes(form.scope))

  function pickDefaultLocale (): string {
    const items = langResource.items.value
    const def = items.find(row => row.is_default)?.code
    return def || items[0]?.code || ''
  }

  function languageLabelFor (code: string | undefined): string {
    const row = langResource.items.value.find(item => item.code === code)
    return row ? languageLabel(row) : (code || '')
  }

  function onSelectLanguage (code: unknown): void {
    form.language = typeof code === 'string' ? code : ''
  }

  watch(() => langResource.items.value, () => {
    if (!dialog.value || form.language) return
    form.language = pickDefaultLocale()
  })

  function required (value: string): boolean | string {
    return Boolean(value) || t('common.required')
  }

  function displayTitle (item: AdminAnnouncement): string {
    const title = (item.title ?? '').trim()
    return title || item.id || ''
  }

  function inferScope (item: AdminAnnouncement): ScopeKind {
    const hasV = Boolean(item.version_id)
    const hasOs = Boolean(item.os)
    const hasArch = Boolean(item.arch)
    if (hasV && hasOs && hasArch) return 'platform'
    if (hasV && hasOs) return 'version_os'
    if (hasV && hasArch) return 'version_arch'
    if (hasV) return 'version'
    if (hasOs) return 'os'
    if (hasArch) return 'arch'
    return 'project'
  }

  function versionTitle (id: string | undefined | null): string {
    if (!id) return ''
    return versionOptions.value.find(row => row.value === id)?.title ?? id.slice(0, 8)
  }

  function scopeLabel (item: AdminAnnouncement): string {
    const scope = inferScope(item)
    const version = versionTitle(item.version_id)
    if (scope === 'version') return `${t('announcements.scopeVersion')} ${version}`
    if (scope === 'os') return `${t('announcements.scopeOs')} ${item.os ?? ''}`
    if (scope === 'arch') return `${t('announcements.scopeArch')} ${item.arch ?? ''}`
    if (scope === 'version_os') return `${version} / ${item.os ?? ''}`
    if (scope === 'version_arch') return `${version} / ${item.arch ?? ''}`
    if (scope === 'platform') return `${version} / ${item.os ?? ''} / ${item.arch ?? ''}`
    return t('announcements.scopeProject')
  }

  function displayStatus (item: AdminAnnouncement): string {
    if (item.status === 'draft' || item.status === 'scheduled') return item.status
    if (item.ends_at && Date.parse(item.ends_at) <= Date.now()) return 'expired'
    return 'published'
  }

  function windowLabel (item: AdminAnnouncement): string {
    if (!item.starts_at && !item.ends_at) return t('announcements.windowNone')
    return [item.starts_at ?? '—', item.ends_at ?? '—'].join(' → ')
  }

  function isFirst (item: AdminAnnouncement): boolean {
    return resource.items.value[0]?.id === item.id
  }
  function isLast (item: AdminAnnouncement): boolean {
    const items = resource.items.value
    return items.at(-1)?.id === item.id
  }

  function resetForm (): void {
    Object.assign(form, {
      scope: 'project',
      versionId: '',
      os: '',
      arch: '',
      matrixPair: '',
      language: pickDefaultLocale(),
      title: '',
      subtitle: '',
      content: '',
      status: 'draft',
      startsAt: '',
      endsAt: '',
    })
    formError.value = null
  }

  function onScopeChange (): void {
    form.versionId = ''
    form.os = ''
    form.arch = ''
    form.matrixPair = ''
  }

  async function loadOptions (): Promise<void> {
    try {
      const [{ data: versions }, { data: matrix }] = await Promise.all([
        listVersions({ path: { project_ref: projectRef.value } }),
        listMatrix({ path: { project_ref: projectRef.value } }),
      ])
      versionOptions.value = (versions?.versions ?? []).flatMap(item => {
        if (!item.id) return []
        const title = item.version_semver ?? String(item.version_integer ?? item.id)
        return [{ title, value: item.id }]
      })
      matrixRows.value = matrix?.matrix ?? []
    } catch {
      versionOptions.value = []
      matrixRows.value = []
    }
  }

  function applyRow (item: AdminAnnouncement): void {
    Object.assign(form, {
      scope: inferScope(item),
      versionId: item.version_id ?? '',
      os: item.os ?? '',
      arch: item.arch ?? '',
      matrixPair: item.os && item.arch ? `${item.os}|${item.arch}` : '',
      language: item.language ?? pickDefaultLocale(),
      title: item.title ?? '',
      subtitle: item.subtitle ?? '',
      content: item.content ?? '',
      status: item.status ?? 'draft',
      startsAt: item.status === 'scheduled' ? (item.starts_at ?? '') : '',
      endsAt: item.ends_at ?? '',
    })
  }

  function openCreate (): void {
    editing.value = null
    bodyReady.value = true
    resetForm()
    void loadOptions()
    dialog.value = true
  }

  async function openEdit (item: AdminAnnouncement): Promise<void> {
    if (!item.id) return
    editing.value = item
    bodyReady.value = false
    applyRow(item)
    formError.value = null
    void loadOptions()
    dialog.value = true
    saving.value = true
    try {
      const { data } = await getAnnouncement({
        path: { project_ref: projectRef.value, announcement_id: item.id },
      })
      if (!data) {
        throw new Error(t('common.required'))
      }
      editing.value = data
      applyRow(data)
      bodyReady.value = true
    } catch (error) {
      formError.value = error
      bodyReady.value = false
    } finally {
      saving.value = false
    }
  }

  function comboValue (value: unknown): string {
    if (typeof value === 'string') return value.trim()
    if (value == null) return ''
    return String(value).trim()
  }

  function parsePair (value: string): { os: string, arch: string } | undefined {
    const [os, arch] = value.split('|')
    if (!os || !arch) return undefined
    return { os, arch }
  }

  function scopeFields (): { version_id: string | null, os: string | null, arch: string | null } {
    const versionId = comboValue(form.versionId)
    const os = comboValue(form.os)
    const arch = comboValue(form.arch)
    const pair = parsePair(form.matrixPair)
    if (form.scope === 'version') return { version_id: versionId, os: null, arch: null }
    if (form.scope === 'os') return { version_id: null, os, arch: null }
    if (form.scope === 'arch') return { version_id: null, os: null, arch }
    if (form.scope === 'version_os') return { version_id: versionId, os, arch: null }
    if (form.scope === 'version_arch') return { version_id: versionId, os: null, arch }
    if (form.scope === 'platform') return { version_id: versionId, os: pair?.os ?? os, arch: pair?.arch ?? arch }
    return { version_id: null, os: null, arch: null }
  }

  async function save (): Promise<void> {
    if (editing.value && !bodyReady.value) {
      return
    }
    saving.value = true
    formError.value = null
    try {
      const scoped = scopeFields()
      if (needsVersion.value && !scoped.version_id) {
        formError.value = new Error(t('common.required'))
        return
      }
      if ((form.scope === 'os' || form.scope === 'version_os') && !scoped.os) {
        formError.value = new Error(t('common.required'))
        return
      }
      if ((form.scope === 'arch' || form.scope === 'version_arch') && !scoped.arch) {
        formError.value = new Error(t('common.required'))
        return
      }
      if (form.scope === 'platform' && (!scoped.os || !scoped.arch)) {
        formError.value = new Error(t('common.required'))
        return
      }
      if (form.status === 'scheduled' && !form.startsAt.trim()) {
        formError.value = new Error(t('common.required'))
        return
      }
      const language = comboValue(form.language)
      if (!language) {
        formError.value = new Error(t('common.required'))
        return
      }
      const content = editorRef.value?.flush() ?? form.content
      const title = form.title.trim()
      if (!title) {
        formError.value = new Error(t('announcements.titleRequired'))
        return
      }
      const body = {
        ...scoped,
        language,
        title,
        subtitle: form.subtitle,
        content,
        starts_at: form.status === 'scheduled' ? (form.startsAt.trim() || null) : null,
        ends_at: form.endsAt.trim() || null,
      }
      const status = form.status as 'draft' | 'scheduled' | 'published'
      if (editing.value?.id) {
        await patchAnnouncement({
          path: { project_ref: projectRef.value, announcement_id: editing.value.id },
          body: { ...body, status },
        })
      } else {
        const { data: created } = await createAnnouncement({
          path: { project_ref: projectRef.value },
          body,
        })
        if (created?.id && status !== 'draft') {
          await patchAnnouncement({
            path: { project_ref: projectRef.value, announcement_id: created.id },
            body: { status, starts_at: body.starts_at, ends_at: body.ends_at },
          })
        }
      }
      snackbar.show(`${t('common.save')} ✓`)
      dialog.value = false
      await resource.refresh()
    } catch (error) {
      formError.value = error
    } finally {
      saving.value = false
    }
  }

  async function move (item: AdminAnnouncement, delta: number): Promise<void> {
    const items = [...resource.items.value]
    const index = items.findIndex(row => row.id === item.id)
    const next = index + delta
    if (index < 0 || next < 0 || next >= items.length) return
    const current = items[index]
    const other = items[next]
    if (!current || !other) return
    items[index] = other
    items[next] = current
    try {
      await reorderAnnouncements({
        path: { project_ref: projectRef.value },
        body: { ids: items.flatMap(row => row.id ? [row.id] : []) },
      })
      await resource.refresh()
    } catch (error) {
      snackbar.show(String(error), 'error', 5000)
    }
  }

  function askDelete (item: AdminAnnouncement): void {
    deleteTarget.value = item
    deleteDialog.value = true
  }

  async function doDelete (): Promise<void> {
    if (!deleteTarget.value?.id) return
    try {
      await deleteAnnouncement({
        path: { project_ref: projectRef.value, announcement_id: deleteTarget.value.id },
      })
      snackbar.show(`${t('common.delete')} ✓`)
      await resource.refresh()
    } catch (error) {
      snackbar.show(String(error), 'error', 5000)
    }
  }

  onMounted(() => {
    void loadOptions()
  })
  watch(projectRef, () => {
    void loadOptions()
  })
</script>
