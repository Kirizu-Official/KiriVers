<!-- pages/projects/[projectRef]/languages.vue — 项目语言列表 -->
<template>
  <div>
    <PageHeader :description="t('languages.description')" :title="t('languages.title')">
      <template #actions>
        <v-btn color="primary" prepend-icon="mdi-plus" @click="openCreate">{{ t('languages.create') }}</v-btn>
      </template>
    </PageHeader>

    <ErrorAlert :error="resource.error.value" />
    <EmptyState v-if="!resource.loading.value && resource.items.value.length === 0" icon="mdi-translate" :text="t('common.empty')" />

    <v-card v-else>
      <v-data-table
        :headers="headers"
        hide-default-footer
        :items="resource.items.value"
        items-key="id"
        :loading="resource.loading.value"
      >
        <template #item.code="{ item }">
          <span class="font-weight-medium">{{ item.code }}</span>

          <v-chip
            v-if="item.is_default"
            class="ml-2"
            color="primary"
            size="x-small"
            variant="tonal"
          >
            {{ t('languages.default') }}
          </v-chip>
        </template>

        <template #item.display_name="{ item }">
          {{ item.display_name || item.code }}
        </template>

        <template #item.actions="{ item }">
          <v-btn prepend-icon="mdi-pencil-outline" size="small" variant="text" @click="openEdit(item)">
            {{ t('common.edit') }}
          </v-btn>

          <v-btn
            color="error"
            :disabled="Boolean(item.is_default) || resource.items.value.length <= 1"
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
      <v-card :title="editing ? t('common.edit') : t('languages.create')">
        <v-card-text>
          <ErrorAlert :error="formError" />

          <v-text-field
            v-model="form.code"
            :disabled="Boolean(editing)"
            :hint="t('languages.codeHint')"
            :label="t('languages.code')"
            persistent-hint
            :rules="editing ? [] : [required, codeRule]"
          />

          <v-text-field
            v-model="form.display_name"
            :hint="t('languages.displayNameHint')"
            :label="t('languages.displayName')"
            persistent-hint
          />

          <v-text-field
            v-model.number="form.sort_order"
            :hint="t('languages.sortHint')"
            :label="t('languages.sortOrder')"
            persistent-hint
            type="number"
          />

          <v-switch
            v-model="form.is_default"
            color="primary"
            density="compact"
            :disabled="Boolean(editing?.is_default)"
            :hint="t('languages.defaultHint')"
            :label="t('languages.default')"
            persistent-hint
          />
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
      :text="deleteTarget ? t('languages.deleteText', { name: deleteTarget.display_name || deleteTarget.code }) : ''"
      :title="t('languages.deleteTitle')"
      @confirm="doDelete"
    />
  </div>
</template>

<script lang="ts" setup>
  import type { ProjectLanguage } from '@/api/generated'
  import { computed, reactive, ref } from 'vue'
  import { useI18n } from 'vue-i18n'
  import { useRoute } from 'vue-router'
  import { createLanguage, deleteLanguage, updateLanguage } from '@/api/generated'
  import ConfirmDialog from '@/components/ConfirmDialog.vue'
  import EmptyState from '@/components/EmptyState.vue'
  import ErrorAlert from '@/components/ErrorAlert.vue'
  import PageHeader from '@/components/PageHeader.vue'
  import { useProjectLanguages } from '@/composables/useProjectLanguages'
  import { useSnackbarStore } from '@/stores/snackbar'

  const { t } = useI18n()
  const route = useRoute('/projects/[projectRef]/languages')
  const snackbar = useSnackbarStore()

  const projectRef = computed(() => route.params.projectRef)
  const resource = useProjectLanguages(projectRef)

  const headers = computed(() => [
    { title: t('languages.code'), key: 'code' },
    { title: t('languages.displayName'), key: 'display_name' },
    { title: t('languages.sortOrder'), key: 'sort_order', align: 'center' as const },
    { title: t('common.actions'), key: 'actions', sortable: false, align: 'end' as const },
  ])

  const dialog = ref(false)
  const editing = ref<ProjectLanguage | null>(null)
  const form = reactive({ code: '', display_name: '', sort_order: 10, is_default: false })
  const saving = ref(false)
  const formError = ref<unknown>(null)
  const deleteDialog = ref(false)
  const deleteTarget = ref<ProjectLanguage | null>(null)

  function required (value: string): boolean | string {
    return Boolean(value) || t('common.required')
  }
  function codeRule (value: string): boolean | string {
    return /^[A-Z0-9][\w-]{1,31}$/i.test(value) || t('languages.codeRule')
  }

  function openCreate (): void {
    editing.value = null
    form.code = ''
    form.display_name = ''
    form.sort_order = 10
    form.is_default = false
    formError.value = null
    dialog.value = true
  }

  function openEdit (item: ProjectLanguage): void {
    editing.value = item
    form.code = item.code ?? ''
    form.display_name = item.display_name ?? ''
    form.sort_order = item.sort_order ?? 0
    form.is_default = Boolean(item.is_default)
    formError.value = null
    dialog.value = true
  }

  async function save (): Promise<void> {
    saving.value = true
    formError.value = null
    try {
      await (editing.value
        ? updateLanguage({
          path: { project_ref: projectRef.value, code: editing.value.code ?? '' },
          body: {
            display_name: form.display_name,
            sort_order: form.sort_order,
            is_default: form.is_default,
          },
        })
        : createLanguage({
          path: { project_ref: projectRef.value },
          body: {
            code: form.code,
            display_name: form.display_name,
            sort_order: form.sort_order,
            is_default: form.is_default,
          },
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

  function askDelete (item: ProjectLanguage): void {
    deleteTarget.value = item
    deleteDialog.value = true
  }

  async function doDelete (): Promise<void> {
    if (!deleteTarget.value) return
    try {
      await deleteLanguage({ path: { project_ref: projectRef.value, code: deleteTarget.value.code ?? '' } })
      snackbar.show(t('common.delete') + ' ✓')
      await resource.refresh()
    } catch (error) {
      snackbar.show(String(error), 'error', 5000)
    }
  }
</script>
