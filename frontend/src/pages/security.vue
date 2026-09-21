<!-- pages/security.vue — 当前管理员的 2FA：TOTP 轮换、恢复码、Passkey -->
<template>
  <div>
    <PageHeader :description="t('accountSecurity.description')" :title="t('accountSecurity.title')" />

    <ErrorAlert :error="loadError" />

    <div v-if="loading && !status" class="d-flex justify-center py-12">
      <v-progress-circular color="primary" indeterminate />
    </div>

    <template v-else-if="status">
      <v-card class="mb-6">
        <v-card-title>{{ t('accountSecurity.totpTitle') }}</v-card-title>

        <v-card-text>
          <v-chip class="mb-4" :color="status.totp_enabled ? 'success' : 'warning'" variant="tonal">
            {{ status.totp_enabled ? t('accountSecurity.totpOn') : t('accountSecurity.totpOff') }}
          </v-chip>

          <div class="text-body-2 text-medium-emphasis">
            {{ t('accountSecurity.totpHint') }}
          </div>
        </v-card-text>

        <v-card-actions>
          <v-btn
            color="primary"
            :disabled="!status.totp_enabled"
            prepend-icon="mdi-rotate-right"
            variant="tonal"
            @click="startRotate"
          >
            {{ t('accountSecurity.rotateTotp') }}
          </v-btn>
        </v-card-actions>
      </v-card>

      <v-card class="mb-6">
        <v-card-title>{{ t('accountSecurity.recoveryTitle') }}</v-card-title>

        <v-card-text>
          <div class="text-body-1">
            {{ t('accountSecurity.recoveryRemaining', { count: status.recovery_remaining ?? 0 }) }}
          </div>
        </v-card-text>

        <v-card-actions>
          <v-btn color="primary" prepend-icon="mdi-refresh" variant="tonal" @click="regenDialog = true">
            {{ t('accountSecurity.regenerate') }}
          </v-btn>
        </v-card-actions>
      </v-card>

      <v-card>
        <v-card-title>{{ t('accountSecurity.passkeyTitle') }}</v-card-title>

        <v-card-text>
          <v-alert v-if="passkeyNotReady" class="mb-4" density="compact" type="info">
            {{ t('accountSecurity.passkeyNotReady') }}
          </v-alert>

          <EmptyState
            v-if="(status.passkeys ?? []).length === 0"
            icon="mdi-key-outline"
            :text="t('accountSecurity.passkeyEmpty')"
          />

          <v-list v-else>
            <v-list-item v-for="item in status.passkeys" :key="item.id">
              <v-list-item-title>{{ item.name || t('auth.passkeyDefaultName') }}</v-list-item-title>
              <v-list-item-subtitle>{{ item.created_at }}</v-list-item-subtitle>

              <template #append>
                <v-btn
                  color="error"
                  icon="mdi-delete-outline"
                  size="small"
                  variant="text"
                  @click="askDeletePasskey(item)"
                />
              </template>
            </v-list-item>
          </v-list>
        </v-card-text>

        <v-card-actions>
          <v-btn color="primary" prepend-icon="mdi-plus" variant="tonal" @click="openAddPasskey">
            {{ t('accountSecurity.addPasskey') }}
          </v-btn>
        </v-card-actions>
      </v-card>
    </template>

    <v-dialog v-model="rotateOpen" max-width="520">
      <v-card :title="t('accountSecurity.rotateTotp')">
        <v-card-text>
          <ErrorAlert :error="formError" />
          <div class="text-caption mb-2">{{ t('auth.totpQrHint') }}</div>
          <OtpQr :value="rotateOtpauth" />
          <div class="text-caption mt-4 mb-2">{{ t('auth.totpSecretHint') }}</div>
          <CopyField :value="rotateSecret" />
          <div class="text-caption mt-4 mb-2">{{ t('auth.otpauthHint') }}</div>
          <CopyField :value="rotateOtpauth" />

          <v-text-field
            v-model="rotateCode"
            class="mt-4"
            inputmode="numeric"
            :label="t('auth.totpCode')"
            maxlength="8"
            variant="outlined"
          />
        </v-card-text>

        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="rotateOpen = false">{{ t('common.cancel') }}</v-btn>
          <v-btn color="primary" :loading="saving" @click="confirmRotate">{{ t('common.confirm') }}</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <v-dialog v-model="codesOpen" max-width="520">
      <v-card :title="t('accountSecurity.newCodesTitle')">
        <v-card-text>
          <v-alert class="mb-4" density="compact" type="warning">
            {{ t('auth.recoveryOnceHint') }}
          </v-alert>

          <CopyField v-for="code in newCodes" :key="code" class="mb-2" :value="code" />
        </v-card-text>

        <v-card-actions>
          <v-btn prepend-icon="mdi-content-copy" variant="tonal" @click="copyNewCodes">
            {{ t('auth.copyAllCodes') }}
          </v-btn>

          <v-spacer />
          <v-btn color="primary" @click="codesOpen = false">{{ t('common.close') }}</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <v-dialog v-model="passkeyOpen" max-width="480">
      <v-card :title="t('accountSecurity.addPasskey')">
        <v-card-text>
          <ErrorAlert :error="formError" />
          <v-text-field v-model="passkeyName" :label="t('auth.passkeyName')" variant="outlined" />
        </v-card-text>

        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="passkeyOpen = false">{{ t('common.cancel') }}</v-btn>
          <v-btn color="primary" :loading="saving" @click="finishAddPasskey">{{ t('common.confirm') }}</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <ConfirmDialog
      v-model="regenDialog"
      :text="t('accountSecurity.regenerateText')"
      :title="t('accountSecurity.regenerate')"
      @confirm="regenerateCodes"
    />

    <ConfirmDialog
      v-model="deleteDialog"
      icon="mdi-delete-outline"
      :text="t('accountSecurity.deletePasskeyText', { name: deleteTarget?.name || t('auth.passkeyDefaultName') })"
      :title="t('accountSecurity.deletePasskey')"
      @confirm="deletePasskey"
    />
  </div>
</template>

<script lang="ts" setup>
  import type { Admin2FaStatusOutput, AdminPasskey } from '@/api/generated'
  import { onMounted, ref } from 'vue'
  import { useI18n } from 'vue-i18n'
  import { isApiError } from '@/api/client'
  import {
    adminPasskeyBegin,
    adminPasskeyFinish,
    adminRecoveryRegenerate,
    adminTotpRotateConfirm,
    adminTotpRotateSetup,
    deleteAdminPasskey,
    getAdmin2Fa,
  } from '@/api/generated'
  import ConfirmDialog from '@/components/ConfirmDialog.vue'
  import CopyField from '@/components/CopyField.vue'
  import EmptyState from '@/components/EmptyState.vue'
  import ErrorAlert from '@/components/ErrorAlert.vue'
  import OtpQr from '@/components/OtpQr.vue'
  import PageHeader from '@/components/PageHeader.vue'
  import { webauthnCreate } from '@/composables/webauthn'
  import { useSnackbarStore } from '@/stores/snackbar'

  const { t, te } = useI18n()
  const snackbar = useSnackbarStore()

  const loading = ref(false)
  const saving = ref(false)
  const loadError = ref<unknown>(null)
  const formError = ref<unknown>(null)
  const status = ref<Admin2FaStatusOutput | null>(null)
  const passkeyNotReady = ref(false)

  const rotateOpen = ref(false)
  const rotateSecret = ref<string | null>(null)
  const rotateOtpauth = ref<string | null>(null)
  const rotateCode = ref('')

  const regenDialog = ref(false)
  const codesOpen = ref(false)
  const newCodes = ref<string[]>([])

  const passkeyOpen = ref(false)
  const passkeyName = ref('')

  const deleteDialog = ref(false)
  const deleteTarget = ref<AdminPasskey | null>(null)

  function toastError (error: unknown): void {
    const code = isApiError(error) ? error.code : 'UNKNOWN_ERROR'
    snackbar.show(te(`errors.${code}`) ? t(`errors.${code}`) : String(error), 'error', 5000)
  }

  async function load (): Promise<void> {
    loading.value = true
    loadError.value = null
    try {
      const { data } = await getAdmin2Fa()
      status.value = data ?? null
    } catch (error) {
      loadError.value = error
    } finally {
      loading.value = false
    }
  }

  async function startRotate (): Promise<void> {
    formError.value = null
    rotateCode.value = ''
    try {
      const { data } = await adminTotpRotateSetup()
      rotateSecret.value = data?.secret ?? null
      rotateOtpauth.value = data?.otpauth_url ?? null
      rotateOpen.value = true
    } catch (error) {
      toastError(error)
    }
  }

  async function confirmRotate (): Promise<void> {
    if (!rotateCode.value) {
      return
    }
    saving.value = true
    formError.value = null
    try {
      await adminTotpRotateConfirm({ body: { code: rotateCode.value } })
      rotateOpen.value = false
      snackbar.show(t('accountSecurity.rotateDone'))
      await load()
    } catch (error) {
      formError.value = error
    } finally {
      saving.value = false
    }
  }

  async function regenerateCodes (): Promise<void> {
    try {
      const { data } = await adminRecoveryRegenerate()
      newCodes.value = data?.recovery_codes ?? []
      codesOpen.value = true
      snackbar.show(t('accountSecurity.regenerateDone'))
      await load()
    } catch (error) {
      toastError(error)
    }
  }

  async function copyNewCodes (): Promise<void> {
    await navigator.clipboard.writeText(newCodes.value.join('\n'))
    snackbar.show(t('common.copied'))
  }

  function openAddPasskey (): void {
    formError.value = null
    passkeyName.value = ''
    passkeyOpen.value = true
  }

  async function finishAddPasskey (): Promise<void> {
    saving.value = true
    formError.value = null
    passkeyNotReady.value = false
    try {
      const { data } = await adminPasskeyBegin()
      const credential = await webauthnCreate(data?.options)
      await adminPasskeyFinish({
        body: {
          credential,
          name: passkeyName.value || t('auth.passkeyDefaultName'),
        },
      })
      passkeyOpen.value = false
      snackbar.show(t('accountSecurity.passkeyAdded'))
      await load()
    } catch (error) {
      if (isApiError(error) && error.code === 'NOT_READY') {
        passkeyNotReady.value = true
        passkeyOpen.value = false
      }
      formError.value = error
    } finally {
      saving.value = false
    }
  }

  function askDeletePasskey (item: AdminPasskey): void {
    deleteTarget.value = item
    deleteDialog.value = true
  }

  async function deletePasskey (): Promise<void> {
    if (!deleteTarget.value?.id) {
      return
    }
    try {
      await deleteAdminPasskey({ path: { id: deleteTarget.value.id } })
      snackbar.show(t('common.delete') + ' ✓')
      await load()
    } catch (error) {
      toastError(error)
    }
  }

  onMounted(() => {
    void load()
  })
</script>
