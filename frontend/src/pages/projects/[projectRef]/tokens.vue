<!-- pages/projects/[projectRef]/tokens.vue — 项目 Token / CI Token 单表 -->
<template>
  <div>
    <PageHeader :title="t('tokens.title')">
      <template #actions>
        <v-btn color="primary" prepend-icon="mdi-key-plus" @click="openCreate">
          {{ t('tokens.create') }}
        </v-btn>
      </template>
    </PageHeader>

    <v-alert class="mb-4" density="compact" type="info">
      {{ t('tokens.ciDescription') }}
    </v-alert>

    <EmptyState v-if="!loading && rows.length === 0" icon="mdi-key-variant" :text="t('common.empty')" />

    <v-card v-else>
      <v-data-table
        :headers="headers"
        hide-default-footer
        :items="rows"
        items-key="id"
        :loading="loading"
      >
        <template #item.kind="{ item }">
          <v-chip size="x-small" variant="tonal">
            {{ item.kind === 'ci' ? t('tokens.typeCi') : t('tokens.typeProject') }}
          </v-chip>
        </template>

        <template #item.name="{ item }">
          <span class="font-weight-medium">{{ item.name }}</span>
        </template>

        <template #item.scopes="{ item }">
          <v-chip
            v-for="scope in item.scopes"
            :key="scope"
            class="mr-1"
            size="x-small"
            variant="tonal"
          >
            {{ scope }}
          </v-chip>
        </template>

        <template #item.fingerprint="{ item }">
          <code class="text-caption">{{ item.fingerprint }}</code>
        </template>

        <template #item.expires_at="{ item }">
          <span class="text-body-2">{{ item.expires_at ?? '∞' }}</span>
        </template>

        <template #item.actions="{ item }">
          <v-btn
            color="error"
            prepend-icon="mdi-key-remove"
            size="small"
            variant="text"
            @click="askDelete(item)"
          >
            {{ t('common.delete') }}
          </v-btn>
        </template>
      </v-data-table>
    </v-card>

    <v-dialog v-model="dialog" max-width="560">
      <v-card :title="t('tokens.create')">
        <v-card-text>
          <ErrorAlert :error="formError" />

          <v-select
            v-model="form.kind"
            :hint="t('tokens.typeHint')"
            :items="kindItems"
            :label="t('tokens.type')"
            persistent-hint
          />

          <v-text-field
            v-model="form.name"
            :hint="t('tokens.nameHint')"
            :label="t('tokens.name')"
            persistent-hint
            :rules="[required]"
          />

          <v-select
            v-model="form.scopes"
            chips
            :hint="t('tokens.scopesHint')"
            :items="scopeOptions"
            :label="t('tokens.scopes')"
            multiple
            persistent-hint
          />

          <DateTimeField v-model="form.expires_at" :hint="t('tokens.expiresHint')" :include-time="false" :label="t('tokens.expiresAt')" />
        </v-card-text>

        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="dialog = false">{{ t('common.cancel') }}</v-btn>
          <v-btn color="primary" :loading="saving" @click="save">{{ t('common.create') }}</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <v-dialog v-model="revealOpen" max-width="560" persistent>
      <v-card prepend-icon="mdi-key-alert" :title="t('tokens.revealTitle')">
        <v-card-text>
          <v-alert class="mb-4" density="compact" type="warning">
            {{ t('tokens.revealText') }}
          </v-alert>

          <CopyField :value="revealedToken" />
        </v-card-text>

        <v-card-actions>
          <v-spacer />
          <v-btn color="primary" @click="revealOpen = false">{{ t('common.close') }}</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <ConfirmDialog
      v-model="deleteDialog"
      icon="mdi-key-remove"
      :text="deleteTarget ? t('tokens.deleteText', { name: deleteTarget.name }) : ''"
      :title="t('tokens.deleteTitle')"
      @confirm="doDelete"
    />
  </div>
</template>

<script lang="ts" setup>
  import type { CiToken, ProjectToken } from '@/api/generated'
  import { computed, onMounted, reactive, ref } from 'vue'
  import { useI18n } from 'vue-i18n'
  import { useRoute } from 'vue-router'
  import {
    createCiToken,
    createProjectToken,
    deleteCiToken,
    deleteProjectToken,
    listCiTokens,
    listProjectTokens,
  } from '@/api/generated'
  import ConfirmDialog from '@/components/ConfirmDialog.vue'
  import CopyField from '@/components/CopyField.vue'
  import DateTimeField from '@/components/DateTimeField.vue'
  import EmptyState from '@/components/EmptyState.vue'
  import ErrorAlert from '@/components/ErrorAlert.vue'
  import PageHeader from '@/components/PageHeader.vue'
  import { useSnackbarStore } from '@/stores/snackbar'

  type TokenKind = 'project' | 'ci'
  type TokenRow = (ProjectToken | CiToken) & { kind: TokenKind }

  const { t } = useI18n()
  const route = useRoute('/projects/[projectRef]/tokens')
  const snackbar = useSnackbarStore()

  const projectRef = computed(() => route.params.projectRef)

  const loading = ref(true)
  const projectTokens = ref<ProjectToken[]>([])
  const ciTokens = ref<CiToken[]>([])

  const rows = computed<TokenRow[]>(() => [
    ...projectTokens.value.map(item => ({ ...item, kind: 'project' as const })),
    ...ciTokens.value.map(item => ({ ...item, kind: 'ci' as const })),
  ])

  const headers = computed(() => [
    { title: t('tokens.type'), key: 'kind' },
    { title: t('tokens.name'), key: 'name' },
    { title: t('tokens.scopes'), key: 'scopes' },
    { title: t('tokens.fingerprint'), key: 'fingerprint' },
    { title: t('tokens.expiresAt'), key: 'expires_at' },
    { title: t('common.actions'), key: 'actions', sortable: false, align: 'end' as const },
  ])

  const kindItems = computed(() => [
    { title: t('tokens.typeProject'), value: 'project' },
    { title: t('tokens.typeCi'), value: 'ci' },
  ])

  const scopeOptions = ['project:read', 'artifact:write', 'release:publish', 'project:admin']

  async function load (): Promise<void> {
    loading.value = true
    try {
      const [projects, cis] = await Promise.all([
        listProjectTokens({ path: { project_ref: projectRef.value } }).then(({ data }) => data?.tokens ?? []),
        listCiTokens({ path: { project_ref: projectRef.value } }).then(({ data }) => data?.tokens ?? []),
      ])
      projectTokens.value = projects
      ciTokens.value = cis
    } finally {
      loading.value = false
    }
  }

  const dialog = ref(false)
  const form = reactive({ kind: 'project' as TokenKind, name: '', scopes: ['artifact:write'] as string[], expires_at: '' })
  const saving = ref(false)
  const formError = ref<unknown>(null)
  const revealOpen = ref(false)
  const revealedToken = ref('')
  const deleteDialog = ref(false)
  const deleteTarget = ref<{ id: string, name: string, kind: TokenKind } | null>(null)

  function required (value: string): boolean | string {
    return Boolean(value) || t('common.required')
  }

  function openCreate (): void {
    form.kind = 'project'
    form.name = ''
    form.scopes = ['artifact:write', 'release:publish']
    form.expires_at = ''
    formError.value = null
    dialog.value = true
  }

  async function save (): Promise<void> {
    saving.value = true
    formError.value = null
    try {
      const payload = {
        name: form.name,
        scopes: form.scopes as Array<'project:read' | 'artifact:write' | 'release:publish' | 'project:admin'>,
        expires_at: form.expires_at || undefined,
      }
      const created = form.kind === 'ci'
        ? (await createCiToken({ path: { project_ref: projectRef.value }, body: payload })).data
        : (await createProjectToken({ path: { project_ref: projectRef.value }, body: payload })).data
      dialog.value = false
      revealedToken.value = created?.token ?? ''
      revealOpen.value = true
      await load()
    } catch (error) {
      formError.value = error
    } finally {
      saving.value = false
    }
  }

  function askDelete (item: TokenRow): void {
    deleteTarget.value = {
      id: item.id ?? '',
      name: item.name ?? '',
      kind: item.kind,
    }
    deleteDialog.value = true
  }

  async function doDelete (): Promise<void> {
    if (!deleteTarget.value) return
    try {
      await (deleteTarget.value.kind === 'ci'
        ? deleteCiToken({ path: { project_ref: projectRef.value, token_id: deleteTarget.value.id } })
        : deleteProjectToken({ path: { project_ref: projectRef.value, token_id: deleteTarget.value.id } }))
      snackbar.show(t('common.delete') + ' ✓')
      await load()
    } catch (error) {
      snackbar.show(String(error), 'error', 5000)
    }
  }

  onMounted(load)
</script>
