// The landing page speaks the same languages as the dashboard
// (internal/ui/web/project.inlang/settings.json). Each one lives under /<path>/.
export const LOCALES = {
  en: { label: 'English', path: '/', og: 'en_US' },
  es: { label: 'Español', path: '/es/', og: 'es_ES' },
  pt: { label: 'Português', path: '/pt/', og: 'pt_BR' },
  fr: { label: 'Français', path: '/fr/', og: 'fr_FR' },
  de: { label: 'Deutsch', path: '/de/', og: 'de_DE' },
  id: { label: 'Bahasa Indonesia', path: '/id/', og: 'id_ID' },
  nl: { label: 'Nederlands', path: '/nl/', og: 'nl_NL' },
  tr: { label: 'Türkçe', path: '/tr/', og: 'tr_TR' },
  zh: { label: '简体中文', path: '/zh/', og: 'zh_CN' },
  'zh-TW': { label: '繁體中文', path: '/zh-tw/', og: 'zh_TW' },
  ja: { label: '日本語', path: '/ja/', og: 'ja_JP' },
  ro: { label: 'Română', path: '/ro/', og: 'ro_RO' },
  it: { label: 'Italiano', path: '/it/', og: 'it_IT' },
  pl: { label: 'Polski', path: '/pl/', og: 'pl_PL' },
  vi: { label: 'Tiếng Việt', path: '/vi/', og: 'vi_VN' },
}

export const STORAGE_KEY = 'lerd-lang'

// pickLocale maps the browser's language list onto a supported code. It is
// inlined into the page head as source, so it must stay self-contained.
export function pickLocale(languages, supported) {
  const lower = supported.map((c) => c.toLowerCase())
  for (const raw of languages || []) {
    const tag = String(raw).toLowerCase()
    const exact = lower.indexOf(tag)
    if (exact !== -1) return supported[exact]
    const base = tag.split('-')[0]
    // Traditional script readers get zh-TW, everyone else writing Chinese gets zh.
    if (base === 'zh' && /-(tw|hk|mo|hant)\b/.test(tag) && lower.includes('zh-tw')) return supported[lower.indexOf('zh-tw')]
    const loose = lower.indexOf(base)
    if (loose !== -1) return supported[loose]
  }
  return 'en'
}

// localeOf names the locale a page belongs to from its path under docs/.
export function localeOf(relativePath) {
  const first = relativePath.split('/')[0]
  const code = Object.keys(LOCALES).find((c) => c !== 'en' && LOCALES[c].path === `/${first}/`)
  return code || 'en'
}

// redirectScript runs before the page paints: only the English root moves, and
// only for a visitor who has never picked a language on the site.
export function redirectScript() {
  const paths = Object.fromEntries(Object.entries(LOCALES).map(([c, l]) => [c, l.path]))
  return `(function(){try{if(location.pathname!=='/')return;var p=${JSON.stringify(paths)};var pick=${pickLocale.toString()};var c=localStorage.getItem(${JSON.stringify(STORAGE_KEY)})||pick(navigator.languages||[navigator.language],Object.keys(p));if(p[c]&&c!=='en')location.replace(p[c]+location.hash)}catch(e){}})()`
}
