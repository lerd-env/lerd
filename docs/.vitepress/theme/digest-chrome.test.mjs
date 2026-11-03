import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

// The digest is static HTML, so its header and footer are copies of the Vue
// components; these tests catch a link added to one and not the other.
const read = (rel) => readFileSync(new URL(rel, import.meta.url), 'utf8')
const PAGES = ['../../digest-template.html', '../../public/digest/v1.38.0.html']

// Resolves the component's href bindings the way the landing page renders them off /.
function componentHrefs(vue) {
  const consts = Object.fromEntries([...vue.matchAll(/const (\w+) = '([^']+)'/g)].map((m) => [m[1], m[2]]))
  const tpl = vue.slice(vue.indexOf('<template>'))
  return [...tpl.matchAll(/(:?)href="([^"]+)"/g)].map(([, bound, v]) => {
    if (!bound) return v
    let m
    if ((m = v.match(/^`\$\{home\}(#\w+)`$/))) return '/' + m[1]
    if ((m = v.match(/^withBase\('([^']+)'\)$/))) return m[1]
    if (v in consts) return consts[v]
    throw new Error(`unhandled href binding ${v}`)
  })
}

function blockHrefs(html, open, close) {
  const start = html.indexOf(open)
  assert.ok(start >= 0, `missing ${open}`)
  const block = html.slice(start, html.indexOf(close, start))
  return [...block.matchAll(/href="([^"]+)"/g)].map((m) => m[1])
}

for (const page of PAGES) {
  const html = read(page)

  test(`${page} header links match SiteHeader.vue`, () => {
    assert.deepEqual(blockHrefs(html, '<header class="hdr">', '</header>'), componentHrefs(read('./components/SiteHeader.vue')))
  })

  test(`${page} footer links match SiteFooter.vue`, () => {
    assert.deepEqual(blockHrefs(html, '<footer class="site-foot">', '</footer>'), componentHrefs(read('./components/SiteFooter.vue')))
  })
}
