import { test } from 'node:test'
import assert from 'node:assert/strict'
import { changedUrls, submission } from './indexnow.js'

test('maps changed pages to their clean URLs', () => {
  assert.deepEqual(changedUrls([
    'docs/index.md',
    'docs/de/index.md',
    'docs/usage/sites.md',
    'docs/changelog.md',
    'docs/public/digest/v1.39.2.html',
  ]), [
    'https://lerd.sh/',
    'https://lerd.sh/de/',
    'https://lerd.sh/usage/sites',
    'https://lerd.sh/changelog',
    'https://lerd.sh/digest/v1.39.2.html',
  ])
})

test('skips files that are not pages, and the noindex archive', () => {
  assert.deepEqual(changedUrls([
    'docs/changelog-archive.md',
    'docs/.vitepress/config.ts',
    'docs/public/assets/logo.svg',
    'docs/node_modules/x/README.md',
    'internal/ui/web/README.md',
    '',
  ]), [])
})

test('builds the request IndexNow expects, key file included', () => {
  const body = submission(['https://lerd.sh/'])
  assert.equal(body.host, 'lerd.sh')
  assert.equal(body.keyLocation, `https://lerd.sh/${body.key}.txt`)
  assert.deepEqual(body.urlList, ['https://lerd.sh/'])
})
