<!-- pages/geoip.vue — 平台 GeoIP MMDB：上传、启用、调序 -->
<template>
  <div>
    <PageHeader :description="t('geoip.description')" :title="t('geoip.title')">
      <template #actions>
        <v-btn color="primary" prepend-icon="mdi-upload" @click="openUpload">
          {{ t('geoip.upload') }}
        </v-btn>
      </template>
    </PageHeader>

    <ErrorAlert :error="resource.error.value" />

    <EmptyState v-if="!resource.loading.value && resource.items.value.length === 0" icon="mdi-earth" :text="t('common.empty')">
      <template #actions>
        <v-btn color="primary" @click="openUpload">{{ t('geoip.upload') }}</v-btn>
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
        <template #item.size="{ item }">
          {{ formatBytes(item.size ?? 0) }}
        </template>

        <template #item.enabled="{ item }">
          <v-switch
            color="primary"
            density="compact"
            hide-details
            :model-value="item.enabled"
            @update:model-value="value => toggleEnabled(item, Boolean(value))"
          />
        </template>

        <template #item.actions="{ item }">
          <v-btn
            :aria-label="t('geoip.moveUp')"
            :disabled="isFirst(item)"
            icon="mdi-arrow-up"
            size="small"
            variant="text"
            @click="move(item, -1)"
          />

          <v-btn
            :aria-label="t('geoip.moveDown')"
            :disabled="isLast(item)"
            icon="mdi-arrow-down"
            size="small"
            variant="text"
            @click="move(item, 1)"
          />

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

    <v-dialog v-model="dialog" max-width="480">
      <v-card :title="t('geoip.upload')">
        <v-card-text>
          <ErrorAlert :error="formError" />

          <v-file-input
            v-model="file"
            accept=".mmdb,application/octet-stream"
            :hint="t('geoip.fileHint')"
            :label="t('geoip.file')"
            persistent-hint
            prepend-icon=""
            prepend-inner-icon="mdi-database"
            show-size
          />

          <v-text-field v-model="form.name" :label="t('geoip.name')" />

          <v-text-field
            v-model.number="form.rank"
            :hint="t('geoip.rankHint')"
            :label="t('geoip.rank')"
            persistent-hint
            type="number"
          />

          <UploadProgress
            :active="saving"
            :bytes-per-sec="uploadProgress.bytesPerSec"
            :loaded="uploadProgress.loaded"
            :progress="uploadProgress.progress"
            :total="uploadProgress.total"
          />
        </v-card-text>

        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="dialog = false">{{ t('common.cancel') }}</v-btn>
          <v-btn color="primary" :disabled="!file" :loading="saving" @click="save">{{ t('common.save') }}</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <ConfirmDialog
      v-model="deleteDialog"
      icon="mdi-delete-alert-outline"
      :text="deleteTarget ? t('geoip.deleteText', { name: deleteTarget.name || deleteTarget.file_name }) : ''"
      :title="t('geoip.deleteTitle')"
      @confirm="doDelete"
    />
  </div>
</template>

<script lang="ts" setup>
  import type { GeoipDatabase } from '@/api/generated'
  import { computed, reactive, ref } from 'vue'
  import { useI18n } from 'vue-i18n'
  import { deleteGeoipDatabase, listGeoipDatabases, updateGeoipDatabase, uploadGeoipDatabase } from '@/api/generated'
  import ConfirmDialog from '@/components/ConfirmDialog.vue'
  import EmptyState from '@/components/EmptyState.vue'
  import ErrorAlert from '@/components/ErrorAlert.vue'
  import PageHeader from '@/components/PageHeader.vue'
  import UploadProgress from '@/components/UploadProgress.vue'
  import { useApiResource } from '@/composables/useApiResource'
  import { createByteProgressTracker } from '@/composables/useUpload'
  import { useSnackbarStore } from '@/stores/snackbar'
  import { formatBytes } from '@/utils/bytes'

  const { t } = useI18n()
  const snackbar = useSnackbarStore()

  const resource = useApiResource<GeoipDatabase>(async () => {
    const { data } = await listGeoipDatabases()
    return data?.databases ?? []
  })

  const headers = computed(() => [
    { title: t('geoip.name'), key: 'name' },
    { title: t('geoip.file'), key: 'file_name' },
    { title: t('geoip.rank'), key: 'rank', align: 'center' as const },
    { title: t('geoip.size'), key: 'size' },
    { title: t('common.enabled'), key: 'enabled', align: 'center' as const },
    { title: t('common.actions'), key: 'actions', sortable: false, align: 'end' as const },
  ])

  const ordered = computed(() => [...resource.items.value].toSorted((a, b) => (a.rank ?? 0) - (b.rank ?? 0)))

  const dialog = ref(false)
  const file = ref<File | File[] | null>(null)
  const form = reactive({ name: '', rank: 10 })
  const saving = ref(false)
  const formError = ref<unknown>(null)
  const deleteDialog = ref(false)
  const deleteTarget = ref<GeoipDatabase | null>(null)
  const uploadProgress = reactive({ progress: 0, loaded: 0, total: 0, bytesPerSec: 0 })
  const uploadTracker = createByteProgressTracker()

  function pickedFile (): File | null {
    const value = file.value
    if (Array.isArray(value)) return value[0] ?? null
    return value
  }

  function isFirst (item: GeoipDatabase): boolean {
    return ordered.value[0]?.id === item.id
  }

  function isLast (item: GeoipDatabase): boolean {
    return ordered.value.at(-1)?.id === item.id
  }

  function openUpload (): void {
    file.value = null
    form.name = ''
    form.rank = (ordered.value.at(-1)?.rank ?? 0) + 10
    formError.value = null
    uploadProgress.progress = 0
    uploadProgress.loaded = 0
    uploadProgress.total = 0
    uploadProgress.bytesPerSec = 0
    uploadTracker.resetSpeed()
    dialog.value = true
  }

  async function save (): Promise<void> {
    const picked = pickedFile()
    if (!picked) return
    saving.value = true
    formError.value = null
    uploadProgress.progress = 0
    uploadProgress.loaded = 0
    uploadProgress.total = picked.size
    uploadProgress.bytesPerSec = 0
    uploadTracker.resetSpeed()
    try {
      await uploadGeoipDatabase({
        body: {
          file: picked,
          name: form.name.trim() || undefined,
          rank: form.rank,
        },
        onUploadProgress: event => {
          const loaded = event.loaded ?? 0
          const total = event.total ?? picked.size
          uploadProgress.loaded = loaded
          uploadProgress.total = total
          uploadProgress.bytesPerSec = uploadTracker.sample(loaded)
          uploadProgress.progress = total ? Math.min(99, Math.round((loaded / total) * 100)) : 0
        },
      })
      uploadProgress.progress = 100
      uploadProgress.loaded = uploadProgress.total
      snackbar.show(t('geoip.upload') + ' ✓')
      dialog.value = false
      await resource.refresh()
    } catch (error) {
      formError.value = error
    } finally {
      saving.value = false
    }
  }

  async function toggleEnabled (item: GeoipDatabase, value: boolean): Promise<void> {
    if (!item.id) return
    await updateGeoipDatabase({ path: { id: item.id }, body: { enabled: value } })
    await resource.refresh()
  }

  async function move (item: GeoipDatabase, delta: number): Promise<void> {
    const rows = ordered.value
    const index = rows.findIndex(row => row.id === item.id)
    const swap = rows[index + delta]
    if (!item.id || !swap?.id) return
    await Promise.all([
      updateGeoipDatabase({ path: { id: item.id }, body: { rank: swap.rank ?? 0 } }),
      updateGeoipDatabase({ path: { id: swap.id }, body: { rank: item.rank ?? 0 } }),
    ])
    await resource.refresh()
  }

  function askDelete (item: GeoipDatabase): void {
    deleteTarget.value = item
    deleteDialog.value = true
  }

  async function doDelete (): Promise<void> {
    if (!deleteTarget.value?.id) return
    try {
      await deleteGeoipDatabase({ path: { id: deleteTarget.value.id } })
      snackbar.show(t('common.delete') + ' ✓')
      await resource.refresh()
    } catch (error) {
      snackbar.show(String(error), 'error', 5000)
    }
  }
</script>
