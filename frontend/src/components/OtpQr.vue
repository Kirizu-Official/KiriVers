<!-- OtpQr.vue — 把 otpauth URL 画成可供身份验证器扫码的二维码 -->
<template>
  <div v-if="src" class="otp-qr d-flex justify-center">
    <img
      :alt="t('auth.totpQrAlt')"
      class="otp-qr__img"
      height="192"
      :src="src"
      width="192"
    >
  </div>
</template>

<script lang="ts" setup>
  import QRCode from 'qrcode'
  import { ref, watch } from 'vue'
  import { useI18n } from 'vue-i18n'

  const props = defineProps<{ value?: string | null }>()
  const { t } = useI18n()
  const src = ref('')
  let generation = 0

  watch(() => props.value, async next => {
    const id = ++generation
    src.value = ''
    if (!next) {
      return
    }
    try {
      const dataUrl = await QRCode.toDataURL(next, {
        color: { dark: '#000000', light: '#ffffff' },
        errorCorrectionLevel: 'M',
        margin: 2,
        width: 192,
      })
      if (id === generation) {
        src.value = dataUrl
      }
    } catch {
      if (id === generation) {
        src.value = ''
      }
    }
  }, { immediate: true })
</script>

<style scoped>
.otp-qr__img {
  width: 192px;
  height: 192px;
  border-radius: 12px;
  background: #fff;
}
</style>
