<!-- pages/admins.vue — 管理员用户管理（会话 Token） -->
<template>
  <div>
    <PageHeader :description="t('admins.description')" :title="t('admins.title')">
      <template #actions>
        <v-btn color="primary" prepend-icon="mdi-plus" @click="openCreate">
          {{ t('admins.create') }}
        </v-btn>
      </template>
    </PageHeader>

    <ErrorAlert :error="resource.error.value" />

    <EmptyState v-if="!resource.loading.value && resource.items.value.length === 0" icon="mdi-shield-account-outline" :text="t('common.empty')" />

    <v-card v-else>
      <v-data-table
        :headers="headers"
        hide-default-footer
        :items="resource.items.value"
        items-key="id"
        :loading="resource.loading.value"
      >
        <template #item.username="{ item }">
          <span class="font-weight-medium">{{ item.username }}</span>
          <v-icon v-if="item.username === auth.username" class="ml-2" color="primary" size="14">mdi-account-check</v-icon>
        </template>

        <template #item.is_platform_admin="{ item }">
          <v-chip :color="item.is_platform_admin !== false ? 'primary' : undefined" size="x-small" variant="tonal">
            {{ item.is_platform_admin !== false ? t('admins.platformAdmin') : t('admins.projectOnly') }}
          </v-chip>
        </template>

        <template #item.last_login_at="{ item }">
          {{ item.last_login_at ?? t('admins.neverLoggedIn') }}
        </template>

        <template #item.last_login_ip="{ item }">
          {{ item.last_login_ip ?? t('admins.neverLoggedIn') }}
        </template>

        <template #item.actions="{ item }">
          <v-btn
            prepend-icon="mdi-pencil-outline"
            size="small"
            variant="text"
            @click="openEdit(item)"
          >
            {{ t('admins.rename') }}
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

    <!-- 创建 / 编辑对话框 -->
    <v-dialog v-model="dialog" max-width="480">
      <v-card :title="editingId ? t('admins.rename') : t('admins.create')">
        <v-card-text>
          <ErrorAlert :error="formError" />
          <v-text-field v-model="username" :label="t('admins.username')" :rules="[required]" />

          <v-text-field
            v-model="password"
            :label="editingId ? t('admins.newPassword') : t('admins.password')"
            :placeholder="editingId ? t('admins.passwordHint') : ''"
            type="password"
          />

          <v-alert v-if="editingId" density="compact" type="info">
            {{ t('admins.lastAdminWarn') }}
          </v-alert>
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
      :text="deleteTarget ? t('admins.deleteText', { name: deleteTarget.username }) : ''"
      :title="t('admins.deleteTitle')"
      @confirm="doDelete"
    />
  </div>
</template>

<script lang="ts" setup>
  import type { Admin } from '@/api/generated'
  import { computed, ref } from 'vue'
  import { useI18n } from 'vue-i18n'
  import { isApiError } from '@/api/client'
  import { createAdmin, deleteAdmin, listAdmins, updateAdmin } from '@/api/generated'
  import ConfirmDialog from '@/components/ConfirmDialog.vue'
  import EmptyState from '@/components/EmptyState.vue'
  import ErrorAlert from '@/components/ErrorAlert.vue'
  import PageHeader from '@/components/PageHeader.vue'
  import { useApiResource } from '@/composables/useApiResource'
  import { useAuthStore } from '@/stores/auth'
  import { useSnackbarStore } from '@/stores/snackbar'

  const { t, te } = useI18n()
  const snackbar = useSnackbarStore()
  const auth = useAuthStore()

  const resource = useApiResource<Admin>(async () => {
    const { data } = await listAdmins()
    return data?.admins ?? []
  })

  const headers = computed(() => [
    { title: t('admins.username'), key: 'username' },
    { title: t('admins.kind'), key: 'is_platform_admin' },
    { title: t('projects.createdAt'), key: 'created_at' },
    { title: t('admins.lastLoginAt'), key: 'last_login_at' },
    { title: t('admins.lastLoginIp'), key: 'last_login_ip' },
    { title: t('common.actions'), key: 'actions', sortable: false, align: 'end' as const },
  ])

  const dialog = ref(false)
  const editingId = ref<string | null>(null)
  const username = ref('')
  const password = ref('')
  const saving = ref(false)
  const formError = ref<unknown>(null)

  const deleteDialog = ref(false)
  const deleteTarget = ref<Admin | null>(null)

  function required (value: string): boolean | string {
    return Boolean(value) || t('common.required')
  }

  function openCreate (): void {
    editingId.value = null
    username.value = ''
    password.value = ''
    formError.value = null
    dialog.value = true
  }

  function openEdit (item: Admin): void {
    editingId.value = item.id ?? null
    username.value = item.username ?? ''
    password.value = ''
    formError.value = null
    dialog.value = true
  }

  async function save (): Promise<void> {
    saving.value = true
    formError.value = null
    try {
      if (editingId.value) {
        await updateAdmin({
          path: { id: editingId.value },
          body: { username: username.value, password: password.value || undefined },
        })
        snackbar.show(t('common.save') + ' ✓')
      } else {
        await createAdmin({ body: { username: username.value, password: password.value } })
        snackbar.show(t('admins.create') + ' ✓')
      }
      dialog.value = false
      await resource.refresh()
    } catch (error) {
      formError.value = error
    } finally {
      saving.value = false
    }
  }

  function askDelete (item: Admin): void {
    deleteTarget.value = item
    deleteDialog.value = true
  }

  async function doDelete (): Promise<void> {
    if (!deleteTarget.value) return
    try {
      await deleteAdmin({ path: { id: deleteTarget.value.id ?? '' } })
      snackbar.show(t('common.delete') + ' ✓')
      await resource.refresh()
    } catch (error) {
      // LAST_ADMIN 等错误码在此呈现
      const code = codeOf(error)
      snackbar.show(te(`errors.${code}`) ? t(`errors.${code}`) : String(error), 'error', 5000)
    }
  }

  function codeOf (err: unknown): string {
    return isApiError(err) ? err.code : 'UNKNOWN_ERROR'
  }
</script>
