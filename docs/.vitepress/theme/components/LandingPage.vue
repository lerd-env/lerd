<script setup>
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { withBase } from 'vitepress'
import * as D from '../landing-data.js'
import { data as starData } from '../stars.data'
import { data as storeData } from '../store.data'

const GITHUB = 'https://github.com/lerd-env/lerd'
const DISCORD = 'https://discord.gg/5JK54s7xCC'
const pad = (n) => String(n).padStart(2, '0')

const stars = starData.stars
const starLabel = (n) => (n >= 1000 ? (n / 1000).toFixed(1).replace(/\.0$/, '') + 'k' : String(n))

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
const demoBar = ref('')
let demoObserver = null
function onDemoLoad(e) {
  const doc = e.target.contentDocument
  if (!doc) return
  const sync = () => {
    const rail = doc.querySelector('aside')
    if (rail) demoBar.value = getComputedStyle(rail).backgroundColor
  }
  sync()
  demoObserver = new MutationObserver(sync)
  demoObserver.observe(doc.documentElement, { attributes: true, attributeFilter: ['class', 'style', 'data-theme'] })
  demoObserver.observe(doc.body, { childList: true })
}
// A lineup screenshot is cropped to its panel; clicking it shows the whole capture.
const zoom = ref('')
const onDemoKey = (e) => { if (e.key === 'Escape') { demoOpen.value = false; zoom.value = '' } }
watch(() => demoOpen.value || Boolean(zoom.value), (open) => {
  document.documentElement.style.overflow = open ? 'hidden' : ''
  if (open) document.addEventListener('keydown', onDemoKey)
  else {
    document.removeEventListener('keydown', onDemoKey)
    if (demoObserver) { demoObserver.disconnect(); demoObserver = null }
  }
})
onBeforeUnmount(() => { demoOpen.value = false; document.documentElement.style.overflow = ''; document.removeEventListener('keydown', onDemoKey) })
</script>

<template>
  <div class="lerd-landing">

    <header class="hdr"><div class="wrap">
      <a class="brand" href="#top"><img :src="withBase('/assets/logo.svg')" alt="" width="26" height="26">Lerd</a>
      <nav>
        <a href="#lineup">Features</a>
        <a href="#frameworks">Frameworks</a>
        <a href="#services">Services</a>
        <a href="#principles">Why Lerd</a>
        <a href="#compare">Compare</a>
        <a :href="withBase('/getting-started/requirements')">Docs</a>
      </nav>
      <div class="hdr-right">
        <a class="gh" :href="GITHUB" target="_blank" rel="noopener">GitHub<span v-if="stars" class="stars">★ {{ starLabel(stars) }}</span></a>
        <a :href="DISCORD" target="_blank" rel="noopener">Discord</a>
        <a class="btn ink" href="#start">Install</a>
      </div>
    </div></header>

    <main id="top">
      <section class="hero"><div class="wrap">
        <div class="label"><b>00</b> / Open source, rootless, no Docker</div>
        <h1>Lerd <span>Local PHP development for Linux and macOS</span></h1>
        <p>Lerd gives each project a .test domain with HTTPS, its own PHP and Node versions, and the databases and services it needs, all running as rootless Podman containers. It's free and open source, with no Docker daemon and no sudo.</p>
        <div class="hero-row">
          <div class="cmd"><i>$</i><code>{{ D.INSTALL }}</code><button class="copy" type="button" @click="copyInstall">{{ copied ? 'COPIED' : 'COPY' }}</button></div>
          <button class="btn primary" type="button" @click="demoOpen = true">Try the live demo</button>
          <a class="btn" :href="withBase('/getting-started/requirements')">Read the docs →</a>
        </div>
        <div class="meta-row">
        <div class="meta">
          <div><strong>7.4 – 8.6</strong>PHP versions, 8.6 in preview</div>
          <div v-if="storeData.services"><strong>{{ storeData.services }}</strong>service presets</div>
          <div v-if="storeData.frameworks"><strong>{{ storeData.frameworks }}</strong>framework definitions</div>
          <div><strong>0</strong>sudo prompts</div>
        </div>
        <a class="sponsor" href="https://www.jetbrains.com/phpstorm/" target="_blank" rel="noopener">
          <span class="label">Sponsored by</span>
          <img :src="withBase('/assets/sponsors/phpstorm-dark.svg')" alt="JetBrains PhpStorm" height="36">
        </a>
        </div>
      </div>
        <div class="hero-art"><div class="win"><div class="win-bar"><i></i><i></i><i></i></div><img :src="withBase('/assets/landing/dashboard.png')" alt="Lerd web dashboard with sites, services and workers"></div></div>
      </section>

      <section class="sec" id="lineup" ref="lineup"><div class="wrap">
        <div class="sec-head">
          <div class="label"><b>01–{{ pad(D.LINEUP.length) }}</b> / The lineup</div>
          <h2>What you get <span>after one install.</span></h2>
        </div>
        <div class="slides" :class="{ paused: hovered || !onScreen }" @mouseenter="hovered = true" @mouseleave="hovered = false">
          <div class="index" role="tablist" aria-label="Features">
            <button v-for="(c, i) in D.LINEUP" :key="c.id" type="button" role="tab" :aria-selected="i === slide" :class="{ on: i === slide }" @click="slide = i">
              <span class="n">{{ pad(i + 1) }}</span><span class="t">{{ c.title }}</span>
              <span v-if="i === slide" class="bar" @animationend="next"></span>
            </button>
          </div>
          <div class="cards">
          <article v-for="(c, i) in D.LINEUP" :key="c.id" class="card" :class="{ on: i === slide }" role="tabpanel" :aria-hidden="i !== slide" :inert="i !== slide">
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
            <button class="shot" type="button" :aria-label="`Enlarge the ${c.title} screenshot`" @click="zoom = withBase(`/assets/${c.shot}.png`)"><img :src="withBase(`/assets/${c.shot}.png`)" :alt="c.alt" loading="lazy"></button>
          </article>
          </div>
        </div>
      </div></section>

      <section class="sec" id="frameworks"><div class="wrap">
        <div class="sec-head">
          <div class="label"><b>{{ pad(D.LINEUP.length + 1) }}</b> / Frameworks</div>
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
          <div class="label"><b>{{ pad(D.LINEUP.length + 2) }}</b> / Beyond PHP</div>
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
          <div class="label"><b>{{ pad(D.LINEUP.length + 3) }}</b> / Services</div>
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

      <section class="sec" id="platforms"><div class="wrap">
        <div class="sec-head">
          <div class="label"><b>{{ pad(D.LINEUP.length + 4) }}</b> / Platforms</div>
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
          <div class="label"><b>{{ pad(D.LINEUP.length + 5) }}</b> / Principles</div>
          <h2>How Lerd <span>is built.</span></h2>
        </div>
        <div class="grid6">
          <div v-for="(p, i) in D.PRINCIPLES" :key="p.h" class="p"><span class="n">{{ pad(i + 1) }}</span><h3>{{ p.h }}</h3><p>{{ p.p }}</p></div>
        </div>
      </div></section>

      <section class="sec" id="compare"><div class="wrap">
        <div class="sec-head">
          <div class="label"><b>{{ pad(D.LINEUP.length + 6) }}</b> / Compare</div>
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
        <p class="cmp-note">On Windows, Lerd runs inside WSL2 and is in beta, see the <a :href="withBase('/getting-started/wsl2')">Windows (WSL2) guide</a>. Coming from another tool? See <a :href="withBase('/getting-started/herd-linux')">Laravel Herd for Linux</a>, <a :href="withBase('/getting-started/laragon-linux')">Laragon for Linux</a> and the <a :href="withBase('/getting-started/comparison')">full comparison</a>.</p>
      </div></section>

      <section class="cta" id="start"><div class="wrap">
        <div class="label"><b>{{ pad(D.LINEUP.length + 7) }}</b> / Get started</div>
        <h2>Install, then run lerd init.</h2>
        <p>The script installs Lerd and sets up DNS and certificates. Then cd into a project, run lerd init and open its .test address.</p>
        <div class="hero-row">
          <div class="cmd"><i>$</i><code>{{ D.INSTALL }}</code><button class="copy" type="button" @click="copyInstall">{{ copied ? 'COPIED' : 'COPY' }}</button></div>
          <a class="btn" :href="GITHUB" target="_blank" rel="noopener">Star on GitHub<span v-if="stars" class="stars">{{ starLabel(stars) }}</span></a>
        </div>
      </div></section>
      <div class="wrap">
        <a class="digest" :href="withBase('/digest/v1.36.0.html')" target="_blank" rel="noopener">
          <span class="badge ok">NEW</span>
          <span class="digest-body">
            <span class="digest-title">v1.36.0 dev digest</span>
            <span class="digest-sub">A dashboard that wears the desktop it runs on, a Debug window that hears logs, exceptions and messages, and worktrees that pick their own database.</span>
          </span>
          <span class="digest-cta">Read the digest →</span>
        </a>
      </div>
    </main>

    <footer><div class="wrap">
      <div class="foot">
        <div>
          <a class="brand" href="#top"><img :src="withBase('/assets/logo.svg')" alt="" width="26" height="26">Lerd</a>
          <p class="small">Open-source local PHP development for Linux and macOS, and Windows via WSL2 (beta). Runs on rootless Podman.</p>
          <div class="social">
              <a href="https://github.com/lerd-env/lerd" target="_blank" rel="noopener" aria-label="GitHub"><svg viewBox="0 0 24 24" fill="currentColor"><path d="M12 .297c-6.63 0-12 5.373-12 12 0 5.303 3.438 9.8 8.205 11.385.6.113.82-.258.82-.577 0-.285-.01-1.04-.015-2.04-3.338.724-4.042-1.61-4.042-1.61C4.422 18.07 3.633 17.7 3.633 17.7c-1.087-.744.084-.729.084-.729 1.205.084 1.838 1.236 1.838 1.236 1.07 1.835 2.809 1.305 3.495.998.108-.776.417-1.305.76-1.605-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23A11.509 11.509 0 0 1 12 5.803c1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222 0 1.606-.014 2.898-.014 3.293 0 .322.216.694.825.576C20.565 22.092 24 17.592 24 12.297c0-6.627-5.373-12-12-12"/></svg></a>
              <a href="https://discord.gg/5JK54s7xCC" target="_blank" rel="noopener" aria-label="Discord"><svg viewBox="0 0 24 24" fill="currentColor"><path d="M20.317 4.3698a19.7913 19.7913 0 00-4.8851-1.5152.0741.0741 0 00-.0785.0371c-.211.3753-.4447.8648-.6083 1.2495-1.8447-.2762-3.68-.2762-5.4868 0-.1636-.3933-.4058-.8742-.6177-1.2495a.077.077 0 00-.0785-.037 19.7363 19.7363 0 00-4.8852 1.515.0699.0699 0 00-.0321.0277C.5334 9.0458-.319 13.5799.0992 18.0578a.0824.0824 0 00.0312.0561c2.0528 1.5076 4.0413 2.4228 5.9929 3.0294a.0777.0777 0 00.0842-.0276c.4616-.6304.8731-1.2952 1.226-1.9942a.076.076 0 00-.0416-.1057c-.6528-.2476-1.2743-.5495-1.8722-.8923a.077.077 0 01-.0076-.1277c.1258-.0943.2517-.1923.3718-.2914a.0743.0743 0 01.0776-.0105c3.9278 1.7933 8.18 1.7933 12.0614 0a.0739.0739 0 01.0785.0095c.1202.099.246.1981.3728.2924a.077.077 0 01-.0066.1276 12.2986 12.2986 0 01-1.873.8914.0766.0766 0 00-.0407.1067c.3604.698.7719 1.3628 1.225 1.9932a.076.076 0 00.0842.0286c1.961-.6067 3.9495-1.5219 6.0023-3.0294a.077.077 0 00.0313-.0552c.5004-5.177-.8382-9.6739-3.5485-13.6604a.061.061 0 00-.0312-.0286zM8.02 15.3312c-1.1825 0-2.1569-1.0857-2.1569-2.419 0-1.3332.9555-2.4189 2.157-2.4189 1.2108 0 2.1757 1.0952 2.1568 2.419 0 1.3332-.9555 2.4189-2.1569 2.4189zm7.9748 0c-1.1825 0-2.1569-1.0857-2.1569-2.419 0-1.3332.9554-2.4189 2.1569-2.4189 1.2108 0 2.1757 1.0952 2.1568 2.419 0 1.3332-.946 2.4189-2.1568 2.4189Z"/></svg></a>
              <a href="https://x.com/lerdphp" target="_blank" rel="noopener" aria-label="X"><svg viewBox="0 0 24 24" fill="currentColor"><path d="M18.901 1.153h3.68l-8.04 9.19L24 22.846h-7.406l-5.8-7.584-6.638 7.584H.474l8.6-9.83L0 1.154h7.594l5.243 6.932ZM17.61 20.644h2.039L6.486 3.24H4.298Z"/></svg></a>
              <a href="https://bsky.app/profile/lerdphp.bsky.social" target="_blank" rel="noopener" aria-label="Bluesky"><svg viewBox="0 0 24 24" fill="currentColor"><path d="M12 10.8c-1.087-2.114-4.046-6.053-6.798-7.995C2.566.944 1.561 1.266.902 1.565.139 1.908 0 3.08 0 3.768c0 .69.378 5.65.624 6.479.815 2.736 3.713 3.66 6.383 3.364.136-.02.275-.039.415-.056-.138.022-.276.04-.415.056-3.912.58-7.387 2.005-2.83 7.078 5.013 5.19 6.87-1.113 7.823-4.308.953 3.195 2.05 9.271 7.733 4.308 4.267-4.308 1.172-6.498-2.74-7.078a8.741 8.741 0 0 1-.415-.056c.14.017.279.036.415.056 2.67.297 5.568-.628 6.383-3.364.246-.828.624-5.79.624-6.478 0-.69-.139-1.861-.902-2.206-.659-.298-1.664-.62-4.3 1.24C16.046 4.748 13.087 8.687 12 10.8Z"/></svg></a>
              <a href="https://reddit.com/r/lerd" target="_blank" rel="noopener" aria-label="Reddit"><svg viewBox="0 0 24 24" fill="currentColor"><path d="M12 0A12 12 0 0 0 0 12a12 12 0 0 0 12 12 12 12 0 0 0 12-12A12 12 0 0 0 12 0zm5.01 4.744c.688 0 1.25.561 1.25 1.249a1.25 1.25 0 0 1-2.498.056l-2.597-.547-.8 3.747c1.824.07 3.48.632 4.674 1.488.308-.309.73-.491 1.207-.491.968 0 1.754.786 1.754 1.754 0 .716-.435 1.333-1.01 1.614a3.111 3.111 0 0 1 .042.52c0 2.694-3.13 4.87-7.004 4.87-3.874 0-7.004-2.176-7.004-4.87 0-.183.015-.366.043-.534A1.748 1.748 0 0 1 4.028 12c0-.968.786-1.754 1.754-1.754.463 0 .898.196 1.207.49 1.207-.883 2.878-1.43 4.744-1.487l.885-4.182a.342.342 0 0 1 .14-.197.35.35 0 0 1 .238-.042l2.906.617a1.214 1.214 0 0 1 1.108-.701zM9.25 12C8.561 12 8 12.562 8 13.25c0 .687.561 1.248 1.25 1.248.687 0 1.248-.561 1.248-1.249 0-.688-.561-1.249-1.249-1.249zm5.5 0c-.687 0-1.248.561-1.248 1.25 0 .687.561 1.248 1.249 1.248.688 0 1.249-.561 1.249-1.249 0-.687-.562-1.249-1.25-1.249zm-5.466 3.99a.327.327 0 0 0-.231.094.33.33 0 0 0 0 .463c.842.842 2.484.913 2.961.913.477 0 2.105-.056 2.961-.913a.361.361 0 0 0 .029-.463.33.33 0 0 0-.464 0c-.547.533-1.684.73-2.512.73-.828 0-1.979-.196-2.512-.73a.326.326 0 0 0-.232-.095z"/></svg></a>
            </div>
        </div>
        <div><h5>Product</h5><ul>
          <li><a href="#lineup">Features</a></li>
          <li><a :href="withBase('/features/web-ui')">Web UI</a></li>
          <li><a :href="withBase('/features/mcp')">MCP server</a></li>
          <li><a :href="withBase('/features/tui')">Terminal dashboard</a></li>
          <li><a href="#compare">Comparison</a></li>
        </ul></div>
        <div><h5>Resources</h5><ul>
          <li><a :href="withBase('/getting-started/installation')">Installation</a></li>
          <li><a :href="withBase('/getting-started/requirements')">Documentation</a></li>
          <li><a :href="withBase('/usage/sites')">Site management</a></li>
          <li><a :href="withBase('/getting-started/laravel')">Laravel walkthrough</a></li>
          <li><a :href="withBase('/getting-started/herd-linux')">Laravel Herd for Linux</a></li>
          <li><a :href="withBase('/getting-started/laragon-linux')">Laragon for Linux</a></li>
        </ul></div>
        <div><h5>Community</h5><ul>
          <li><a :href="GITHUB" target="_blank" rel="noopener">GitHub</a></li>
          <li><a :href="DISCORD" target="_blank" rel="noopener">Discord</a></li>
          <li><a :href="withBase('/changelog')">Changelog</a></li>
          <li><a :href="withBase('/getting-started/comparison')">Lerd vs other tools</a></li>
          <li><a :href="withBase('/contributing/building')">Contributing</a></li>
          <li><a href="https://github.com/sponsors/geodro" target="_blank" rel="noopener">Sponsors</a></li>
        </ul></div>
      </div>
      <div class="legal"><span>© 2026 Lerd · MIT-licensed open source</span><span>lerd.sh</span></div>
    </div></footer>

    <div v-if="zoom" class="demo zoom" role="dialog" aria-modal="true" aria-label="Screenshot" @click="zoom = ''">
      <img :src="zoom" alt="">
    </div>

    <div v-if="demoOpen" class="demo" @click.self="demoOpen = false">
      <div class="pwa" role="dialog" aria-modal="true" aria-label="Lerd live demo">
        <div class="pwa-bar" :style="{ background: demoBar }">
          <img :src="withBase('/assets/logo.svg')" alt="" width="16" height="16">
          <span class="pwa-title">Lerd</span>
          <span class="pwa-note">Live demo, sample data</span>
          <span class="pwa-ctl" aria-hidden="true"><i class="min"></i><i class="max"></i></span>
          <button class="pwa-close" type="button" aria-label="Close demo" @click="demoOpen = false">✕</button>
        </div>
        <iframe :src="withBase('/demo/index.html')" title="Lerd dashboard live demo" @load="onDemoLoad"></iframe>
      </div>
    </div>
  </div>
</template>
