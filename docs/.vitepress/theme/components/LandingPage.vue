<script setup>
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useData, withBase } from 'vitepress'
import * as D from '../landing-data.js'
import { data as starData } from '../stars.data'
import { data as storeData } from '../store.data'

const GITHUB = 'https://github.com/lerd-env/lerd'
const DISCORD = 'https://discord.gg/5JK54s7xCC'
const pad = (n) => String(n).padStart(2, '0')
const { isDark } = useData()
// Each lineup capture has a -light twin taken from the demo in the light theme.
const shot = (c, light) => withBase(`/assets/${c.shot}${light ? '-light' : ''}.png`)
// The cards show the capture at most ~620px wide, so they load WebP copies; the zoom keeps the PNG.
const shotSet = (c, light) => [640, 1280].map((w) => `${withBase(`/assets/${c.shot}${light ? '-light' : ''}-${w}.webp`)} ${w}w`).join(', ')
const SHOT_SIZES = '(max-width: 760px) calc(100vw - 56px), 620px'

const stars = starData.stars
const starLabel = (n) => (n >= 1000 ? (n / 1000).toFixed(1).replace(/\.0$/, '') + 'k' : String(n))

const PLUGINS = 'https://plugins.lerd.sh/'
const counts = ref({ frameworks: storeData.frameworks, services: storeData.services, plugins: storeData.plugins })
onMounted(async () => {
  const live = await D.liveCounts(storeData.builtin)
  for (const key of Object.keys(live)) if (live[key]) counts.value[key] = live[key]
})

// Below 960px the nav folds behind a menu button.
const menuOpen = ref(false)
const menuBtn = ref(null)
// Escape hides the link that had focus, so focus goes back to the button.
function closeMenu() {
  if (!menuOpen.value) return
  menuOpen.value = false
  menuBtn.value.focus()
}

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

    <header class="hdr" :class="{ open: menuOpen }" @keydown.esc="closeMenu"><div class="wrap">
      <a class="brand" href="#top"><img :src="withBase('/assets/logo.svg')" alt="" width="26" height="26">Lerd</a>
      <nav id="hdr-nav" @click="menuOpen = false">
        <a href="#lineup">Features</a>
        <a href="#agents">AI agents</a>
        <a href="#frameworks">Frameworks</a>
        <a href="#services">Services</a>
        <a href="#plugins">Plugins</a>
        <a href="#principles">Why Lerd</a>
        <a href="#compare">Compare</a>
        <a href="#faq">FAQ</a>
        <a :href="withBase('/getting-started/requirements')">Docs</a>
      </nav>
      <div class="hdr-right">
        <a class="gh" :href="GITHUB" target="_blank" rel="noopener">GitHub<span v-if="stars" class="stars">★ {{ starLabel(stars) }}</span></a>
        <a class="discord" :href="DISCORD" target="_blank" rel="noopener" aria-label="Discord"><svg viewBox="0 0 24 24" fill="currentColor"><path d="M20.317 4.3698a19.7913 19.7913 0 00-4.8851-1.5152.0741.0741 0 00-.0785.0371c-.211.3753-.4447.8648-.6083 1.2495-1.8447-.2762-3.68-.2762-5.4868 0-.1636-.3933-.4058-.8742-.6177-1.2495a.077.077 0 00-.0785-.037 19.7363 19.7363 0 00-4.8852 1.515.0699.0699 0 00-.0321.0277C.5334 9.0458-.319 13.5799.0992 18.0578a.0824.0824 0 00.0312.0561c2.0528 1.5076 4.0413 2.4228 5.9929 3.0294a.0777.0777 0 00.0842-.0276c.4616-.6304.8731-1.2952 1.226-1.9942a.076.076 0 00-.0416-.1057c-.6528-.2476-1.2743-.5495-1.8722-.8923a.077.077 0 01-.0076-.1277c.1258-.0943.2517-.1923.3718-.2914a.0743.0743 0 01.0776-.0105c3.9278 1.7933 8.18 1.7933 12.0614 0a.0739.0739 0 01.0785.0095c.1202.099.246.1981.3728.2924a.077.077 0 01-.0066.1276 12.2986 12.2986 0 01-1.873.8914.0766.0766 0 00-.0407.1067c.3604.698.7719 1.3628 1.225 1.9932a.076.076 0 00.0842.0286c1.961-.6067 3.9495-1.5219 6.0023-3.0294a.077.077 0 00.0313-.0552c.5004-5.177-.8382-9.6739-3.5485-13.6604a.061.061 0 00-.0312-.0286zM8.02 15.3312c-1.1825 0-2.1569-1.0857-2.1569-2.419 0-1.3332.9555-2.4189 2.157-2.4189 1.2108 0 2.1757 1.0952 2.1568 2.419 0 1.3332-.9555 2.4189-2.1569 2.4189zm7.9748 0c-1.1825 0-2.1569-1.0857-2.1569-2.419 0-1.3332.9554-2.4189 2.1569-2.4189 1.2108 0 2.1757 1.0952 2.1568 2.419 0 1.3332-.946 2.4189-2.1568 2.4189Z"/></svg></a>
        <button class="theme" type="button" :aria-label="isDark ? 'Switch to light theme' : 'Switch to dark theme'" @click="isDark = !isDark">
          <svg class="on-light" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8z"/></svg>
          <svg class="on-dark" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"/></svg>
        </button>
        <a class="btn ink" href="#start">Install</a>
        <button ref="menuBtn" class="menu" type="button" aria-controls="hdr-nav" :aria-expanded="menuOpen" :aria-label="menuOpen ? 'Close menu' : 'Open menu'" @click="menuOpen = !menuOpen">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><path v-if="menuOpen" d="M6 6l12 12M18 6L6 18"/><path v-else d="M4 7h16M4 12h16M4 17h16"/></svg>
        </button>
      </div>
    </div></header>

    <main id="top">
      <section class="hero"><div class="wrap">
        <div class="label"><b>00</b> / Open source, rootless, no Docker</div>
        <h1>Lerd <span>Local PHP development for Linux, macOS and Windows</span></h1>
        <p>Lerd gives each project a .test domain with HTTPS, its own PHP and Node versions, and the databases and services it needs, all running as rootless Podman containers. It's free and open source, with no Docker daemon.</p>
        <div class="hero-row">
          <div class="cmd"><i>$</i><code>{{ D.INSTALL }}</code><button class="copy" type="button" @click="copyInstall">{{ copied ? 'COPIED' : 'COPY' }}</button></div>
          <button class="btn primary" type="button" @click="demoOpen = true">Try the live demo</button>
          <a class="btn" :href="D.WINDOWS_INSTALLER"><svg viewBox="0 0 24 24" width="14" height="14" fill="currentColor" aria-hidden="true"><path d="M0 3.4 9.8 2v9.5H0zm10.9-1.6L24 0v11.4H10.9zM0 12.6h9.8V22L0 20.6zm10.9 0H24V24l-13.1-1.9z"/></svg>Windows</a>
          <a class="btn" :href="withBase('/getting-started/requirements')">Read the docs →</a>
        </div>
        <div class="meta-row">
        <div class="meta">
          <div><strong>7.4 – 8.6</strong>PHP versions, 8.6 in preview</div>
          <div v-if="counts.services"><strong>{{ counts.services }}</strong>service presets</div>
          <div v-if="counts.frameworks"><strong>{{ counts.frameworks }}</strong>framework definitions</div>
          <a v-if="counts.plugins" href="#plugins"><strong>{{ counts.plugins }}</strong>community plugins</a>
        </div>
        <a class="sponsor" href="https://www.jetbrains.com/phpstorm/" target="_blank" rel="noopener">
          <span class="label">Sponsored by</span>
          <img class="on-light" :src="withBase('/assets/sponsors/phpstorm.svg')" alt="JetBrains PhpStorm" width="125" height="36">
          <img class="on-dark" :src="withBase('/assets/sponsors/phpstorm-dark.svg')" alt="JetBrains PhpStorm" width="125" height="36">
        </a>
        </div>
      </div>
        <div class="hero-art"><div class="win"><div class="win-bar"><i></i><i></i><i></i></div><img class="on-light" :src="withBase('/assets/landing/dashboard-light.png')" :srcset="shotSet({ shot: 'landing/dashboard' }, true)" sizes="min(1000px, 62vw)" width="1280" height="800" alt="Lerd web dashboard with sites, services and workers" loading="lazy"><img class="on-dark" :src="withBase('/assets/landing/dashboard.png')" :srcset="shotSet({ shot: 'landing/dashboard' }, false)" sizes="min(1000px, 62vw)" width="1280" height="800" alt="Lerd web dashboard with sites, services and workers" loading="lazy"></div></div>
      </section>

      <section class="sec" id="lineup" ref="lineup"><div class="wrap">
        <div class="sec-head">
          <div class="label"><b>01–{{ pad(D.LINEUP.length) }}</b> / The lineup</div>
          <h2>What you get <span>after one install.</span></h2>
        </div>
        <div ref="slides" class="slides" :class="{ paused: hovered || focused || !onScreen }" @mouseenter="hovered = true" @mouseleave="hovered = false" @focusin="focused = true" @focusout="onSlidesFocusOut">
          <div class="index" role="tablist" aria-label="Features">
            <button v-for="(c, i) in D.LINEUP" :key="c.id" type="button" role="tab" :aria-selected="i === slide" :class="{ on: i === slide }" @click="slide = i">
              <span class="n">{{ pad(i + 1) }}</span><span class="t">{{ c.title }}</span>
              <span v-if="i === slide" class="bar" @animationend="next"></span>
            </button>
          </div>
          <div class="cards">
          <div v-for="(c, i) in D.LINEUP" :key="c.id" class="card" :class="{ on: i === slide }" role="tabpanel" :aria-hidden="i !== slide" :inert="i !== slide">
            <div class="card-body">
              <div class="card-top"><span class="num">{{ pad(i + 1) }} / {{ c.title }}</span><span class="badge ok">{{ c.status || 'STABLE' }}</span></div>
              <h3>{{ c.h }}</h3>
              <p>{{ c.p }}</p>
              <div class="badges">
                <span v-for="t in c.tags" :key="t.t || t" class="badge" :class="{ beta: t.beta }">{{ t.t || t }}</span>
              </div>
              <p v-if="c.names" class="names"><span v-for="(n, i) in c.names" :key="n"><i v-if="i">·</i>{{ n }}</span></p>
              <div class="cli"><i>$</i> {{ c.cli }}</div>
              <a class="explore" :href="withBase(c.href)">{{ c.more }} →</a>
            </div>
            <button class="shot" type="button" :aria-label="`Enlarge the ${c.title} screenshot`" @click="zoom = shot(c, !isDark)"><img class="on-light" :src="shot(c, true)" :srcset="shotSet(c, true)" :sizes="SHOT_SIZES" width="1280" height="800" :alt="c.alt" loading="lazy"><img class="on-dark" :src="shot(c, false)" :srcset="shotSet(c, false)" :sizes="SHOT_SIZES" width="1280" height="800" :alt="c.alt" loading="lazy"></button>
          </div>
          </div>
        </div>
      </div></section>

      <section class="sec" id="agents"><div class="wrap">
        <div class="sec-head">
          <div class="label"><b>{{ pad(D.LINEUP.length + 1) }}</b> / AI agents</div>
          <div>
            <h2>An MCP server for Claude Code, <span>Cursor and Codex.</span></h2>
            <p class="sec-lead">Lerd comes with an MCP server, so the assistant you code with can manage your environment as well: link sites, change PHP versions, start services and workers, run artisan and composer, and look at what a request did. One command registers it with every assistant on the list below.</p>
            <div class="cli agents-cli"><i>$</i> lerd mcp:enable-global</div>
          </div>
        </div>
        <div class="fw-grid rt-grid ag-grid">
          <div v-for="(a, i) in D.AGENTS" :key="a.name" class="fw">
            <span class="n">{{ pad(i + 1) }}</span>
            <h3 class="fw-name">{{ a.name }}</h3>
            <code class="rt-cmd">{{ a.cmd }}</code>
            <p class="rt-p">{{ a.p }}</p>
          </div>
        </div>
        <div class="more">
          <div class="more-row"><span class="label">Works with</span><p><span v-for="(n, i) in D.ASSISTANTS" :key="n"><i v-if="i">·</i>{{ n }}</span></p></div>
        </div>
        <div class="plug-links">
          <a class="btn primary" :href="withBase('/features/mcp')">Set up your assistant →</a>
        </div>
      </div></section>

      <section class="sec" id="frameworks"><div class="wrap">
        <div class="sec-head">
          <div class="label"><b>{{ pad(D.LINEUP.length + 2) }}</b> / Frameworks</div>
          <h2>Frameworks Lerd <span>already knows.</span></h2>
        </div>
        <div class="fw-grid">
          <a v-for="(f, i) in D.FRAMEWORKS" :key="f.logo" class="fw" :href="withBase(`/getting-started/${f.page}`)">
            <span class="n">{{ pad(i + 1) }}</span>
            <span class="fw-gl" v-html="D.glyph(f.logo, undefined, { bare: true })"></span>
            <span class="fw-name">{{ f.name }}</span>
          </a>
          <a class="fw fw-own" :href="withBase('/usage/framework-definitions')"><span class="n">+</span><span class="fw-gl fw-plus">+</span><span class="fw-name">Yours, as YAML →</span></a>
        </div>
      </div></section>

      <section class="sec" id="runtimes"><div class="wrap">
        <div class="sec-head">
          <div class="label"><b>{{ pad(D.LINEUP.length + 3) }}</b> / Beyond PHP</div>
          <div>
            <h2>Your Node, Python and Rails apps, <span>on the same .test domains.</span></h2>
            <p class="sec-lead">lerd init recognises the project, picks its dev server and a free port, and runs it on the host, so file watching and hot reload behave as usual. Lerd restarts it if it dies, keeps its logs and puts nginx with TLS in front. Databases and services go into its .env like on any PHP site. <a :href="withBase('/usage/host-proxy')">Non-PHP apps guide →</a></p>
          </div>
        </div>
        <div class="fw-grid rt-grid">
          <div v-for="(r, i) in D.RUNTIMES" :key="r.name" class="fw">
            <span class="n">{{ pad(i + 1) }}</span>
            <h3 class="fw-name">{{ r.name }}</h3>
            <code class="rt-cmd">{{ r.cmd }}</code>
            <p class="rt-p">{{ r.p }}</p>
          </div>
        </div>
      </div></section>

      <section class="sec" id="services"><div class="wrap">
        <div class="sec-head">
          <div class="label"><b>{{ pad(D.LINEUP.length + 4) }}</b> / Services</div>
          <h2>Databases, caches, search <span>and mail.</span></h2>
        </div>
        <div class="fw-grid">
          <div v-for="(s, i) in D.SVC_SHOW" :key="s.logo" class="fw">
            <span class="n">{{ pad(i + 1) }}</span>
            <span class="fw-gl" v-html="D.glyph(s.logo, undefined, { bare: true })"></span>
            <span class="fw-name">{{ s.name }}<span class="fw-port">{{ s.port }}</span></span>
          </div>
          <a class="fw fw-own" :href="withBase('/usage/custom-services')"><span class="n">+</span><span class="fw-gl fw-plus">+</span><span class="fw-name">Yours, as YAML →</span></a>
        </div>
        <div class="more">
          <div class="more-row"><span class="label">+{{ D.SVC_MORE.length }} in the store</span><p><span v-for="(n, i) in D.SVC_MORE" :key="n"><i v-if="i">·</i>{{ n }}</span></p></div>
          <div class="more-row"><span class="label">Admin UIs</span><p><span v-for="(n, i) in D.SVC_ADMIN" :key="n"><i v-if="i">·</i>{{ n }}</span></p></div>
        </div>
      </div></section>

      <section class="sec" id="plugins"><div class="wrap">
        <div class="sec-head">
          <div class="label"><b>{{ pad(D.LINEUP.length + 5) }}</b> / Community plugins</div>
          <div>
            <h2>Community plugins for <span>everything else.</span></h2>
            <p class="sec-lead">A plugin adds what Lerd doesn't do yet: CLI commands, dashboard pages, site tabs, tools for AI assistants and hooks that run when something happens. Write one in any language that runs on your machine. Every plugin in the catalog is reviewed at the exact commit Lerd installs.</p>
            <div class="plug-links">
              <a class="btn primary" :href="PLUGINS" target="_blank" rel="noopener">Browse community plugins<span v-if="counts.plugins" class="stars">{{ counts.plugins }}</span></a>
              <a class="btn" :href="withBase('/features/plugins')">Create a plugin →</a>
            </div>
          </div>
        </div>
      </div></section>

      <section class="sec" id="platforms"><div class="wrap">
        <div class="sec-head">
          <div class="label"><b>{{ pad(D.LINEUP.length + 6) }}</b> / Platforms</div>
          <h2>Linux, macOS <span>and Windows.</span></h2>
        </div>
        <div class="grid6 plat">
          <div v-for="(pl, i) in D.PLATFORMS" :key="pl.name" class="p">
            <div class="card-top"><span class="n">{{ pad(i + 1) }}</span><span class="badge" :class="pl.beta ? 'beta' : 'ok'">{{ pl.status }}</span></div>
            <h3>{{ pl.name }}</h3>
            <p>{{ pl.p }}</p>
            <a class="explore" :href="withBase(pl.href)">{{ pl.name }} install guide →</a>
          </div>
        </div>
      </div></section>

      <section class="sec" id="principles"><div class="wrap">
        <div class="sec-head">
          <div class="label"><b>{{ pad(D.LINEUP.length + 7) }}</b> / Principles</div>
          <h2>How Lerd <span>is built.</span></h2>
        </div>
        <div class="grid6">
          <div v-for="(p, i) in D.PRINCIPLES" :key="p.h" class="p"><span class="n">{{ pad(i + 1) }}</span><h3>{{ p.h }}</h3><p>{{ p.p }}</p></div>
        </div>
      </div></section>

      <section class="sec" id="compare"><div class="wrap">
        <div class="sec-head">
          <div class="label"><b>{{ pad(D.LINEUP.length + 8) }}</b> / Compare</div>
          <h2>Lerd next to <span>Herd, Laragon, DDEV, Lando and Sail.</span></h2>
        </div>
        <div class="cmp-wrap">
          <table class="cmp">
            <thead><tr><th></th><th v-for="(c, i) in D.CMP.cols" :key="c" scope="col" :class="{ lerd: i === 0 }">{{ c }}</th></tr></thead>
            <tbody>
              <tr v-for="r in D.CMP.rows" :key="r.f">
                <th scope="row">{{ r.f }}</th>
                <td v-for="(v, i) in r.v" :key="i" :class="[v, { lerd: i === 0 }]" :aria-label="v">{{ v === 'yes' ? '✓' : v === 'no' ? '✕' : '–' }}</td>
              </tr>
            </tbody>
          </table>
        </div>
        <p class="cmp-note">Coming from another tool? See <a :href="withBase('/getting-started/herd-linux')">Laravel Herd for Linux</a>, <a :href="withBase('/getting-started/laragon-linux')">Laragon for Linux</a> and the <a :href="withBase('/getting-started/comparison')">full comparison</a>.</p>
      </div></section>

      <section class="sec" id="pricing"><div class="wrap">
        <div class="sec-head">
          <div class="label"><b>{{ pad(D.LINEUP.length + 9) }}</b> / Pricing</div>
          <h2>Free for everyone, <span>with support when you need it.</span></h2>
        </div>
        <div class="grid6 price">
          <div class="p">
            <div class="card-top"><span class="n">01</span><span class="badge ok">MIT</span></div>
            <h3>Free <span class="price-tag">forever</span></h3>
            <p>All of Lerd, for personal and commercial work. No paid tier, no account and no feature held back.</p>
            <a class="explore" href="#start">Install Lerd →</a>
          </div>
          <div class="p">
            <div class="card-top"><span class="n">02</span><span class="badge">OPTIONAL</span></div>
            <h3>Sponsor</h3>
            <p>Rely on Lerd? Sponsoring funds the time that keeps it maintained, adds new features and gets fixes out faster.</p>
            <a class="explore" href="https://github.com/sponsors/geodro" target="_blank" rel="noopener">Become a sponsor →</a>
          </div>
          <div class="p">
            <div class="card-top"><span class="n">03</span><span class="badge">FOR TEAMS</span></div>
            <h3>Commercial support</h3>
            <p>Using Lerd across a team? Support covers onboarding, priority fixes and help with your stack.</p>
            <a class="explore" href="mailto:george@dumitres.co">Get in touch →</a>
          </div>
        </div>
      </div></section>

      <section class="sec" id="faq"><div class="wrap">
        <div class="sec-head">
          <div class="label"><b>{{ pad(D.LINEUP.length + 10) }}</b> / FAQ</div>
          <h2>Questions <span>people ask first.</span></h2>
        </div>
        <div class="faq">
          <details v-for="f in D.FAQ" :key="f.q">
            <summary>{{ f.q }}</summary>
            <p>{{ f.a }} <a v-if="f.href" :href="f.href.startsWith('http') ? f.href : withBase(f.href)" v-bind="f.href.startsWith('http') ? { target: '_blank', rel: 'noopener' } : {}">{{ f.link }} →</a></p>
          </details>
        </div>
      </div></section>

      <section class="cta" id="start"><div class="wrap">
        <div class="label"><b>{{ pad(D.LINEUP.length + 11) }}</b> / Get started</div>
        <h2>Install, then run lerd init.</h2>
        <p>The script installs Lerd and sets up DNS and certificates. Then cd into a project, run lerd init and open its .test address.</p>
        <div class="hero-row">
          <div class="cmd"><i>$</i><code>{{ D.INSTALL }}</code><button class="copy" type="button" @click="copyInstall">{{ copied ? 'COPIED' : 'COPY' }}</button></div>
          <a class="btn" :href="D.WINDOWS_INSTALLER"><svg viewBox="0 0 24 24" width="14" height="14" fill="currentColor" aria-hidden="true"><path d="M0 3.4 9.8 2v9.5H0zm10.9-1.6L24 0v11.4H10.9zM0 12.6h9.8V22L0 20.6zm10.9 0H24V24l-13.1-1.9z"/></svg>Windows</a>
          <a class="btn" :href="GITHUB" target="_blank" rel="noopener">Star on GitHub<span v-if="stars" class="stars">{{ starLabel(stars) }}</span></a>
        </div>
      </div></section>
      <div class="wrap">
        <a class="digest" :href="withBase('/digest/v1.37.0.html')" target="_blank" rel="noopener">
          <span class="badge ok">NEW</span>
          <span class="digest-body">
            <span class="digest-title">v1.37.0 dev digest</span>
            <span class="digest-sub">A timeline for every request from nginx to the browser, services that sleep when nothing uses them, secrets kept out of .env, and a dashboard a tenth of its old size.</span>
          </span>
          <span class="digest-cta">Read the digest →</span>
        </a>
      </div>
    </main>

    <footer><div class="wrap">
      <div class="foot">
        <div>
          <a class="brand" href="#top"><img :src="withBase('/assets/logo.svg')" alt="" width="26" height="26">Lerd</a>
          <p class="small">Open-source local PHP development for Linux and macOS, and Windows via WSL2. Runs on rootless Podman.</p>
          <div class="social">
              <a href="https://github.com/lerd-env/lerd" target="_blank" rel="noopener" aria-label="GitHub"><svg viewBox="0 0 24 24" fill="currentColor"><path d="M12 .297c-6.63 0-12 5.373-12 12 0 5.303 3.438 9.8 8.205 11.385.6.113.82-.258.82-.577 0-.285-.01-1.04-.015-2.04-3.338.724-4.042-1.61-4.042-1.61C4.422 18.07 3.633 17.7 3.633 17.7c-1.087-.744.084-.729.084-.729 1.205.084 1.838 1.236 1.838 1.236 1.07 1.835 2.809 1.305 3.495.998.108-.776.417-1.305.76-1.605-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23A11.509 11.509 0 0 1 12 5.803c1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222 0 1.606-.014 2.898-.014 3.293 0 .322.216.694.825.576C20.565 22.092 24 17.592 24 12.297c0-6.627-5.373-12-12-12"/></svg></a>
              <a href="https://discord.gg/5JK54s7xCC" target="_blank" rel="noopener" aria-label="Discord"><svg viewBox="0 0 24 24" fill="currentColor"><path d="M20.317 4.3698a19.7913 19.7913 0 00-4.8851-1.5152.0741.0741 0 00-.0785.0371c-.211.3753-.4447.8648-.6083 1.2495-1.8447-.2762-3.68-.2762-5.4868 0-.1636-.3933-.4058-.8742-.6177-1.2495a.077.077 0 00-.0785-.037 19.7363 19.7363 0 00-4.8852 1.515.0699.0699 0 00-.0321.0277C.5334 9.0458-.319 13.5799.0992 18.0578a.0824.0824 0 00.0312.0561c2.0528 1.5076 4.0413 2.4228 5.9929 3.0294a.0777.0777 0 00.0842-.0276c.4616-.6304.8731-1.2952 1.226-1.9942a.076.076 0 00-.0416-.1057c-.6528-.2476-1.2743-.5495-1.8722-.8923a.077.077 0 01-.0076-.1277c.1258-.0943.2517-.1923.3718-.2914a.0743.0743 0 01.0776-.0105c3.9278 1.7933 8.18 1.7933 12.0614 0a.0739.0739 0 01.0785.0095c.1202.099.246.1981.3728.2924a.077.077 0 01-.0066.1276 12.2986 12.2986 0 01-1.873.8914.0766.0766 0 00-.0407.1067c.3604.698.7719 1.3628 1.225 1.9932a.076.076 0 00.0842.0286c1.961-.6067 3.9495-1.5219 6.0023-3.0294a.077.077 0 00.0313-.0552c.5004-5.177-.8382-9.6739-3.5485-13.6604a.061.061 0 00-.0312-.0286zM8.02 15.3312c-1.1825 0-2.1569-1.0857-2.1569-2.419 0-1.3332.9555-2.4189 2.157-2.4189 1.2108 0 2.1757 1.0952 2.1568 2.419 0 1.3332-.9555 2.4189-2.1569 2.4189zm7.9748 0c-1.1825 0-2.1569-1.0857-2.1569-2.419 0-1.3332.9554-2.4189 2.1569-2.4189 1.2108 0 2.1757 1.0952 2.1568 2.419 0 1.3332-.946 2.4189-2.1568 2.4189Z"/></svg></a>
              <a href="https://x.com/lerdphp" target="_blank" rel="noopener" aria-label="X"><svg viewBox="0 0 24 24" fill="currentColor"><path d="M18.901 1.153h3.68l-8.04 9.19L24 22.846h-7.406l-5.8-7.584-6.638 7.584H.474l8.6-9.83L0 1.154h7.594l5.243 6.932ZM17.61 20.644h2.039L6.486 3.24H4.298Z"/></svg></a>
              <a href="https://bsky.app/profile/lerdphp.bsky.social" target="_blank" rel="noopener" aria-label="Bluesky"><svg viewBox="0 0 24 24" fill="currentColor"><path d="M12 10.8c-1.087-2.114-4.046-6.053-6.798-7.995C2.566.944 1.561 1.266.902 1.565.139 1.908 0 3.08 0 3.768c0 .69.378 5.65.624 6.479.815 2.736 3.713 3.66 6.383 3.364.136-.02.275-.039.415-.056-.138.022-.276.04-.415.056-3.912.58-7.387 2.005-2.83 7.078 5.013 5.19 6.87-1.113 7.823-4.308.953 3.195 2.05 9.271 7.733 4.308 4.267-4.308 1.172-6.498-2.74-7.078a8.741 8.741 0 0 1-.415-.056c.14.017.279.036.415.056 2.67.297 5.568-.628 6.383-3.364.246-.828.624-5.79.624-6.478 0-.69-.139-1.861-.902-2.206-.659-.298-1.664-.62-4.3 1.24C16.046 4.748 13.087 8.687 12 10.8Z"/></svg></a>
              <a href="https://reddit.com/r/lerd" target="_blank" rel="noopener" aria-label="Reddit"><svg viewBox="0 0 24 24" fill="currentColor"><path d="M12 0A12 12 0 0 0 0 12a12 12 0 0 0 12 12 12 12 0 0 0 12-12A12 12 0 0 0 12 0zm5.01 4.744c.688 0 1.25.561 1.25 1.249a1.25 1.25 0 0 1-2.498.056l-2.597-.547-.8 3.747c1.824.07 3.48.632 4.674 1.488.308-.309.73-.491 1.207-.491.968 0 1.754.786 1.754 1.754 0 .716-.435 1.333-1.01 1.614a3.111 3.111 0 0 1 .042.52c0 2.694-3.13 4.87-7.004 4.87-3.874 0-7.004-2.176-7.004-4.87 0-.183.015-.366.043-.534A1.748 1.748 0 0 1 4.028 12c0-.968.786-1.754 1.754-1.754.463 0 .898.196 1.207.49 1.207-.883 2.878-1.43 4.744-1.487l.885-4.182a.342.342 0 0 1 .14-.197.35.35 0 0 1 .238-.042l2.906.617a1.214 1.214 0 0 1 1.108-.701zM9.25 12C8.561 12 8 12.562 8 13.25c0 .687.561 1.248 1.25 1.248.687 0 1.248-.561 1.248-1.249 0-.688-.561-1.249-1.249-1.249zm5.5 0c-.687 0-1.248.561-1.248 1.25 0 .687.561 1.248 1.249 1.248.688 0 1.249-.561 1.249-1.249 0-.687-.562-1.249-1.25-1.249zm-5.466 3.99a.327.327 0 0 0-.231.094.33.33 0 0 0 0 .463c.842.842 2.484.913 2.961.913.477 0 2.105-.056 2.961-.913a.361.361 0 0 0 .029-.463.33.33 0 0 0-.464 0c-.547.533-1.684.73-2.512.73-.828 0-1.979-.196-2.512-.73a.326.326 0 0 0-.232-.095z"/></svg></a>
            </div>
        </div>
        <div><h2>Product</h2><ul>
          <li><a href="#lineup">Features</a></li>
          <li><a :href="withBase('/features/web-ui')">Web UI</a></li>
          <li><a :href="withBase('/features/mcp')">MCP server</a></li>
          <li><a :href="withBase('/features/tui')">Terminal dashboard</a></li>
          <li><a href="#compare">Comparison</a></li>
        </ul></div>
        <div><h2>Resources</h2><ul>
          <li><a :href="withBase('/getting-started/installation')">Installation</a></li>
          <li><a :href="withBase('/getting-started/requirements')">Documentation</a></li>
          <li><a :href="withBase('/usage/sites')">Site management</a></li>
          <li><a :href="withBase('/getting-started/laravel')">Laravel on Linux without Docker</a></li>
          <li><a :href="withBase('/getting-started/herd-linux')">Laravel Herd for Linux</a></li>
          <li><a :href="withBase('/getting-started/laragon-linux')">Laragon for Linux</a></li>
          <li><a :href="withBase('/getting-started/windows')">Lerd for Windows</a></li>
        </ul></div>
        <div><h2>Community</h2><ul>
          <li><a :href="GITHUB" target="_blank" rel="noopener">GitHub</a></li>
          <li><a :href="DISCORD" target="_blank" rel="noopener">Discord</a></li>
          <li><a :href="withBase('/changelog')">Changelog</a></li>
          <li><a :href="withBase('/getting-started/comparison')">Lerd vs other tools</a></li>
          <li><a :href="withBase('/contributing/building')">Contributing</a></li>
          <li><a href="https://github.com/sponsors/geodro" target="_blank" rel="noopener">Sponsors</a></li>
        </ul></div>
      </div>
      <div class="legal"><span>© 2026 <a href="https://github.com/geodro" target="_blank" rel="noopener">George Dumitrescu</a> · MIT-licensed open source</span><span>lerd.sh</span></div>
    </div></footer>

    <div v-if="zoom" ref="overlay" class="demo zoom" role="dialog" aria-modal="true" aria-label="Screenshot" tabindex="-1" @click="zoom = ''">
      <img :src="zoom" alt="">
    </div>

    <div v-if="demoOpen" ref="overlay" class="demo" @click.self="demoOpen = false">
      <div class="pwa" :class="`os-${os}`" role="dialog" aria-modal="true" aria-label="Lerd live demo">
        <div class="pwa-bar" :style="demoBar">
          <span v-if="os === 'mac'" class="mac-lights"><button class="mac-close" type="button" aria-label="Close demo" @click="demoOpen = false"></button><i></i><i></i></span>
          <img v-if="os === 'windows'" :src="withBase('/assets/logo.svg')" alt="" width="16" height="16">
          <span class="pwa-head"><span class="pwa-title">Lerd</span><span class="pwa-note">Live demo, sample data</span></span>
          <template v-if="os === 'windows'">
            <span class="pwa-ctl" aria-hidden="true"><i class="min"></i><i class="max"></i></span>
            <button class="pwa-close" type="button" aria-label="Close demo" @click="demoOpen = false">✕</button>
          </template>
          <button v-if="os === 'linux'" class="adw-close" type="button" aria-label="Close demo" @click="demoOpen = false"><svg viewBox="0 0 16 16" width="16" height="16" aria-hidden="true"><path d="M4.5 4.5l7 7m0-7l-7 7" stroke="currentColor" stroke-width="1.6" stroke-linecap="round"/></svg></button>
        </div>
        <iframe :src="withBase(`/demo/index.html?theme=${isDark ? 'dark' : 'light'}`)" title="Lerd dashboard live demo" @load="onDemoLoad"></iframe>
      </div>
    </div>
  </div>
</template>
