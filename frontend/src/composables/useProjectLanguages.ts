/**
 * composables/useProjectLanguages.ts
 *
 * Shared project language list for the languages tab, LocaleTabs, announcements, and preview.
 */
import type { ProjectLanguage } from '@/api/generated'
import type { MaybeRefOrGetter } from 'vue'
import { toValue } from 'vue'
import { listLanguages } from '@/api/generated'
import { useApiResource } from '@/composables/useApiResource'

export function useProjectLanguages (projectRef: MaybeRefOrGetter<string>) {
  return useApiResource<ProjectLanguage>(async () => {
    const { data } = await listLanguages({ path: { project_ref: toValue(projectRef) } })
    return data?.languages ?? []
  })
}

export function languageLabel (row: { code?: string, display_name?: string } | null | undefined): string {
  if (!row) {
    return ''
  }
  const name = row.display_name?.trim()
  return name || row.code || ''
}
