<!-- pages/projects/[projectRef]/channels.vue — 渠道管理 -->
<template>
  <div>
    <PageHeader :description="t('channels.description')" :title="t('channels.title')">
      <template #actions>
        <v-btn color="primary" prepend-icon="mdi-plus" @click="openCreate">{{ t('channels.create') }}</v-btn>
      </template>
    </PageHeader>

    <ErrorAlert :error="resource.error.value" />

    <EmptyState v-if="!resource.loading.value && resource.items.value.length === 0" icon="mdi-source-branch" :text="t('common.empty')" />

    <v-card v-else>
      <v-data-table
        :headers="headers"
        hide-default-footer
        :items="resource.items.value"
        items-key="id"
        :loading="resource.loading.value"
      >
        <template #item.slug="{ item }">
          <span class="font-weight-medium">{{ item.slug }}</span>

          <v-chip
            v-if="item.system"
            class="ml-2"
            color="primary"
            size="x-small"
            variant="tonal"
          >
            {{ t('channels.system') }}
          </v-chip>
        </template>

        <template #item.unlisted="{ item }">
          <v-chip v-if="item.unlisted" size="x-small" variant="tonal">{{ t('channels.unlisted') }}</v-chip>
          <span v-else class="text-medium-emphasis">—</span>
        </template>

        <template #item.token="{ item }">
          <CopyField v-if="item.token" :value="item.token" />
          <span v-else class="text-medium-emphasis">—</span>
        </template>

        <template #item.token_required="{ item }">
          <v-chip v-if="item.token_required" color="warning" size="x-small" variant="tonal">
            {{ t('channels.tokenRequired') }}
          </v-chip>

          <span v-else class="text-medium-emphasis">—</span>
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
          <v-btn prepend-icon="mdi-pencil-outline" size="small" variant="text" @click="openEdit(item)">
            {{ t('common.edit') }}
          </v-btn>

          <v-btn prepend-icon="mdi-file-cog-outline" size="small" variant="text" @click="openPolicy(item)">
            {{ t('installPolicy.edit') }}
          </v-btn>

          <v-btn
            v-if="!item.system"
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

    <v-dialog v-model="policyDialog" max-width="960">
      <v-card :title="t('installPolicy.channelTitle', { slug: policySlug })">
        <v-card-text>
          <InstallPolicyEditor
            v-if="policyDialog && policySlug"
            :channel-slug="policySlug"
            :project-ref="projectRef"
            scope="channel"
          />
        </v-card-text>

        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="policyDialog = false">{{ t('common.close') }}</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <v-dialog v-model="dialog" max-width="480">
      <v-card :title="editing ? t('common.edit') : t('channels.create')">
        <v-card-text>
          <ErrorAlert :error="formError" />

          <v-text-field
            v-model="form.name"
            :hint="t('channels.nameHint')"
            :label="t('channels.name')"
            persistent-hint
            :rules="[required]"
          />

          <v-text-field
            v-model="form.slug"
            :disabled="Boolean(editing)"
            :hint="t('channels.slugHint')"
            :label="t('channels.slug')"
            persistent-hint
            :rules="editing ? [] : [required, channelSlugRule]"
          />

          <v-text-field
            v-model.number="form.stability_rank"
            :hint="t('channels.rankHint')"
            :label="t('channels.rank')"
            persistent-hint
            type="number"
          />

          <v-switch
            v-model="form.unlisted"
            color="primary"
            density="compact"
            :hint="t('channels.unlistedHint')"
            :label="t('channels.unlisted')"
            persistent-hint
          />

          <v-text-field
            v-model="form.token"
            :disabled="form.clearToken"
            :hint="t('channels.tokenHint')"
            :label="t('channels.token')"
            persistent-hint
          />

          <v-checkbox
            v-if="editing"
            v-model="form.clearToken"
            :label="t('channels.tokenClear')"
          />

          <div class="text-caption text-medium-emphasis">{{ t('channels.systemHint') }}</div>
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
      :text="deleteTarget ? t('channels.deleteText', { name: deleteTarget.slug }) : ''"
      :title="t('channels.deleteTitle')"
      @confirm="doDelete"
    />
  </div>
</template>

<script lang="ts" setup>
  import type { Channel } from '@/api/generated'
  import { computed, reactive, ref } from 'vue'
  import { useI18n } from 'vue-i18n'
  import { useRoute } from 'vue-router'
  import { createChannel, deleteChannel, listChannels, updateChannel } from '@/api/generated'
  import ConfirmDialog from '@/components/ConfirmDialog.vue'
  import CopyField from '@/components/CopyField.vue'
  import EmptyState from '@/components/EmptyState.vue'
  import ErrorAlert from '@/components/ErrorAlert.vue'
  import InstallPolicyEditor from '@/components/InstallPolicyEditor.vue'
  import PageHeader from '@/components/PageHeader.vue'
  import { useApiResource } from '@/composables/useApiResource'
  import { useSnackbarStore } from '@/stores/snackbar'

  const { t } = useI18n()
  const route = useRoute('/projects/[projectRef]/channels')
  const snackbar = useSnackbarStore()

  const projectRef = computed(() => route.params.projectRef)
  const resource = useApiResource<Channel>(async () => {
    const { data } = await listChannels({ path: { project_ref: projectRef.value } })
    return data?.channels ?? []
  })

  const headers = computed(() => [
    { title: t('channels.name'), key: 'name' },
    { title: t('channels.slug'), key: 'slug' },
    { title: t('channels.rank'), key: 'stability_rank', align: 'center' as const },
    { title: t('channels.unlisted'), key: 'unlisted', align: 'center' as const },
    { title: t('channels.token'), key: 'token' },
    { title: t('channels.tokenRequired'), key: 'token_required', align: 'center' as const },
    { title: t('common.enabled'), key: 'enabled', align: 'center' as const },
    { title: t('common.actions'), key: 'actions', sortable: false, align: 'end' as const },
  ])

  const dialog = ref(false)
  const policyDialog = ref(false)
  const policySlug = ref('')
  const editing = ref<Channel | null>(null)
  const form = reactive({
    name: '',
    slug: '',
    stability_rank: 20,
    unlisted: false,
    token: '',
    clearToken: false,
  })
  const saving = ref(false)
  const formError = ref<unknown>(null)
  const deleteDialog = ref(false)
  const deleteTarget = ref<Channel | null>(null)

  function required (value: string): boolean | string {
    return Boolean(value) || t('common.required')
  }
  function channelSlugRule (value: string): boolean | string {
    return /^[a-z0-9-]{3,64}$/.test(value) || t('channels.slugRule')
  }

  function tokenBody (): string | undefined {
    if (form.clearToken) return ''
    const next = form.token.trim()
    return next || undefined
  }

  function openPolicy (item: Channel): void {
    policySlug.value = item.slug ?? ''
    policyDialog.value = true
  }

  function openCreate (): void {
    editing.value = null
    form.name = ''
    form.slug = ''
    form.stability_rank = 20
    form.unlisted = false
    form.token = ''
    form.clearToken = false
    formError.value = null
    dialog.value = true
  }

  function openEdit (item: Channel): void {
    editing.value = item
    form.name = item.name ?? ''
    form.slug = item.slug ?? ''
    form.stability_rank = item.stability_rank ?? 20
    form.unlisted = Boolean(item.unlisted)
    form.token = item.token ?? ''
    form.clearToken = false
    formError.value = null
    dialog.value = true
  }

  async function save (): Promise<void> {
    saving.value = true
    formError.value = null
    try {
      const token = tokenBody()
      await (editing.value
        ? updateChannel({
          path: { project_ref: projectRef.value, slug: editing.value.slug ?? '' },
          body: {
            name: form.name,
            stability_rank: form.stability_rank,
            unlisted: form.unlisted,
            ...(token === undefined ? {} : { token }),
          },
        })
        : createChannel({
          path: { project_ref: projectRef.value },
          body: {
            name: form.name,
            slug: form.slug,
            stability_rank: form.stability_rank,
            unlisted: form.unlisted,
            ...(token === undefined ? {} : { token }),
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

  async function toggleEnabled (item: Channel, value: boolean): Promise<void> {
    await updateChannel({
      path: { project_ref: projectRef.value, slug: item.slug ?? '' },
      body: { enabled: value },
    })
    await resource.refresh()
  }

  function askDelete (item: Channel): void {
    deleteTarget.value = item
    deleteDialog.value = true
  }

  async function doDelete (): Promise<void> {
    if (!deleteTarget.value) return
    try {
      await deleteChannel({ path: { project_ref: projectRef.value, slug: deleteTarget.value.slug ?? '' } })
      snackbar.show(t('common.delete') + ' ✓')
      await resource.refresh()
    } catch (error) {
      snackbar.show(String(error), 'error', 5000)
    }
  }
</script>
