/**
 * Shared project drawer + tab items. Highlight uses the longest `to` prefix of the current path
 * so `/versions/:v/gray` stays on Versions and overview/settings do not steal each other.
 */
export interface ProjectNavItem {
  to: string
  title: string
  icon: string
}

export function projectNavItems (projectRef: string, t: (key: string) => string): ProjectNavItem[] {
  const base = `/projects/${projectRef}`
  return [
    { to: `${base}/overview`, icon: 'mdi-chart-box-outline', title: t('nav.overview') },
    { to: `${base}/settings`, icon: 'mdi-tune-variant', title: t('nav.settings') },
    { to: `${base}/clients`, icon: 'mdi-cellphone-link', title: t('nav.clients') },
    { to: `${base}/channels`, icon: 'mdi-source-branch', title: t('nav.channels') },
    { to: `${base}/matrix`, icon: 'mdi-server', title: t('nav.matrix') },
    { to: `${base}/hw-revs`, icon: 'mdi-chip', title: t('nav.hwRevs') },
    { to: `${base}/languages`, icon: 'mdi-translate', title: t('nav.languages') },
    { to: `${base}/versions`, icon: 'mdi-tag-multiple-outline', title: t('nav.versions') },
    { to: `${base}/announcements`, icon: 'mdi-bullhorn-outline', title: t('nav.announcements') },
    { to: `${base}/tokens`, icon: 'mdi-key-variant', title: t('nav.tokens') },
    { to: `${base}/release`, icon: 'mdi-rocket-launch-outline', title: t('nav.release') },
    { to: `${base}/audit`, icon: 'mdi-text-box-search-outline', title: t('nav.audit') },
  ]
}

export function matchProjectNav (path: string, items: Array<{ to: string }>): string {
  const matches = items.filter(item => path === item.to || path.startsWith(`${item.to}/`))
  matches.sort((a, b) => b.to.length - a.to.length)
  return matches[0]?.to ?? path
}
