import { test } from 'node:test'
import assert from 'node:assert/strict'
import { LOCALES, localeOf, pickLocale, redirectScript } from './i18n/locales.js'
import en from './i18n/en.js'

const ALL = ['en', 'es', 'pt', 'zh', 'zh-TW', 'ja']

test('a regional variant gets its language', () => {
  assert.equal(pickLocale(['es-MX'], ALL), 'es')
  assert.equal(pickLocale(['pt-BR', 'en'], ALL), 'pt')
})

test('the first supported language in the list wins', () => {
  assert.equal(pickLocale(['ko-KR', 'ja', 'es'], ALL), 'ja')
})

test('English first keeps English even when another follows', () => {
  assert.equal(pickLocale(['en-GB', 'es'], ALL), 'en')
})

test('Traditional Chinese readers get zh-TW, the rest zh', () => {
  assert.equal(pickLocale(['zh-TW'], ALL), 'zh-TW')
  assert.equal(pickLocale(['zh-HK'], ALL), 'zh-TW')
  assert.equal(pickLocale(['zh-Hant-TW'], ALL), 'zh-TW')
  assert.equal(pickLocale(['zh-CN'], ALL), 'zh')
})

test('nothing supported, or no list at all, falls back to English', () => {
  assert.equal(pickLocale(['ko', 'ar'], ALL), 'en')
  assert.equal(pickLocale([], ALL), 'en')
  assert.equal(pickLocale(undefined, ALL), 'en')
})

test('a page belongs to the locale its folder names', () => {
  assert.equal(localeOf('index.md'), 'en')
  assert.equal(localeOf('es/index.md'), 'es')
  assert.equal(localeOf('usage/sites.md'), 'en')
})

test('the inlined redirect script parses on its own', () => {
  assert.doesNotThrow(() => new Function(redirectScript()))
})

// Every locale is checked against English, so a missing or extra string fails here, not on the page.
function shape(v) {
  if (Array.isArray(v)) return v.map(shape)
  if (v && typeof v === 'object') return Object.fromEntries(Object.keys(v).sort().map((k) => [k, shape(v[k])]))
  return typeof v
}

for (const code of Object.keys(LOCALES).filter((c) => c !== 'en')) {
  test(`${code} has every English string and nothing else`, async () => {
    const t = (await import(`./i18n/${code}.js`)).default
    assert.deepEqual(shape(t), shape(en))
  })
}

test('the copy lines up with the data it is joined to', async () => {
  const D = await import('./landing-data.js')
  assert.deepEqual(D.LINEUP.map((c) => c.id), Object.keys(en.lineup.items))
  assert.equal(en.agents.items.length, D.AGENT_CMDS.length)
  assert.equal(en.runtimes.items.length, D.RUNTIME_CMDS.length)
  assert.equal(en.platforms.items.length, D.PLATFORMS.length)
  assert.equal(en.compare.rows.length, D.CMP.rows.length)
  assert.equal(en.faq.items.length, D.FAQ_LINKS.length)
  D.FAQ_LINKS.forEach((href, i) => assert.equal(Boolean(href), 'link' in en.faq.items[i], `FAQ ${i}`))
})
