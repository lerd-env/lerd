<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useData, withBase } from 'vitepress'
import * as D from '../landing-data.js'
import { data as starData } from '../stars.data'
import SiteHeader from './SiteHeader.vue'
import SiteFooter from './SiteFooter.vue'
import { data as storeData } from '../store.data'

const GITHUB = 'https://github.com/lerd-env/lerd'
const pad = (n) => String(n).padStart(2, '0')
const { isDark, frontmatter, lang } = useData()
// config.ts puts this page's own locale copy here (i18n/en.js is the shape).
const t = computed(() => frontmatter.value.t)
const fill = (s, vars) => s.replace(/\{(\w+)\}/g, (_, k) => vars[k])
// A sentence with {name} slots splits into text and slot parts, so each language keeps its own word order.
const parts = (s) => s.split(/(\{\w+\})/).filter(Boolean).map((x) => (/^\{\w+\}$/.test(x) ? { slot: x.slice(1, -1) } : { text: x }))
// Each lineup capture has a -light twin taken from the demo in the light theme.
const shot = (c, light) => withBase(`/assets/${c.shot}${light ? '-light' : ''}.png`)
// The cards show the capture at most ~620px wide, so they load WebP copies; the zoom keeps the PNG.
const shotSet = (c, light) => [640, 1280].map((w) => `${withBase(`/assets/${c.shot}${light ? '-light' : ''}-${w}.webp`)} ${w}w`).join(', ')
const SHOT_SIZES = '(max-width: 760px) calc(100vw - 56px), 620px'

const stars = starData.stars
const starLabel = (n) => (n >= 1000 ? (n / 1000).toFixed(1).replace(/\.0$/, '') + 'k' : String(n))

const PLUGINS = 'https://plugins.lerd.sh/'
const CMP_LINKS = { herd: '/getting-started/herd-linux', laragon: '/getting-started/laragon-linux', full: '/getting-started/comparison' }
const counts = ref({ frameworks: storeData.frameworks, services: storeData.services, plugins: storeData.plugins })
onMounted(async () => {
  const live = await D.liveCounts(storeData.builtin)
  for (const key of Object.keys(live)) if (live[key]) counts.value[key] = live[key]
})

const copied = ref(false)
let copyTimer = null
async function copyInstall() {
  try { await navigator.clipboard.writeText(D.INSTALL) } catch (e) { return }
  copied.value = true
  clearTimeout(copyTimer)
  copyTimer = setTimeout(() => { copied.value = false }, 1600)
}
onBeforeUnmount(() => clearTimeout(copyTimer))

// The lineup advances when the active tab's progress bar finishes its CSS
// animation, so the bar and the slide change can never drift apart.
const slide = ref(0)
const hovered = ref(false)
// Keyboard focus pauses it too, so a focused link is never hidden under the reader.
const focused = ref(false)
const slides = ref(null)
const onSlidesFocusOut = (e) => { focused.value = slides.value.contains(e.relatedTarget) }
const onScreen = ref(false)
const lineup = ref(null)
function next() { slide.value = (slide.value + 1) % D.LINEUP.length }
let io = null
onMounted(() => {
  io = new IntersectionObserver(([e]) => { onScreen.value = e.isIntersecting }, { threshold: 0.35 })
  io.observe(lineup.value)
})
onBeforeUnmount(() => io && io.disconnect())

// The live demo opens in a window drawn like the installed PWA. Its title bar
// takes the dashboard's own header colour, so it follows whatever theme the demo wears.
const demoOpen = ref(false)
// The frame wears the visitor's own window controls. Browsers don't say which
// Linux desktop is running, so every Linux visitor gets GNOME's Adwaita.
const os = ref('windows')
onMounted(() => {
  const p = (navigator.userAgentData?.platform || navigator.platform || '').toLowerCase()
  if (p.includes('mac')) os.value = 'mac'
  else if (p.includes('linux')) os.value = 'linux'
})
const demoBar = ref({})
let demoObserver = null
function onDemoLoad(e) {
  const doc = e.target.contentDocument
  if (!doc) return
  const sync = () => {
    const rail = doc.querySelector('aside')
    if (!rail) return
    // The demo keeps its own theme, so the bar takes its text colour as well as its ground.
    const { backgroundColor, color } = getComputedStyle(rail)
    demoBar.value = { background: backgroundColor, color, '--ink-2': color, '--muted': `color-mix(in srgb, ${color} 60%, transparent)` }
  }
  sync()
  // Keys pressed inside the demo stay in its document, so Escape is heard there too.
  doc.addEventListener('keydown', (k) => { if (k.key === 'Escape') demoOpen.value = false })
  demoObserver = new MutationObserver(sync)
  demoObserver.observe(doc.documentElement, { attributes: true, attributeFilter: ['class', 'style', 'data-theme'] })
  demoObserver.observe(doc.body, { childList: true })
}
// A lineup screenshot is cropped to its panel; clicking it shows the whole capture.
const zoom = ref('')
// Both overlays are modal: focus moves in on open, Tab stays inside, and focus
// goes back to whatever opened them on close.
const overlay = ref(null)
let opener = null
function keepTabInside(e) {
  const el = overlay.value
  const items = [...el.querySelectorAll('button, [href], iframe')]
  if (!items.length) { e.preventDefault(); el.focus(); return }
  const first = items[0], last = items[items.length - 1]
  if (e.shiftKey && document.activeElement === first) { e.preventDefault(); last.focus() }
  else if (!e.shiftKey && document.activeElement === last) { e.preventDefault(); first.focus() }
}
const onDemoKey = (e) => {
  if (e.key === 'Escape') { demoOpen.value = false; zoom.value = '' }
  else if (e.key === 'Tab' && overlay.value) keepTabInside(e)
}
// Tabbing out of the demo's frame skips the handler above, so focus that lands
// behind the overlay is sent back to it.
const onFocusIn = (e) => {
  const el = overlay.value
  if (el && !el.contains(e.target)) (el.querySelector('button') || el).focus()
}
watch(() => demoOpen.value || Boolean(zoom.value), async (open) => {
  document.documentElement.style.overflow = open ? 'hidden' : ''
  if (open) {
    opener = document.activeElement
    document.addEventListener('keydown', onDemoKey)
    document.addEventListener('focusin', onFocusIn)
    await nextTick()
    const el = overlay.value
    ;(el.querySelector('button') || el).focus()
    return
  }
  document.removeEventListener('keydown', onDemoKey)
  document.removeEventListener('focusin', onFocusIn)
  if (demoObserver) { demoObserver.disconnect(); demoObserver = null }
  if (opener) { opener.focus(); opener = null }
})
onBeforeUnmount(() => { demoOpen.value = false; document.documentElement.style.overflow = ''; document.removeEventListener('keydown', onDemoKey); document.removeEventListener('focusin', onFocusIn) })
</script>

<template>
  <div class="lerd-landing">

    <SiteHeader />

    <main id="top">
      <section class="hero"><div class="wrap">
        <div class="label"><b>00</b> / {{ t.hero.label }}</div>
        <h1>Lerd <span>{{ t.hero.h1 }}</span></h1>
        <p>{{ t.hero.p }}</p>
        <div class="hero-row">
          <div class="cmd"><i>$</i><code>{{ D.INSTALL }}</code><button class="copy" type="button" @click="copyInstall">{{ copied ? t.hero.copied : t.hero.copy }}</button></div>
          <button class="btn primary" type="button" @click="demoOpen = true">{{ t.hero.demo }}</button>
          <a class="btn" :href="D.WINDOWS_INSTALLER"><svg viewBox="0 0 24 24" width="14" height="14" fill="currentColor" aria-hidden="true"><path d="M0 3.4 9.8 2v9.5H0zm10.9-1.6L24 0v11.4H10.9zM0 12.6h9.8V22L0 20.6zm10.9 0H24V24l-13.1-1.9z"/></svg>Windows</a>
          <a class="btn" :href="withBase('/getting-started/requirements')">{{ t.hero.docs }}</a>
        </div>
        <div class="meta-row">
        <div class="meta">
          <div><strong>7.4 – 8.6</strong>{{ t.hero.php }}</div>
          <div v-if="counts.services"><strong>{{ counts.services }}</strong>{{ t.hero.services }}</div>
          <div v-if="counts.frameworks"><strong>{{ counts.frameworks }}</strong>{{ t.hero.frameworks }}</div>
          <a v-if="counts.plugins" href="#plugins"><strong>{{ counts.plugins }}</strong>{{ t.hero.plugins }}</a>
        </div>
        <a class="sponsor" href="https://www.jetbrains.com/phpstorm/" target="_blank" rel="noopener">
          <span class="label">{{ t.hero.sponsored }}</span>
          <img class="on-light" :src="withBase('/assets/sponsors/phpstorm.svg')" alt="JetBrains PhpStorm" width="125" height="36">
          <img class="on-dark" :src="withBase('/assets/sponsors/phpstorm-dark.svg')" alt="JetBrains PhpStorm" width="125" height="36">
        </a>
        </div>
      </div>
        <div class="hero-art"><div class="win"><div class="win-bar"><i></i><i></i><i></i></div><img class="on-light" :src="withBase('/assets/landing/dashboard-light.png')" :srcset="shotSet({ shot: 'landing/dashboard' }, true)" sizes="min(1000px, 62vw)" width="1280" height="800" :alt="t.hero.dashAlt" loading="lazy"><img class="on-dark" :src="withBase('/assets/landing/dashboard.png')" :srcset="shotSet({ shot: 'landing/dashboard' }, false)" sizes="min(1000px, 62vw)" width="1280" height="800" :alt="t.hero.dashAlt" loading="lazy"></div></div>
      </section>

      <section class="sec" id="lineup" ref="lineup"><div class="wrap">
        <div class="sec-head">
          <div class="label"><b>01–{{ pad(D.LINEUP.length) }}</b> / {{ t.lineup.label }}</div>
          <h2>{{ t.lineup.h[0] }} <span>{{ t.lineup.h[1] }}</span></h2>
        </div>
        <div ref="slides" class="slides" :class="{ paused: hovered || focused || !onScreen }" @mouseenter="hovered = true" @mouseleave="hovered = false" @focusin="focused = true" @focusout="onSlidesFocusOut">
          <div class="index" role="tablist" :aria-label="t.lineup.tabs">
            <button v-for="(c, i) in D.LINEUP" :key="c.id" type="button" role="tab" :aria-selected="i === slide" :class="{ on: i === slide }" @click="slide = i">
              <span class="n">{{ pad(i + 1) }}</span><span class="t">{{ t.lineup.items[c.id].title }}</span>
              <span v-if="i === slide" class="bar" @animationend="next"></span>
            </button>
          </div>
          <div class="cards">
          <div v-for="(c, i) in D.LINEUP" :key="c.id" class="card" :class="{ on: i === slide }" role="tabpanel" :aria-hidden="i !== slide" :inert="i !== slide">
            <div class="card-body">
              <div class="card-top"><span class="num">{{ pad(i + 1) }} / {{ t.lineup.items[c.id].title }}</span><span class="badge ok">{{ c.isNew ? t.lineup.new : t.lineup.stable }}</span></div>
              <h3>{{ t.lineup.items[c.id].h }}</h3>
              <p>{{ t.lineup.items[c.id].p }}</p>
              <div class="badges">
                <span v-for="tag in t.lineup.items[c.id].tags" :key="tag" class="badge">{{ tag }}</span>
              </div>
              <p v-if="c.names" class="names"><span v-for="(n, i) in c.names" :key="n"><i v-if="i">·</i>{{ n }}</span></p>
              <div class="cli"><i>$</i> {{ c.cli }}</div>
              <a class="explore" :href="withBase(c.href)">{{ t.lineup.items[c.id].more }} →</a>
            </div>
            <button class="shot" type="button" :aria-label="fill(t.lineup.enlarge, { title: t.lineup.items[c.id].title })" @click="zoom = shot(c, !isDark)"><img class="on-light" :src="shot(c, true)" :srcset="shotSet(c, true)" :sizes="SHOT_SIZES" width="1280" height="800" :alt="t.lineup.items[c.id].alt" loading="lazy"><img class="on-dark" :src="shot(c, false)" :srcset="shotSet(c, false)" :sizes="SHOT_SIZES" width="1280" height="800" :alt="t.lineup.items[c.id].alt" loading="lazy"></button>
          </div>
          </div>
        </div>
      </div></section>

      <section class="sec" id="agents"><div class="wrap">
        <div class="sec-head">
          <div class="label"><b>{{ pad(D.LINEUP.length + 1) }}</b> / {{ t.agents.label }}</div>
          <div>
            <h2>{{ t.agents.h[0] }} <span>{{ t.agents.h[1] }}</span></h2>
            <p class="sec-lead">{{ t.agents.lead }}</p>
            <div class="cli agents-cli"><i>$</i> lerd mcp:enable-global</div>
          </div>
        </div>
        <div class="fw-grid rt-grid ag-grid">
          <div v-for="(a, i) in t.agents.items" :key="a.name" class="fw">
            <span class="n">{{ pad(i + 1) }}</span>
            <h3 class="fw-name">{{ a.name }}</h3>
            <code class="rt-cmd">{{ D.AGENT_CMDS[i] }}</code>
            <p class="rt-p">{{ a.p }}</p>
          </div>
        </div>
        <div class="more">
          <div class="more-row"><span class="label">{{ t.agents.worksWith }}</span><p><span v-for="(n, i) in D.ASSISTANTS" :key="n"><i v-if="i">·</i>{{ n }}</span></p></div>
        </div>
        <div class="plug-links">
          <a class="btn primary" :href="withBase('/features/mcp')">{{ t.agents.setup }}</a>
        </div>
      </div></section>

      <section class="sec" id="frameworks"><div class="wrap">
        <div class="sec-head">
          <div class="label"><b>{{ pad(D.LINEUP.length + 2) }}</b> / {{ t.frameworks.label }}</div>
          <h2>{{ t.frameworks.h[0] }} <span>{{ t.frameworks.h[1] }}</span></h2>
        </div>
        <div class="fw-grid">
          <a v-for="(f, i) in D.FRAMEWORKS" :key="f.logo" class="fw" :href="withBase(`/getting-started/${f.page}`)">
            <span class="n">{{ pad(i + 1) }}</span>
            <span class="fw-gl" v-html="D.glyph(f.logo, undefined, { bare: true })"></span>
            <span class="fw-name">{{ f.name }}</span>
          </a>
          <a class="fw fw-own" :href="withBase('/usage/framework-definitions')"><span class="n">+</span><span class="fw-gl fw-plus">+</span><span class="fw-name">{{ t.frameworks.yours }}</span></a>
        </div>
      </div></section>

      <section class="sec" id="runtimes"><div class="wrap">
        <div class="sec-head">
          <div class="label"><b>{{ pad(D.LINEUP.length + 3) }}</b> / {{ t.runtimes.label }}</div>
          <div>
            <h2>{{ t.runtimes.h[0] }} <span>{{ t.runtimes.h[1] }}</span></h2>
            <p class="sec-lead">{{ t.runtimes.lead }} <a :href="withBase('/usage/host-proxy')">{{ t.runtimes.guide }}</a></p>
          </div>
        </div>
        <div class="fw-grid rt-grid">
          <div v-for="(r, i) in t.runtimes.items" :key="r.name" class="fw">
            <span class="n">{{ pad(i + 1) }}</span>
            <h3 class="fw-name">{{ r.name }}</h3>
            <code class="rt-cmd">{{ D.RUNTIME_CMDS[i] }}</code>
            <p class="rt-p">{{ r.p }}</p>
          </div>
        </div>
      </div></section>

      <section class="sec" id="services"><div class="wrap">
        <div class="sec-head">
          <div class="label"><b>{{ pad(D.LINEUP.length + 4) }}</b> / {{ t.services.label }}</div>
          <h2>{{ t.services.h[0] }} <span>{{ t.services.h[1] }}</span></h2>
        </div>
        <div class="fw-grid">
          <div v-for="(s, i) in D.SVC_SHOW" :key="s.logo" class="fw">
            <span class="n">{{ pad(i + 1) }}</span>
            <span class="fw-gl" v-html="D.glyph(s.logo, undefined, { bare: true })"></span>
            <span class="fw-name">{{ s.name }}<span class="fw-port">{{ s.port }}</span></span>
          </div>
          <a class="fw fw-own" :href="withBase('/usage/custom-services')"><span class="n">+</span><span class="fw-gl fw-plus">+</span><span class="fw-name">{{ t.frameworks.yours }}</span></a>
        </div>
        <div class="more">
          <div class="more-row"><span class="label">{{ fill(t.services.more, { n: D.SVC_MORE.length }) }}</span><p><span v-for="(n, i) in D.SVC_MORE" :key="n"><i v-if="i">·</i>{{ n }}</span></p></div>
          <div class="more-row"><span class="label">{{ t.services.admin }}</span><p><span v-for="(n, i) in D.SVC_ADMIN" :key="n"><i v-if="i">·</i>{{ n }}</span></p></div>
        </div>
      </div></section>

      <section class="sec" id="plugins"><div class="wrap">
        <div class="sec-head">
          <div class="label"><b>{{ pad(D.LINEUP.length + 5) }}</b> / {{ t.plugins.label }}</div>
          <div>
            <h2>{{ t.plugins.h[0] }} <span>{{ t.plugins.h[1] }}</span></h2>
            <p class="sec-lead">{{ t.plugins.lead }}</p>
            <div class="plug-links">
              <a class="btn primary" :href="PLUGINS" target="_blank" rel="noopener">{{ t.plugins.browse }}<span v-if="counts.plugins" class="stars">{{ counts.plugins }}</span></a>
              <a class="btn" :href="withBase('/features/plugins')">{{ t.plugins.create }}</a>
            </div>
          </div>
        </div>
      </div></section>

      <section class="sec" id="platforms"><div class="wrap">
        <div class="sec-head">
          <div class="label"><b>{{ pad(D.LINEUP.length + 6) }}</b> / {{ t.platforms.label }}</div>
          <h2>{{ t.platforms.h[0] }} <span>{{ t.platforms.h[1] }}</span></h2>
        </div>
        <div class="grid6 plat">
          <div v-for="(pl, i) in D.PLATFORMS" :key="pl.name" class="p">
            <div class="card-top"><span class="n">{{ pad(i + 1) }}</span><span class="badge ok">{{ pl.status || t.platforms.stable }}</span></div>
            <h3>{{ pl.name }}</h3>
            <p>{{ t.platforms.items[i] }}</p>
            <a class="explore" :href="withBase(pl.href)">{{ fill(t.platforms.guide, { name: pl.name }) }}</a>
          </div>
        </div>
      </div></section>

      <section class="sec" id="principles"><div class="wrap">
        <div class="sec-head">
          <div class="label"><b>{{ pad(D.LINEUP.length + 7) }}</b> / {{ t.principles.label }}</div>
          <h2>{{ t.principles.h[0] }} <span>{{ t.principles.h[1] }}</span></h2>
        </div>
        <div class="grid6">
          <div v-for="(p, i) in t.principles.items" :key="p.h" class="p"><span class="n">{{ pad(i + 1) }}</span><h3>{{ p.h }}</h3><p>{{ p.p }}</p></div>
        </div>
      </div></section>

      <section class="sec" id="compare"><div class="wrap">
        <div class="sec-head">
          <div class="label"><b>{{ pad(D.LINEUP.length + 8) }}</b> / {{ t.compare.label }}</div>
          <h2>{{ t.compare.h[0] }} <span>{{ t.compare.h[1] }}</span></h2>
        </div>
        <div class="cmp-wrap">
          <table class="cmp">
            <thead><tr><th></th><th v-for="(c, i) in D.CMP.cols" :key="c" scope="col" :class="{ lerd: i === 0 }">{{ c }}</th></tr></thead>
            <tbody>
              <tr v-for="(r, ri) in D.CMP.rows" :key="ri">
                <th scope="row">{{ t.compare.rows[ri] }}</th>
                <td v-for="(v, i) in r" :key="i" :class="[v, { lerd: i === 0 }]" :aria-label="t.compare[v]">{{ v === 'yes' ? '✓' : v === 'no' ? '✕' : '–' }}</td>
              </tr>
            </tbody>
          </table>
        </div>
        <p class="cmp-note"><template v-for="(part, pi) in parts(t.compare.note)" :key="pi"><a v-if="part.slot" :href="withBase(CMP_LINKS[part.slot])">{{ t.compare[part.slot] }}</a><template v-else>{{ part.text }}</template></template></p>
      </div></section>

      <section class="sec" id="pricing"><div class="wrap">
        <div class="sec-head">
          <div class="label"><b>{{ pad(D.LINEUP.length + 9) }}</b> / {{ t.pricing.label }}</div>
          <h2>{{ t.pricing.h[0] }} <span>{{ t.pricing.h[1] }}</span></h2>
        </div>
        <div class="grid6 price">
          <div class="p">
            <div class="card-top"><span class="n">01</span><span class="badge ok">{{ t.pricing.free.badge }}</span></div>
            <h3>{{ t.pricing.free.h }} <span class="price-tag">{{ t.pricing.free.tag }}</span></h3>
            <p>{{ t.pricing.free.p }}</p>
            <a class="explore" href="#start">{{ t.pricing.free.cta }}</a>
          </div>
          <div class="p">
            <div class="card-top"><span class="n">02</span><span class="badge">{{ t.pricing.sponsor.badge }}</span></div>
            <h3>{{ t.pricing.sponsor.h }}</h3>
            <p>{{ t.pricing.sponsor.p }}</p>
            <a class="explore" href="https://github.com/sponsors/geodro" target="_blank" rel="noopener">{{ t.pricing.sponsor.cta }}</a>
          </div>
          <div class="p">
            <div class="card-top"><span class="n">03</span><span class="badge">{{ t.pricing.support.badge }}</span></div>
            <h3>{{ t.pricing.support.h }}</h3>
            <p>{{ t.pricing.support.p }}</p>
            <a class="explore" href="mailto:george@dumitres.co">{{ t.pricing.support.cta }}</a>
          </div>
        </div>
      </div></section>

      <section class="sec" id="faq"><div class="wrap">
        <div class="sec-head">
          <div class="label"><b>{{ pad(D.LINEUP.length + 10) }}</b> / {{ t.faq.label }}</div>
          <h2>{{ t.faq.h[0] }} <span>{{ t.faq.h[1] }}</span></h2>
        </div>
        <div class="faq">
          <details v-for="(f, i) in t.faq.items" :key="f.q">
            <summary>{{ f.q }}</summary>
            <p>{{ f.a }} <a v-if="D.FAQ_LINKS[i]" :href="D.FAQ_LINKS[i].startsWith('http') ? D.FAQ_LINKS[i] : withBase(D.FAQ_LINKS[i])" v-bind="D.FAQ_LINKS[i].startsWith('http') ? { target: '_blank', rel: 'noopener' } : {}">{{ f.link }} →</a></p>
          </details>
        </div>
      </div></section>

      <section class="cta" id="start"><div class="wrap">
        <div class="label"><b>{{ pad(D.LINEUP.length + 11) }}</b> / {{ t.start.label }}</div>
        <h2>{{ t.start.h }}</h2>
        <p>{{ t.start.p }}</p>
        <div class="hero-row">
          <div class="cmd"><i>$</i><code>{{ D.INSTALL }}</code><button class="copy" type="button" @click="copyInstall">{{ copied ? t.hero.copied : t.hero.copy }}</button></div>
          <a class="btn" :href="D.WINDOWS_INSTALLER"><svg viewBox="0 0 24 24" width="14" height="14" fill="currentColor" aria-hidden="true"><path d="M0 3.4 9.8 2v9.5H0zm10.9-1.6L24 0v11.4H10.9zM0 12.6h9.8V22L0 20.6zm10.9 0H24V24l-13.1-1.9z"/></svg>Windows</a>
          <a class="btn" :href="GITHUB" target="_blank" rel="noopener">{{ t.start.star }}<span v-if="stars" class="stars">{{ starLabel(stars) }}</span></a>
        </div>
      </div></section>
      <!-- The digest itself is English and changes every release, so only the English page points to it. -->
      <div v-if="lang === 'en-US'" class="wrap">
        <a class="digest" :href="withBase('/digest/v1.38.0.html')" target="_blank" rel="noopener">
          <span class="badge ok">NEW</span>
          <span class="digest-body">
            <span class="digest-title">v1.38.0 dev digest</span>
            <span class="digest-sub">Lerd on Windows from one installer, plugins from a catalog that approves each one at a commit, certificates issued without mkcert, and a PHP language server for AI agents.</span>
          </span>
          <span class="digest-cta">Read the digest →</span>
        </a>
      </div>
    </main>

    <SiteFooter />

    <div v-if="zoom" ref="overlay" class="demo zoom" role="dialog" aria-modal="true" :aria-label="t.demo.screenshot" tabindex="-1" @click="zoom = ''">
      <img :src="zoom" alt="">
    </div>

    <div v-if="demoOpen" ref="overlay" class="demo" @click.self="demoOpen = false">
      <div class="pwa" :class="`os-${os}`" role="dialog" aria-modal="true" :aria-label="t.demo.dialog">
        <div class="pwa-bar" :style="demoBar">
          <span v-if="os === 'mac'" class="mac-lights"><button class="mac-close" type="button" :aria-label="t.demo.close" @click="demoOpen = false"></button><i></i><i></i></span>
          <img v-if="os === 'windows'" :src="withBase('/assets/logo.svg')" alt="" width="16" height="16">
          <span class="pwa-head"><span class="pwa-title">Lerd</span><span class="pwa-note">{{ t.demo.title }}</span></span>
          <template v-if="os === 'windows'">
            <span class="pwa-ctl" aria-hidden="true"><i class="min"></i><i class="max"></i></span>
            <button class="pwa-close" type="button" :aria-label="t.demo.close" @click="demoOpen = false">✕</button>
          </template>
          <button v-if="os === 'linux'" class="adw-close" type="button" :aria-label="t.demo.close" @click="demoOpen = false"><svg viewBox="0 0 16 16" width="16" height="16" aria-hidden="true"><path d="M4.5 4.5l7 7m0-7l-7 7" stroke="currentColor" stroke-width="1.6" stroke-linecap="round"/></svg></button>
        </div>
        <iframe :src="withBase(`/demo/index.html?theme=${isDark ? 'dark' : 'light'}`)" :title="t.demo.frame" @load="onDemoLoad"></iframe>
      </div>
    </div>
  </div>
</template>
