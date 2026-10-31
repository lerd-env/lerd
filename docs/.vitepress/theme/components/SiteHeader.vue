<script setup>
import { ref } from 'vue'
import { useData, withBase } from 'vitepress'
import { data as starData } from '../stars.data'

// Off the landing page its section links point back to /.
defineProps({ home: { type: String, default: '' } })
const GITHUB = 'https://github.com/lerd-env/lerd'
const DISCORD = 'https://discord.gg/5JK54s7xCC'
const { isDark } = useData()
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
  <header class="hdr" :class="{ open: menuOpen }" @keydown.esc="closeMenu"><div class="wrap">
    <a class="brand" :href="`${home}#top`"><img :src="withBase('/assets/logo.svg')" alt="" width="26" height="26">Lerd</a>
    <nav id="hdr-nav" @click="menuOpen = false">
      <a :href="`${home}#lineup`">Features</a>
      <a :href="`${home}#agents`">AI agents</a>
      <a :href="`${home}#frameworks`">Frameworks</a>
      <a :href="`${home}#services`">Services</a>
      <a :href="`${home}#plugins`">Plugins</a>
      <a :href="`${home}#principles`">Why Lerd</a>
      <a :href="`${home}#compare`">Compare</a>
      <a :href="`${home}#faq`">FAQ</a>
      <a :href="withBase('/getting-started/requirements')">Docs</a>
    </nav>
    <div class="hdr-right">
      <a class="gh" :href="GITHUB" target="_blank" rel="noopener">GitHub<span v-if="stars" class="stars">★ {{ starLabel(stars) }}</span></a>
      <a class="discord" :href="DISCORD" target="_blank" rel="noopener" aria-label="Discord"><svg viewBox="0 0 24 24" fill="currentColor"><path d="M20.317 4.3698a19.7913 19.7913 0 00-4.8851-1.5152.0741.0741 0 00-.0785.0371c-.211.3753-.4447.8648-.6083 1.2495-1.8447-.2762-3.68-.2762-5.4868 0-.1636-.3933-.4058-.8742-.6177-1.2495a.077.077 0 00-.0785-.037 19.7363 19.7363 0 00-4.8852 1.515.0699.0699 0 00-.0321.0277C.5334 9.0458-.319 13.5799.0992 18.0578a.0824.0824 0 00.0312.0561c2.0528 1.5076 4.0413 2.4228 5.9929 3.0294a.0777.0777 0 00.0842-.0276c.4616-.6304.8731-1.2952 1.226-1.9942a.076.076 0 00-.0416-.1057c-.6528-.2476-1.2743-.5495-1.8722-.8923a.077.077 0 01-.0076-.1277c.1258-.0943.2517-.1923.3718-.2914a.0743.0743 0 01.0776-.0105c3.9278 1.7933 8.18 1.7933 12.0614 0a.0739.0739 0 01.0785.0095c.1202.099.246.1981.3728.2924a.077.077 0 01-.0066.1276 12.2986 12.2986 0 01-1.873.8914.0766.0766 0 00-.0407.1067c.3604.698.7719 1.3628 1.225 1.9932a.076.076 0 00.0842.0286c1.961-.6067 3.9495-1.5219 6.0023-3.0294a.077.077 0 00.0313-.0552c.5004-5.177-.8382-9.6739-3.5485-13.6604a.061.061 0 00-.0312-.0286zM8.02 15.3312c-1.1825 0-2.1569-1.0857-2.1569-2.419 0-1.3332.9555-2.4189 2.157-2.4189 1.2108 0 2.1757 1.0952 2.1568 2.419 0 1.3332-.9555 2.4189-2.1569 2.4189zm7.9748 0c-1.1825 0-2.1569-1.0857-2.1569-2.419 0-1.3332.9554-2.4189 2.1569-2.4189 1.2108 0 2.1757 1.0952 2.1568 2.419 0 1.3332-.946 2.4189-2.1568 2.4189Z"/></svg></a>
      <button class="theme" type="button" :aria-label="isDark ? 'Switch to light theme' : 'Switch to dark theme'" @click="isDark = !isDark">
        <svg class="on-light" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8z"/></svg>
        <svg class="on-dark" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"/></svg>
      </button>
      <a class="btn ink" :href="`${home}#start`">Install</a>
      <button ref="menuBtn" class="menu" type="button" aria-controls="hdr-nav" :aria-expanded="menuOpen" :aria-label="menuOpen ? 'Close menu' : 'Open menu'" @click="menuOpen = !menuOpen">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><path v-if="menuOpen" d="M6 6l12 12M18 6L6 18"/><path v-else d="M4 7h16M4 12h16M4 17h16"/></svg>
      </button>
    </div>
  </div></header>
</template>
