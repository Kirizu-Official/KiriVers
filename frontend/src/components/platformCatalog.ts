/**
 * Shared OS/arch combobox helpers: catalog `slug` (fallback `name`) union common presets.
 */

import { COMMON_ARCH, COMMON_OS } from '@/constants/platforms'

export type CatalogSlugEntry = {
  slug?: string
  name?: string
  aliases?: string[]
}

/** Live catalog entries → slug strings. `name` is only a stale-type fallback. */
export function catalogSlugs (entries: CatalogSlugEntry[] | undefined): string[] {
  const out: string[] = []
  for (const entry of entries ?? []) {
    const value = entry.slug ?? entry.name
    if (value) {
      out.push(value)
    }
  }
  return out
}

/** uniq(sorted(catalog ∪ common)) so dropdowns stay populated if catalog is empty. */
export function mergePlatformItems (catalog: readonly string[], common: readonly string[]): string[] {
  return [...new Set([...catalog, ...common])].toSorted((a, b) => a.localeCompare(b))
}

/**
 * Map a combobox value (canonical slug or catalog alias like darwin/amd64) to the catalog slug.
 * Unknown custom matrix slugs pass through lowercased so pair matching still works.
 */
export function resolveCatalogSlug (raw: string, entries: CatalogSlugEntry[] | undefined): string {
  const value = String(raw ?? '').trim().toLowerCase()
  if (!value) {
    return ''
  }
  for (const entry of entries ?? []) {
    const slug = (entry.slug ?? entry.name ?? '').trim().toLowerCase()
    if (!slug) {
      continue
    }
    if (slug === value) {
      return slug
    }
    if ((entry.aliases ?? []).some(alias => alias.trim().toLowerCase() === value)) {
      return slug
    }
  }
  return value
}

export function osComboboxItems (entries: CatalogSlugEntry[] | undefined): string[] {
  return mergePlatformItems(catalogSlugs(entries), COMMON_OS)
}

export function archComboboxItems (entries: CatalogSlugEntry[] | undefined): string[] {
  return mergePlatformItems(catalogSlugs(entries), COMMON_ARCH)
}
