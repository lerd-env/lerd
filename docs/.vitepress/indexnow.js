import { LOCALES } from './theme/i18n/locales.js'

// IndexNow proves ownership with a public key file, so the key is not a secret.
// It must match docs/public/<KEY>.txt.
const KEY = 'f34919ee8997f86c61d202ff3dda3dba'
const HOST = 'lerd.sh'
const SITE_URL = `https://${HOST}`
const THEME = 'docs/.vitepress/theme/'

// The landing and about pages take their copy from the theme, not from their
// .md files, so those theme files count as edits to the pages they build.
const LANDINGS = Object.values(LOCALES).map((l) => `${SITE_URL}${l.path}`)
const THEME_PAGES = {
  'components/LandingPage.vue': LANDINGS,
  'landing-data.js': LANDINGS,
  'i18n/index.js': LANDINGS,
  'i18n/locales.js': LANDINGS,
  'components/AboutPage.vue': [`${SITE_URL}/about`],
  ...Object.fromEntries(Object.entries(LOCALES).map(([code, l]) => [`i18n/${code}.js`, [`${SITE_URL}${l.path}`]])),
}

// Turns changed repo paths into the lerd.sh URLs they publish, in the same
// clean-URL form the sitemap uses. Anything that is not a page is dropped.
export function changedUrls(files) {
  const urls = new Set()
  for (const file of files) {
    for (const url of pageUrls(file)) urls.add(url)
  }
  return [...urls]
}

function pageUrls(file) {
  if (file.startsWith(THEME)) return THEME_PAGES[file.slice(THEME.length)] ?? []
  const digest = file.match(/^docs\/public\/(digest\/[^/]+\.html)$/)
  if (digest) return [`${SITE_URL}/${digest[1]}`]
  const page = file.match(/^docs\/(.+)\.md$/)
  if (!page || page[1].startsWith('node_modules/') || page[1] === 'changelog-archive') return []
  return [`${SITE_URL}/${page[1].replace(/(^|\/)index$/, '$1')}`]
}

export function submission(urlList) {
  return { host: HOST, key: KEY, keyLocation: `${SITE_URL}/${KEY}.txt`, urlList }
}
