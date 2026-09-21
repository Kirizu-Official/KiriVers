import type { ChangelogMap } from '@/api/generated'

function localeMarkdown (value: unknown): string {
  if (typeof value === 'string') {
    return value.trim()
  }
  if (value && typeof value === 'object' && 'markdown' in value) {
    const markdown = (value as { markdown?: unknown }).markdown
    return typeof markdown === 'string' ? markdown.trim() : ''
  }
  return ''
}

/** Flatten GET ChangelogMap into LocaleTabs locale→markdown strings. */
export function flattenChangelogMap (raw: ChangelogMap | undefined | null): Record<string, string> {
  if (!raw) {
    return {}
  }
  const out: Record<string, string> = {}
  for (const [locale, value] of Object.entries(raw)) {
    const markdown = localeMarkdown(value)
    if (markdown) {
      out[locale] = markdown
    }
  }
  return out
}

/** LocaleTabs draft → PUT/PATCH changelog_i18n. Empty map is omitted to avoid 400. */
export function changelogDraftToI18n (draft: Record<string, string> | null | undefined): ChangelogMap | undefined {
  if (!draft) {
    return undefined
  }
  const out: ChangelogMap = {}
  for (const [locale, markdown] of Object.entries(draft)) {
    const text = markdown.trim()
    if (text) {
      out[locale] = { markdown: text }
    }
  }
  return Object.keys(out).length > 0 ? out : undefined
}
