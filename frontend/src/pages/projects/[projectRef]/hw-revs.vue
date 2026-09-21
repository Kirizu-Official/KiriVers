<!-- pages/projects/[projectRef]/hw-revs.vue — 硬件代号管理 -->
<template>
  <div>
    <PageHeader :description="t('hwRevs.description')" :title="t('hwRevs.title')">
      <template #actions>
        <v-btn color="primary" prepend-icon="mdi-plus" @click="openCreate">{{ t('hwRevs.create') }}</v-btn>
      </template>
    </PageHeader>

    <ErrorAlert :error="resource.error.value" />
    <EmptyState v-if="!resource.loading.value && resource.items.value.length === 0" icon="mdi-chip" :text="t('common.empty')" />

    <v-card v-else>
      <v-data-table
        :headers="headers"
        hide-default-footer
        :items="resource.items.value"
        items-key="id"
        :loading="resource.loading.value"
      >
        <template #item.slug="{ item }">
          <code>{{ item.slug }}</code>
        </template>

        <template #item.actions="{ item }">
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

    <v-dialog v-model="dialog" eager max-width="480">
      <v-card :key="editing?.id ?? 'hw-create'" :title="editing ? t('common.edit') : t('hwRevs.create')">
        <v-card-text>
          <ErrorAlert :error="formError" />

          <v-text-field
            v-model="form.slug"
            class="mt-2"
            density="comfortable"
            :disabled="Boolean(editing)"
            :hint="t('hwRevs.slugHint')"
            :label="t('hwRevs.slug')"
            persistent-hint
            :rules="editing ? [] : [required, slugRule]"
          />

          <v-text-field
            v-model.number="form.rank"
            class="mt-2"
            density="comfortable"
            :hint="t('hwRevs.rankHint')"
            :label="t('hwRevs.rank')"
            persistent-hint
            type="number"
          />

          <v-text-field v-model="form.notes" class="mt-2" density="comfortable" :label="t('hwRevs.notes')" />
        </v-card-text>

        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="dialog = false">{{ t('common.cancel') }}</v-btn>
          <v-btn color="primary" :loading="saving" @click="save">{{ t('common.save') }}</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <ConfirmDialog
      v-model="deleteDialog"
      icon="mdi-delete-alert-outline"
      :text="deleteTarget ? `${t('hwRevs.slug')}: ${deleteTarget.slug}` : ''"
      :title="t('common.delete')"
      @confirm="doDelete"
    />
  </div>
</template>

<script lang="ts" setup>
  import type { HwRev } from '@/api/generated'
  import { computed, reactive, ref } from 'vue'
  import { useI18n } from 'vue-i18n'
  import { useRoute } from 'vue-router'
  import { createHwRev, deleteHwRev, listHwRevs, updateHwRev } from '@/api/generated'
  import ConfirmDialog from '@/components/ConfirmDialog.vue'
  import EmptyState from '@/components/EmptyState.vue'
  import ErrorAlert from '@/components/ErrorAlert.vue'
  import PageHeader from '@/components/PageHeader.vue'
  import { useApiResource } from '@/composables/useApiResource'
  import { useSnackbarStore } from '@/stores/snackbar'

  const { t } = useI18n()
  const route = useRoute('/projects/[projectRef]/hw-revs')
  const snackbar = useSnackbarStore()

  const projectRef = computed(() => route.params.projectRef)
  const resource = useApiResource<HwRev>(async () => {
    const { data } = await listHwRevs({ path: { project_ref: projectRef.value } })
    return data?.hw_revs ?? []
  })

  const headers = computed(() => [
    { title: t('hwRevs.slug'), key: 'slug' },
    { title: t('hwRevs.rank'), key: 'rank', align: 'center' as const },
    { title: t('hwRevs.notes'), key: 'notes' },
    { title: t('common.actions'), key: 'actions', sortable: false, align: 'end' as const },
  ])

  const dialog = ref(false)
  const editing = ref<HwRev | null>(null)
  const form = reactive({ slug: '', rank: 1, notes: '' })
  const saving = ref(false)
  const formError = ref<unknown>(null)
  const deleteDialog = ref(false)
  const deleteTarget = ref<HwRev | null>(null)

  function required (value: string): boolean | string {
    return Boolean(value) || t('common.required')
  }
  function slugRule (value: string): boolean | string {
    return /^[\w-]{1,64}$/.test(value) || t('hwRevs.slugHint')
  }

  function openCreate (): void {
    editing.value = null
    Object.assign(form, { slug: '', rank: 1, notes: '' })
    formError.value = null
    dialog.value = true
  }

  function openEdit (item: HwRev): void {
    editing.value = item
    Object.assign(form, { slug: item.slug, rank: item.rank, notes: item.notes ?? '' })
    formError.value = null
    dialog.value = true
  }

  async function save (): Promise<void> {
    saving.value = true
    formError.value = null
    try {
      await (editing.value
        ? updateHwRev({
          path: { project_ref: projectRef.value, slug: editing.value.slug ?? '' },
          body: { rank: form.rank, notes: form.notes },
        })
        : createHwRev({
          path: { project_ref: projectRef.value },
          body: { slug: form.slug, rank: form.rank, notes: form.notes },
        }))
      snackbar.show(t('common.save') + ' ✓')
      dialog.value = false
      await resource.refresh()
    } catch (error) {
      formError.value = error
    } finally {
      saving.value = false
    }
  }

  function askDelete (item: HwRev): void {
    deleteTarget.value = item
    deleteDialog.value = true
  }

  async function doDelete (): Promise<void> {
    if (!deleteTarget.value) return
    try {
      await deleteHwRev({ path: { project_ref: projectRef.value, slug: deleteTarget.value.slug ?? '' } })
      snackbar.show(t('common.delete') + ' ✓')
      await resource.refresh()
    } catch (error) {
      snackbar.show(String(error), 'error', 5000)
    }
  }
</script>
