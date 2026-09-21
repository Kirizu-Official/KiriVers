/**
 * Pick a GeoIP place name for the current console locale.
 * names[locale] → names[lang] → names.en → Intl.DisplayNames → ISO code → unknown.
 */
export function geoLabel (
  names: Record<string, string> | null | undefined,
  locale: string,
  code?: string | null,
  unknown = '',
): string {
  const map = names ?? {}
  const lang = locale.split('-', 2)[0] ?? locale
  const fromMap = map[locale] || map[lang] || map.en
  if (fromMap) {
    return fromMap
  }
  const iso = (code ?? '').trim()
  if (iso) {
    try {
      const label = new Intl.DisplayNames([locale], { type: 'region' }).of(iso)
      if (label) {
        return label
      }
    } catch {
      // ignore unsupported locale/code
    }
    return iso
  }
  return unknown
}
