<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useData, withBase } from 'vitepress'
import { data as starData } from '../stars.data'
import { LOCALES, STORAGE_KEY, localeOf } from '../i18n/locales.js'

// Off the landing page its section links point back to /.
defineProps({ home: { type: String, default: '' } })
const GITHUB = 'https://github.com/lerd-env/lerd'
const PLUGINS = 'https://plugins.lerd.sh/'
const { isDark, frontmatter, page } = useData()
const t = computed(() => frontmatter.value.t.nav)
// The picker only sits on the landing page, the one page that is translated.
const translated = computed(() => frontmatter.value.layout === 'landing')
const current = computed(() => localeOf(page.value.relativePath))
// Remembering the pick stops the root page sending this visitor back to the browser's language.
// The site swaps pages without a reload, so the open menu has to be closed by hand.
const langMenu = ref(null)
const closeLang = () => { if (langMenu.value) langMenu.value.open = false }
function remember(code) {
  localStorage.setItem(STORAGE_KEY, code)
  closeLang()
}
const onOutside = (e) => { if (langMenu.value && !langMenu.value.contains(e.target)) closeLang() }
onMounted(() => document.addEventListener('click', onOutside))
onBeforeUnmount(() => document.removeEventListener('click', onOutside))
const stars = starData.stars
const starLabel = (n) => (n >= 1000 ? (n / 1000).toFixed(1).replace(/\.0$/, '') + 'k' : String(n))

// Below 960px the nav folds behind a menu button.
const menuOpen = ref(false)
const menuBtn = ref(null)
// Escape hides the link that had focus, so focus goes back to the button.
function closeMenu() {
  if (!menuOpen.value) return
  menuOpen.value = false
  menuBtn.value.focus()
}
</script>

<template>
  <header class="hdr" :class="{ open: menuOpen }" @keydown.esc="closeLang(); closeMenu()"><div class="wrap">
    <a class="brand" :href="`${home}#top`"><img class="on-light" :src="withBase('/assets/logo.svg')" alt="" width="26" height="26"><img class="on-dark" :src="withBase('/assets/logo-dark.svg')" alt="" width="26" height="26">Lerd</a>
    <nav id="hdr-nav" @click="menuOpen = false">
      <a :href="`${home}#lineup`">{{ t.features }}</a>
      <a :href="`${home}#agents`">{{ t.agents }}</a>
      <a :href="PLUGINS" target="_blank" rel="noopener">{{ t.plugins }}</a>
      <a :href="`${home}#principles`">{{ t.why }}</a>
      <a :href="`${home}#compare`">{{ t.compare }}</a>
      <a :href="`${home}#faq`">{{ t.faq }}</a>
      <a :href="withBase('/getting-started/requirements')">{{ t.docs }}</a>
    </nav>
    <div class="hdr-right">
      <a class="gh" :href="GITHUB" target="_blank" rel="noopener" aria-label="GitHub"><svg viewBox="0 0 24 24" width="18" height="18" fill="currentColor" aria-hidden="true"><path d="M12 .297c-6.63 0-12 5.373-12 12 0 5.303 3.438 9.8 8.205 11.385.6.113.82-.258.82-.577 0-.285-.01-1.04-.015-2.04-3.338.724-4.042-1.61-4.042-1.61C4.422 18.07 3.633 17.7 3.633 17.7c-1.087-.744.084-.729.084-.729 1.205.084 1.838 1.236 1.838 1.236 1.07 1.835 2.809 1.305 3.495.998.108-.776.417-1.305.76-1.605-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23A11.509 11.509 0 0 1 12 5.803c1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222 0 1.606-.014 2.898-.014 3.293 0 .322.216.694.825.576C20.565 22.092 24 17.592 24 12.297c0-6.627-5.373-12-12-12"/></svg><span v-if="stars" class="stars">★ {{ starLabel(stars) }}</span></a>
      <div class="prefs">
        <details v-if="translated" ref="langMenu" class="lang">
          <summary :aria-label="t.language"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><circle cx="12" cy="12" r="9"/><path d="M3 12h18M12 3a14 14 0 0 1 0 18M12 3a14 14 0 0 0 0 18"/></svg>{{ current.toUpperCase() }}</summary>
          <div class="lang-menu">
            <a v-for="(l, code) in LOCALES" :key="code" :href="withBase(l.path)" :hreflang="code" :lang="code" :aria-current="code === current ? 'true' : undefined" @click="remember(code)">{{ l.label }}</a>
          </div>
        </details>
        <button class="theme" type="button" :aria-label="isDark ? t.toLight : t.toDark" @click="isDark = !isDark">
          <svg class="on-light" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8z"/></svg>
          <svg class="on-dark" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"/></svg>
        </button>
      </div>
      <a class="btn ink" :href="`${home}#start`">{{ t.install }}</a>
      <button ref="menuBtn" class="menu" type="button" aria-controls="hdr-nav" :aria-expanded="menuOpen" :aria-label="menuOpen ? t.closeMenu : t.openMenu" @click="menuOpen = !menuOpen">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><path v-if="menuOpen" d="M6 6l12 12M18 6L6 18"/><path v-else d="M4 7h16M4 12h16M4 17h16"/></svg>
      </button>
    </div>
  </div></header>
</template>
