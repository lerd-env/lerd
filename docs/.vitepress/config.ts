import { existsSync, readdirSync, readFileSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { createMarkdownRenderer, defineConfig } from 'vitepress'
import { atomFeed, parseReleases, recentChangelog } from './changelog.js'
import { FAQ } from './theme/landing-data.js'
import { SIDEBAR } from './sidebar.js'

const SITE_URL = 'https://lerd.sh'
const OG_IMAGE = `${SITE_URL}/assets/social-preview.png`
const PUBLIC_DIR = fileURLToPath(new URL('../public', import.meta.url))
const DIGEST_DIR = fileURLToPath(new URL('../public/digest', import.meta.url))
const CHANGELOG = fileURLToPath(new URL('../changelog.md', import.meta.url))
const hasDigest = (version: string) => existsSync(join(DIGEST_DIR, `v${version}.html`))

// Read the version off the Go source of truth so the structured data can't
// drift behind a release.
const VERSION_GO = fileURLToPath(new URL('../../internal/version/version.go', import.meta.url))
const SOFTWARE_VERSION = readFileSync(VERSION_GO, 'utf8').match(/Version\s*=\s*"([^"]+)"/)?.[1] ?? ''

export default defineConfig({
  title: 'Lerd',
  description: 'Open-source local PHP development for Linux, macOS and Windows (WSL2). Automatic .test domains, HTTPS, per-project PHP and Node, rootless Podman, no Docker daemon.',
  base: '/',
  lang: 'en-US',
  cleanUrls: true,

  sitemap: {
    hostname: SITE_URL,
    transformItems(items) {
      // The archive is noindex; listing it would contradict that.
      items = items.filter((item) => !item.url.startsWith('changelog-archive'))
      // Static digest pages live in public/ and aren't page-derived. Read the
      // directory rather than listing them, so a new release is never missed.
      for (const file of readdirSync(DIGEST_DIR).filter((f) => f.endsWith('.html')).sort().reverse()) {
        items.push({ url: `digest/${file}` })
      }
      return items
    },
  },

  head: [
    ['link', { rel: 'icon', type: 'image/svg+xml', href: '/assets/logo.svg' }],

    // Instrument Sans and JetBrains Mono, the landing page's and the docs' type, declared in style.css
    ['link', { rel: 'preload', href: '/assets/fonts/instrument-sans-latin.woff2', as: 'font', type: 'font/woff2', crossorigin: '' }],
    ['link', { rel: 'preload', href: '/assets/fonts/jetbrains-mono-latin.woff2', as: 'font', type: 'font/woff2', crossorigin: '' }],

    ['meta', { name: 'theme-color', content: '#FF2D20' }],

    // Open Graph
    ['meta', { property: 'og:type', content: 'website' }],
    ['meta', { property: 'og:site_name', content: 'Lerd' }],
    ['meta', { property: 'og:locale', content: 'en_US' }],
    ['meta', { property: 'og:image', content: OG_IMAGE }],
    ['meta', { property: 'og:image:type', content: 'image/png' }],
    ['meta', { property: 'og:image:width', content: '1499' }],
    ['meta', { property: 'og:image:height', content: '787' }],
    ['meta', { property: 'og:image:alt', content: 'Lerd, local PHP development for Linux, macOS and Windows' }],

    // Twitter / X
    ['meta', { name: 'twitter:card', content: 'summary_large_image' }],
    ['meta', { name: 'twitter:image', content: OG_IMAGE }],
    ['meta', { name: 'twitter:image:alt', content: 'Lerd, local PHP development for Linux, macOS and Windows' }],

    // Structured data (rich results / knowledge graph)
    [
      'script',
      { type: 'application/ld+json' },
      JSON.stringify({
        '@context': 'https://schema.org',
        '@graph': [
          {
            '@type': 'SoftwareApplication',
            name: 'Lerd',
            applicationCategory: 'DeveloperApplication',
            operatingSystem: 'Linux, macOS, Windows (WSL2)',
            description:
              'Open-source local PHP development environment for Linux, macOS and Windows (WSL2): automatic .test domains and HTTPS, per-project PHP 7.4–8.6 and Node, rootless Podman, a built-in Web UI, and an MCP server for AI agents. No Docker daemon.',
            keywords:
              'local PHP development, Laravel Herd for Linux, Laragon for Linux, Laragon alternative Linux, .test domains, rootless Podman, PHP-FPM, local development environment, MCP server, AI agents',
            url: SITE_URL,
            downloadUrl: `${SITE_URL}/install.sh`,
            softwareVersion: SOFTWARE_VERSION,
            license: 'https://opensource.org/licenses/MIT',
            image: OG_IMAGE,
            author: { '@type': 'Person', name: 'George Dumitrescu' },
            offers: { '@type': 'Offer', price: '0', priceCurrency: 'USD' },
          },
          {
            '@type': 'WebSite',
            name: 'Lerd',
            url: SITE_URL,
            description:
              'Documentation and downloads for Lerd, the open-source local PHP development environment for Linux, macOS and Windows (WSL2).',
          },
        ],
      }),
    ],
  ],

  markdown: {
    // The plain GitHub themes' comment, keyword and string colours sit under 4.5:1 on our code block backgrounds.
    theme: { light: 'github-light-high-contrast', dark: 'github-dark-high-contrast' },
    config(md) {
      md.core.ruler.before('normalize', 'recent-changelog', (state) => {
        if (state.env.relativePath === 'changelog.md') state.src = recentChangelog(state.src, hasDigest)
      })
    },
  },

  async buildEnd(siteConfig) {
    const md = await createMarkdownRenderer(siteConfig.srcDir, siteConfig.markdown, siteConfig.site.base)
    const { releases } = parseReleases(readFileSync(CHANGELOG, 'utf8'))
    writeFileSync(join(siteConfig.outDir, 'feed.xml'), atomFeed(releases, (src) => md.render(src), SITE_URL))
  },

  transformPageData(pageData, { siteConfig }) {
    const canonicalUrl = `${SITE_URL}/${pageData.relativePath.replace(/\.md$/, '').replace(/index$/, '')}`
    const description = pageData.frontmatter.description ?? pageData.description ?? siteConfig.site.description
    const title = pageData.frontmatter.title ?? pageData.title ?? siteConfig.site.title
    pageData.frontmatter.head ??= []
    pageData.frontmatter.head.push(
      ['link', { rel: 'canonical', href: canonicalUrl }],
      ['meta', { name: 'description', content: description }],
      ['meta', { property: 'og:title', content: title }],
      ['meta', { property: 'og:description', content: description }],
      ['meta', { property: 'og:url', content: canonicalUrl }],
      ['meta', { name: 'twitter:title', content: title }],
      ['meta', { name: 'twitter:description', content: description }],
    )
    if (pageData.relativePath === 'changelog.md') {
      pageData.frontmatter.head.push(['link', { rel: 'alternate', type: 'application/atom+xml', title: 'Lerd releases', href: '/feed.xml' }])
    }
    // The landing FAQ is rendered from the same list, so the markup and the page never disagree.
    if (pageData.relativePath === 'index.md') {
      const faq = {
        '@context': 'https://schema.org',
        '@type': 'FAQPage',
        mainEntity: FAQ.map((f) => ({ '@type': 'Question', name: f.q, acceptedAnswer: { '@type': 'Answer', text: f.a } })),
      }
      pageData.frontmatter.head.push(['script', { type: 'application/ld+json' }, JSON.stringify(faq)])
    }
  },

  themeConfig: {
    logo: { src: '/assets/logo.svg', width: 24, height: 24 },
    siteTitle: 'Lerd',

    nav: [
      { text: 'Docs', link: '/getting-started/' },
      { text: 'Configuration', link: '/configuration' },
      { text: 'Reference', link: '/reference/commands' },
      { text: 'Contributing', link: '/contributing/building' },
      { text: 'Changelog', link: '/changelog' },
    ],

    sidebar: SIDEBAR,

    // Pages put their real sections under ### and ####; showing only ## hid most of them.
    outline: { level: [2, 4] },

    socialLinks: [
      { icon: 'github', link: 'https://github.com/lerd-env/lerd' },
      { icon: 'discord', link: 'https://discord.gg/5JK54s7xCC' },
      {
        icon: {
          svg: '<svg role="img" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg"><title>X</title><path d="M18.901 1.153h3.68l-8.04 9.19L24 22.846h-7.406l-5.8-7.584-6.638 7.584H.474l8.6-9.83L0 1.154h7.594l5.243 6.932ZM17.61 20.644h2.039L6.486 3.24H4.298Z"></path></svg>',
        },
        link: 'https://x.com/lerdphp',
      },
      {
        icon: {
          svg: '<svg role="img" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg"><title>Bluesky</title><path d="M12 10.8c-1.087-2.114-4.046-6.053-6.798-7.995C2.566.944 1.561 1.266.902 1.565.139 1.908 0 3.08 0 3.768c0 .69.378 5.65.624 6.479.815 2.736 3.713 3.66 6.383 3.364.136-.02.275-.039.415-.056-.138.022-.276.04-.415.056-3.912.58-7.387 2.005-2.83 7.078 5.013 5.19 6.87-1.113 7.823-4.308.953 3.195 2.05 9.271 7.733 4.308 4.267-4.308 1.172-6.498-2.74-7.078a8.741 8.741 0 0 1-.415-.056c.14.017.279.036.415.056 2.67.297 5.568-.628 6.383-3.364.246-.828.624-5.79.624-6.478 0-.69-.139-1.861-.902-2.206-.659-.298-1.664-.62-4.3 1.24C16.046 4.748 13.087 8.687 12 10.8Z"></path></svg>',
        },
        link: 'https://bsky.app/profile/lerdphp.bsky.social',
      },
      {
        icon: {
          svg: '<svg role="img" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg"><title>Reddit</title><path d="M12 0A12 12 0 0 0 0 12a12 12 0 0 0 12 12 12 12 0 0 0 12-12A12 12 0 0 0 12 0zm5.01 4.744c.688 0 1.25.561 1.25 1.249a1.25 1.25 0 0 1-2.498.056l-2.597-.547-.8 3.747c1.824.07 3.48.632 4.674 1.488.308-.309.73-.491 1.207-.491.968 0 1.754.786 1.754 1.754 0 .716-.435 1.333-1.01 1.614a3.111 3.111 0 0 1 .042.52c0 2.694-3.13 4.87-7.004 4.87-3.874 0-7.004-2.176-7.004-4.87 0-.183.015-.366.043-.534A1.748 1.748 0 0 1 4.028 12c0-.968.786-1.754 1.754-1.754.463 0 .898.196 1.207.49 1.207-.883 2.878-1.43 4.744-1.487l.885-4.182a.342.342 0 0 1 .14-.197.35.35 0 0 1 .238-.042l2.906.617a1.214 1.214 0 0 1 1.108-.701zM9.25 12C8.561 12 8 12.562 8 13.25c0 .687.561 1.248 1.25 1.248.687 0 1.248-.561 1.248-1.249 0-.688-.561-1.249-1.249-1.249zm5.5 0c-.687 0-1.248.561-1.248 1.25 0 .687.561 1.248 1.249 1.248.688 0 1.249-.561 1.249-1.249 0-.687-.562-1.249-1.25-1.249zm-5.466 3.99a.327.327 0 0 0-.231.094.33.33 0 0 0 0 .463c.842.842 2.484.913 2.961.913.477 0 2.105-.056 2.961-.913a.361.361 0 0 0 .029-.463.33.33 0 0 0-.464 0c-.547.533-1.684.73-2.512.73-.828 0-1.979-.196-2.512-.73a.326.326 0 0 0-.232-.095z"></path></svg>',
        },
        link: 'https://reddit.com/r/lerd',
      },
    ],

    footer: {
      message: 'Released under the MIT License.',
      copyright: '© 2026 George Dumitrescu',
    },

    search: {
      provider: 'local',
    },

    editLink: {
      pattern: 'https://github.com/lerd-env/lerd/edit/main/docs/:path',
      text: 'Edit this page on GitHub',
    },
  },

  vite: {
    plugins: [
      {
        // A static host resolves a directory under public/ to its index.html;
        // the dev server hands those paths to the SPA instead, which is how the
        // demo iframe and the profiler stand-in both ended up loading the docs
        // site into themselves. Resolve them here so dev matches the deploy.
        name: 'lerd-public-dir-index',
        configureServer(server) {
          server.middlewares.use((req, _res, next) => {
            const [path, query] = (req.url || '').split('?')
            if (path.endsWith('/') && existsSync(PUBLIC_DIR + path + 'index.html')) {
              req.url = path + 'index.html' + (query ? '?' + query : '')
            }
            next()
          })
        },
      },
    ],
  },
})
