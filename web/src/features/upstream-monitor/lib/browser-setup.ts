import type { MonitorBrowserSetup, MonitorSite } from '../types'

export function monitorSiteAuthorized(
  site: MonitorSite | undefined,
  setup: MonitorBrowserSetup | undefined
): boolean {
  if (!site || !setup) return false
  return [site.origin, ...site.allowed_origins].every((origin) => {
    let url: URL
    try {
      url = new URL(origin)
    } catch {
      return false
    }
    if (!['http:', 'https:'].includes(url.protocol)) return false
    return setup.granted_origins.some((pattern) => {
      if (pattern === '<all_urls>') return true
      const match = /^(\*|https?):\/\/([^/]+)\/\*$/.exec(pattern)
      if (!match || (match[1] !== '*' && `${match[1]}:` !== url.protocol)) {
        return false
      }
      const host = match[2].toLowerCase()
      if (host === '*') return true
      if (host.startsWith('*.')) {
        const suffix = host.slice(2)
        return url.hostname === suffix || url.hostname.endsWith(`.${suffix}`)
      }
      // Chrome host permissions without a port cover every port on that host.
      try {
        const granted = new URL(`${url.protocol}//${host}`)
        return (
          !granted.username &&
          !granted.password &&
          granted.hostname === url.hostname &&
          (!/:\d+$/.test(host) || granted.port === url.port)
        )
      } catch {
        return false
      }
    })
  })
}
