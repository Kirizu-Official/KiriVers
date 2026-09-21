<!-- pages/login.vue — 管理登录多步向导（blank 布局：深色科技画布 + 居中玻璃卡片） -->
<template>
  <div class="login-shell" :class="themeClass">
    <div aria-hidden="true" class="fx">
      <div class="fx-grid" />
      <div class="fx-orb fx-orb-a" />
      <div class="fx-orb fx-orb-b" />
      <div class="fx-orb fx-orb-c" />
      <div class="fx-vignette" />
    </div>

    <div class="login-layout">
      <v-card class="glass-card" :class="{ wide: stage !== 'password' }" elevation="0" rounded="xl">
        <div class="card-head">
          <div class="brand-mark">
            <v-icon icon="mdi-cloud-upload-outline" size="22" />
          </div>

          <div>
            <div class="card-title">{{ stageTitle }}</div>
            <div class="card-subtitle">{{ stageSubtitle }}</div>
          </div>
        </div>

        <v-alert v-if="sessionExpired && stage === 'password'" class="mb-6" density="compact" type="warning">
          {{ t('auth.sessionExpired') }}
        </v-alert>

        <v-alert v-if="apiError" class="mb-6" density="compact" type="error">
          {{ errorText }}
        </v-alert>

        <v-form v-if="stage === 'password'" class="login-form" @submit.prevent="submitPassword">
          <v-text-field
            v-model="username"
            autocomplete="username"
            autofocus
            :label="t('auth.username')"
            prepend-inner-icon="mdi-account-outline"
            :rules="[required]"
            variant="outlined"
          />

          <v-text-field
            v-model="password"
            autocomplete="current-password"
            :label="t('auth.password')"
            prepend-inner-icon="mdi-lock-outline"
            :rules="[required]"
            type="password"
            variant="outlined"
          />

          <v-btn
            block
            class="submit-btn mt-2"
            :loading="loading"
            size="large"
            type="submit"
          >
            {{ loading ? t('auth.loggingIn') : t('auth.submit') }}
          </v-btn>
        </v-form>

        <v-form v-else-if="stage === 'enroll_totp'" class="login-form" @submit.prevent="confirmTotp">
          <div class="text-caption mb-2">{{ t('auth.totpQrHint') }}</div>
          <OtpQr :value="auth.otpauthUrl" />
          <div class="text-caption mt-4 mb-2">{{ t('auth.totpSecretHint') }}</div>
          <CopyField :value="auth.totpSecret" />
          <div class="text-caption mt-4 mb-2">{{ t('auth.otpauthHint') }}</div>
          <CopyField :value="auth.otpauthUrl" />

          <v-text-field
            v-model="totpCode"
            autocomplete="one-time-code"
            class="mt-4"
            inputmode="numeric"
            :label="t('auth.totpCode')"
            maxlength="8"
            prepend-inner-icon="mdi-shield-key-outline"
            :rules="[required]"
            variant="outlined"
          />

          <v-btn
            block
            class="submit-btn mt-2"
            :loading="loading"
            size="large"
            type="submit"
          >
            {{ t('auth.confirmTotp') }}
          </v-btn>
        </v-form>

        <v-form v-else-if="stage === 'ack_recovery'" class="login-form" @submit.prevent="ackRecovery">
          <v-alert class="mb-4" density="compact" type="warning">
            {{ t('auth.recoveryOnceHint') }}
          </v-alert>

          <div class="recovery-codes mb-4">
            <CopyField v-for="code in auth.recoveryCodes" :key="code" :value="code" />
          </div>

          <v-btn class="mb-4" prepend-icon="mdi-content-copy" variant="tonal" @click="copyRecoveryCodes">
            {{ t('auth.copyAllCodes') }}
          </v-btn>

          <v-checkbox v-model="savedCodes" color="primary" :label="t('auth.savedCodes')" />

          <v-btn
            block
            class="submit-btn mt-2"
            :disabled="!savedCodes"
            :loading="loading"
            size="large"
            type="submit"
          >
            {{ t('auth.continue') }}
          </v-btn>
        </v-form>

        <div v-else-if="stage === 'optional_passkey'" class="login-form">
          <v-text-field
            v-model="passkeyName"
            :label="t('auth.passkeyName')"
            prepend-inner-icon="mdi-key-outline"
            variant="outlined"
          />

          <v-btn
            block
            class="submit-btn mt-2"
            :loading="loading"
            size="large"
            @click="registerPasskey"
          >
            {{ t('auth.registerPasskey') }}
          </v-btn>

          <v-btn
            block
            class="mt-3"
            :loading="loading"
            size="large"
            variant="tonal"
            @click="skipPasskey"
          >
            {{ t('auth.skipPasskey') }}
          </v-btn>
        </div>

        <div v-else-if="stage === 'second_factor'" class="login-form">
          <v-btn-toggle
            v-model="factorTab"
            class="mb-4 w-100"
            color="primary"
            divided
            mandatory
            variant="outlined"
          >
            <v-btn class="flex-grow-1" value="totp">{{ t('auth.factorTotp') }}</v-btn>
            <v-btn class="flex-grow-1" value="recovery">{{ t('auth.factorRecovery') }}</v-btn>
            <v-btn class="flex-grow-1" value="passkey">{{ t('auth.factorPasskey') }}</v-btn>
          </v-btn-toggle>

          <v-form v-if="factorTab === 'totp'" @submit.prevent="verifyTotp">
            <v-text-field
              v-model="totpCode"
              autocomplete="one-time-code"
              inputmode="numeric"
              :label="t('auth.totpCode')"
              maxlength="8"
              prepend-inner-icon="mdi-shield-key-outline"
              :rules="[required]"
              variant="outlined"
            />

            <v-btn
              block
              class="submit-btn mt-2"
              :loading="loading"
              size="large"
              type="submit"
            >
              {{ t('auth.verify') }}
            </v-btn>
          </v-form>

          <v-form v-else-if="factorTab === 'recovery'" @submit.prevent="verifyRecovery">
            <v-text-field
              v-model="recoveryCode"
              :label="t('auth.recoveryCode')"
              prepend-inner-icon="mdi-backup-restore"
              :rules="[required]"
              variant="outlined"
            />

            <v-btn
              block
              class="submit-btn mt-2"
              :loading="loading"
              size="large"
              type="submit"
            >
              {{ t('auth.verify') }}
            </v-btn>
          </v-form>

          <div v-else class="d-flex flex-column">
            <v-btn
              block
              class="submit-btn"
              :loading="loading"
              size="large"
              @click="verifyPasskey"
            >
              {{ t('auth.usePasskey') }}
            </v-btn>
          </div>
        </div>

        <v-btn
          v-if="stage !== 'password'"
          block
          class="mt-4"
          variant="text"
          @click="restart"
        >
          {{ t('auth.restart') }}
        </v-btn>
      </v-card>

      <!-- 项目主页（常量：项目名与官网不是可译文案，不进 locales）；blank 布局无 footer/关于按钮 -->
      <a
        class="mt-4 text-caption text-decoration-none text-medium-emphasis"
        href="https://kirivers.kirizu.dev"
        rel="noopener noreferrer"
        target="_blank"
      >
        KiriVers
      </a>
    </div>
  </div>
</template>

<script lang="ts" setup>
  import { computed, ref, watch } from 'vue'
  import { useI18n } from 'vue-i18n'
  import { useRoute, useRouter } from 'vue-router'
  import { isApiError } from '@/api/client'
  import {
    adminRecoveryAck,
    adminSkipPasskey,
    adminTotpConfirm,
    adminTotpSetup,
    adminVerify2Fa,
    adminWebauthnLoginBegin,
    adminWebauthnLoginFinish,
    adminWebauthnRegisterBegin,
    adminWebauthnRegisterFinish,
  } from '@/api/generated'
  import CopyField from '@/components/CopyField.vue'
  import OtpQr from '@/components/OtpQr.vue'
  import { useBuildInfo } from '@/composables/useBuildInfo'
  import { webauthnCreate, webauthnGet } from '@/composables/webauthn'
  import { DARK_THEME } from '@/plugins/vuetify'
  import { useAuthStore } from '@/stores/auth'
  import { useSnackbarStore } from '@/stores/snackbar'
  import { useUiStore } from '@/stores/ui'

  const build = useBuildInfo()
  definePage({ meta: { layout: 'blank' } })

  const { t, te } = useI18n()
  const route = useRoute('/login')
  const router = useRouter()
  const auth = useAuthStore()
  const snackbar = useSnackbarStore()
  const ui = useUiStore()
  const themeClass = computed(() => ui.resolvedName() === DARK_THEME ? 'v-theme--kirivers-dark' : 'v-theme--kirivers-light')

  const username = ref('')
  const password = ref('')
  const totpCode = ref('')
  const recoveryCode = ref('')
  const passkeyName = ref('')
  const savedCodes = ref(false)
  const factorTab = ref<'totp' | 'recovery' | 'passkey'>(
    auth.hasPending && auth.pendingStage === 'second_factor' && auth.hasPasskey ? 'passkey' : 'totp',
  )
  const loading = ref(false)
  const apiError = ref<unknown>(null)
  let setupInflight = false

  const sessionExpired = computed(() => route.query.expired === '1')
  const stage = computed(() => {
    if (!auth.hasPending) {
      return 'password'
    }
    return auth.pendingStage ?? 'password'
  })

  const stageTitle = computed(() => {
    switch (stage.value) {
      case 'enroll_totp': {
        return t('auth.enrollTitle')
      }
      case 'ack_recovery': {
        return t('auth.recoveryTitle')
      }
      case 'optional_passkey': {
        return t('auth.passkeyTitle')
      }
      case 'second_factor': {
        return t('auth.secondFactorTitle')
      }
      default: {
        return t('auth.title')
      }
    }
  })

  const stageSubtitle = computed(() => {
    switch (stage.value) {
      case 'enroll_totp': {
        return t('auth.enrollSubtitle')
      }
      case 'ack_recovery': {
        return t('auth.recoverySubtitle')
      }
      case 'optional_passkey': {
        return t('auth.passkeySubtitle')
      }
      case 'second_factor': {
        return t('auth.secondFactorSubtitle')
      }
      default: {
        return t('auth.subtitle')
      }
    }
  })

  const errorText = computed(() => {
    const err = apiError.value
    if (isApiError(err)) {
      // Password / 2FA failures share UNAUTHORIZED; only expired tokens mean session lost.
      if (err.code === 'UNAUTHORIZED' && err.message === 'invalid credentials') {
        return t('auth.invalidCredentials')
      }
      const key = `errors.${err.code}`
      return te(key) ? t(key) : err.message
    }
    return err instanceof Error ? err.message : String(err ?? '')
  })

  function required (value: string): boolean | string {
    return Boolean(value) || t('common.required')
  }

  async function goHome (): Promise<void> {
    const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : '/projects'
    await build.ensureLoaded()

    await router.push(redirect)
  }

  async function finishIfComplete (status: string | undefined): Promise<boolean> {
    if (status === 'complete' && auth.isAuthenticated) {
      await goHome()
      return true
    }
    return false
  }

  async function ensureTotpSetup (): Promise<void> {
    if (stage.value !== 'enroll_totp' || auth.totpSecret || setupInflight) {
      return
    }
    setupInflight = true
    loading.value = true
    apiError.value = null
    try {
      const { data } = await adminTotpSetup()
      auth.applyAuthResult(data)
    } catch (error) {
      apiError.value = error
    } finally {
      setupInflight = false
      loading.value = false
    }
  }

  watch(stage, () => {
    if (stage.value === 'enroll_totp') {
      void ensureTotpSetup()
    }
  }, { immediate: true })

  async function submitPassword (): Promise<void> {
    if (!username.value || !password.value) {
      return
    }
    loading.value = true
    apiError.value = null
    try {
      const result = await auth.login(username.value, password.value)
      password.value = ''
      if (result.status === 'pending' && result.stage === 'second_factor') {
        factorTab.value = auth.hasPasskey ? 'passkey' : 'totp'
      }
      if (await finishIfComplete(result.status)) {
        return
      }
    } catch (error) {
      apiError.value = error
    } finally {
      loading.value = false
    }
  }

  async function confirmTotp (): Promise<void> {
    if (!totpCode.value) {
      return
    }
    loading.value = true
    apiError.value = null
    try {
      const { data } = await adminTotpConfirm({ body: { code: totpCode.value } })
      totpCode.value = ''
      auth.applyAuthResult(data)
    } catch (error) {
      apiError.value = error
    } finally {
      loading.value = false
    }
  }

  async function copyRecoveryCodes (): Promise<void> {
    await navigator.clipboard.writeText(auth.recoveryCodes.join('\n'))
    snackbar.show(t('common.copied'))
  }

  async function ackRecovery (): Promise<void> {
    if (!savedCodes.value) {
      return
    }
    loading.value = true
    apiError.value = null
    try {
      const { data } = await adminRecoveryAck({ body: { confirmed: true } })
      auth.applyAuthResult(data)
    } catch (error) {
      apiError.value = error
    } finally {
      loading.value = false
    }
  }

  async function skipPasskey (): Promise<void> {
    loading.value = true
    apiError.value = null
    try {
      const { data } = await adminSkipPasskey()
      auth.applyAuthResult(data)
      await finishIfComplete(data?.status)
    } catch (error) {
      apiError.value = error
    } finally {
      loading.value = false
    }
  }

  async function registerPasskey (): Promise<void> {
    loading.value = true
    apiError.value = null
    try {
      const { data } = await adminWebauthnRegisterBegin()
      const credential = await webauthnCreate(data?.options)
      const finished = await adminWebauthnRegisterFinish({
        body: {
          credential,
          name: passkeyName.value || t('auth.passkeyDefaultName'),
        },
      })
      auth.applyAuthResult(finished.data)
      await finishIfComplete(finished.data?.status)
    } catch (error) {
      apiError.value = error
    } finally {
      loading.value = false
    }
  }

  async function verifyTotp (): Promise<void> {
    if (!totpCode.value) {
      return
    }
    loading.value = true
    apiError.value = null
    try {
      const { data } = await adminVerify2Fa({ body: { totp: totpCode.value } })
      totpCode.value = ''
      auth.applyAuthResult(data)
      await finishIfComplete(data?.status)
    } catch (error) {
      apiError.value = error
    } finally {
      loading.value = false
    }
  }

  async function verifyRecovery (): Promise<void> {
    if (!recoveryCode.value) {
      return
    }
    loading.value = true
    apiError.value = null
    try {
      const { data } = await adminVerify2Fa({ body: { recovery_code: recoveryCode.value } })
      recoveryCode.value = ''
      auth.applyAuthResult(data)
      await finishIfComplete(data?.status)
    } catch (error) {
      apiError.value = error
    } finally {
      loading.value = false
    }
  }

  async function verifyPasskey (): Promise<void> {
    loading.value = true
    apiError.value = null
    try {
      const { data } = await adminWebauthnLoginBegin()
      const credential = await webauthnGet(data?.options)
      const finished = await adminWebauthnLoginFinish({ body: { credential } })
      auth.applyAuthResult(finished.data)
      await finishIfComplete(finished.data?.status)
    } catch (error) {
      apiError.value = error
    } finally {
      loading.value = false
    }
  }

  function restart (): void {
    apiError.value = null
    totpCode.value = ''
    recoveryCode.value = ''
    savedCodes.value = false
    auth.clearPending()
  }
</script>

<style scoped>
.login-shell {
    position: relative;
    min-height: 100dvh;
    overflow: hidden;
    display: flex;
    flex-direction: column;
    background:
        radial-gradient(120% 90% at 50% 8%, #131a33 0%, transparent 55%),
        radial-gradient(100% 80% at 50% 100%, #0c1428 0%, transparent 60%),
        #070b16;
    color: rgb(var(--v-theme-on-surface));
}

.fx {
    position: absolute;
    inset: 0;
    pointer-events: none;
}

.fx-grid {
    position: absolute;
    inset: -1px;
    background-image:
        linear-gradient(rgba(148, 163, 255, 0.07) 1px, transparent 1px),
        linear-gradient(90deg, rgba(148, 163, 255, 0.07) 1px, transparent 1px);
    background-size: 44px 44px;
    -webkit-mask-image: radial-gradient(85% 65% at 50% 48%, #000 0%, transparent 78%);
    mask-image: radial-gradient(85% 65% at 50% 48%, #000 0%, transparent 78%);
}

.fx-orb {
    position: absolute;
    border-radius: 50%;
    filter: blur(110px);
    opacity: 0.42;
    animation: orb-drift 22s ease-in-out infinite alternate;
}

.fx-orb-a {
    width: 560px;
    height: 560px;
    top: -180px;
    left: calc(50% - 390px);
    background: #4f5bd5;
}

.fx-orb-b {
    width: 480px;
    height: 480px;
    right: 8%;
    top: 18%;
    background: #7c4dff;
    animation-delay: -7s;
    animation-duration: 26s;
}

.fx-orb-c {
    width: 520px;
    height: 520px;
    bottom: -220px;
    left: calc(50% - 156px);
    background: #00897b;
    opacity: 0.3;
    animation-delay: -13s;
    animation-duration: 30s;
}

.fx-vignette {
    position: absolute;
    inset: 0;
    background: radial-gradient(120% 100% at 50% 50%, transparent 55%, rgba(4, 6, 14, 0.55) 100%);
}

@keyframes orb-drift {
    from {
        transform: translate3d(0, 0, 0) scale(1);
    }

    to {
        transform: translate3d(60px, 40px, 0) scale(1.12);
    }
}

@media (prefers-reduced-motion: reduce) {
    .fx-orb {
        animation: none;
    }
}

.login-layout {
    position: relative;
    flex: 1;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    width: 100%;
    padding: 48px 40px;
}

.brand-mark {
    display: grid;
    place-items: center;
    width: 42px;
    height: 42px;
    flex: none;
    border-radius: 12px;
    color: #fff;
    background: linear-gradient(135deg, #4f5bd5 0%, #7c4dff 100%);
    box-shadow:
        0 12px 32px rgba(79, 91, 213, 0.45),
        inset 0 1px 0 rgba(255, 255, 255, 0.25);
}

.glass-card {
    width: min(420px, 100%);
    padding: 36px 32px 28px;
    background: linear-gradient(160deg, rgba(255, 255, 255, 0.075), rgba(255, 255, 255, 0.03));
    border: 1px solid rgba(255, 255, 255, 0.12);
    backdrop-filter: blur(18px) saturate(140%);
    -webkit-backdrop-filter: blur(18px) saturate(140%);
    box-shadow:
        0 24px 64px rgba(3, 6, 18, 0.55),
        inset 0 1px 0 rgba(255, 255, 255, 0.09);
}

.glass-card.wide {
    width: min(520px, 100%);
}

.card-head {
    display: flex;
    align-items: center;
    gap: 14px;
    margin-bottom: 26px;
}

.card-title {
    font-size: 1.25rem;
    font-weight: 700;
    color: #fff;
    line-height: 1.3;
}

.card-subtitle {
    margin-top: 2px;
    font-size: 0.8rem;
    color: rgba(203, 210, 245, 0.62);
}

.login-form {
    display: flex;
    flex-direction: column;
    gap: 4px;
}

.recovery-codes {
    display: flex;
    flex-direction: column;
    gap: 8px;
    max-height: 240px;
    overflow: auto;
}

.submit-btn {
    background: linear-gradient(135deg, #4f5bd5 0%, #7c4dff 100%);
    color: #fff;
    box-shadow: 0 10px 28px rgba(98, 91, 232, 0.4);
    transition: box-shadow 0.2s ease, filter 0.2s ease;
}

.submit-btn:hover {
    filter: brightness(1.08);
    box-shadow: 0 14px 36px rgba(112, 100, 245, 0.55);
}

.login-shell.v-theme--kirivers-light {
    background:
        radial-gradient(120% 90% at 50% 8%, #e8ebf8 0%, transparent 55%),
        radial-gradient(100% 80% at 50% 100%, #eef1f8 0%, transparent 60%),
        #f6f7fb;
}

.login-shell.v-theme--kirivers-light .glass-card {
    background: linear-gradient(160deg, rgba(255, 255, 255, 0.92), rgba(255, 255, 255, 0.78));
    border: 1px solid rgba(15, 23, 42, 0.08);
    box-shadow:
        0 24px 64px rgba(79, 91, 213, 0.12),
        inset 0 1px 0 rgba(255, 255, 255, 0.8);
}

.login-shell.v-theme--kirivers-light .card-title {
    color: rgb(var(--v-theme-on-surface));
}

.login-shell.v-theme--kirivers-light .card-subtitle {
    color: rgba(74, 77, 85, 0.78);
}

.login-shell.v-theme--kirivers-light .fx-vignette {
    background: radial-gradient(120% 100% at 50% 50%, transparent 55%, rgba(246, 247, 251, 0.35) 100%);
}

@media (max-width: 959px) {
    .login-layout {
        padding: 32px 20px;
    }
}

@media (prefers-reduced-motion: reduce) {
    .submit-btn {
        transition: none;
    }
}
</style>
