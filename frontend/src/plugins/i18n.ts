/**
 * plugins/i18n.ts — vue-i18n 装配：zh-CN 默认、en 兜底。
 * 语言持久化由 stores/ui 负责写入 localStorage；
 * 本文件在创建时读取一次以确定初始语言。
 */
import { createI18n } from 'vue-i18n'
import en from '@/locales/en'
import zhCN from '@/locales/zh-CN'

function initialLocale (): string {
  try {
    const raw = localStorage.getItem('kirivers.ui')
    if (raw) {
      const parsed = JSON.parse(raw) as { locale?: string }
      if (parsed.locale === 'en') {
        return 'en'
      }
    }
  } catch {
    // 忽略损坏数据
  }
  return 'zh-CN'
}

const i18n = createI18n({
  legacy: false,
  locale: initialLocale(),
  fallbackLocale: 'zh-CN',
  messages: {
    'zh-CN': zhCN,
    en,
  },
})

export default i18n
