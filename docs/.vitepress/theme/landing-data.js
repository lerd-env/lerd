/* ===========================================================
   Lerd landing page, shared data
   Ported from the standalone landing mockup (data.js).
   Install commands use the canonical lerd.sh endpoints.
   =========================================================== */

import { MARKS } from './landing-marks.js'

/* ---- Brand glyphs: the store mark where one exists, a lettered tile otherwise ---- */
// c is drawn on the dark page, l (when set) on the light one where c is too pale.
export const LOGOS = {
  laravel:  { ch: 'L', c: '#ff2d20' },
  symfony:  { ch: 'S', c: '#cbd5e1', l: '#18181b' },
  wordpress:{ ch: 'W', c: '#38bdf8', l: '#21759b' },
  drupal:   { ch: 'D', c: '#60a5fa', l: '#0678be' },
  typo3:    { ch: 'T', c: '#ff8700', l: '#d46a00' },
  cake:     { ch: 'C', c: '#f87171', l: '#d33c43' },
  magento:  { ch: 'M', c: '#f46f25', l: '#d9531e' },
  statamic: { ch: 'S', c: '#a78bfa', l: '#7c3aed' },
  codeigniter: { ch: 'C', c: '#f97316', l: '#dd4814' },
  tempest:  { ch: 'T', c: '#29ABE2', l: '#0e7fb0' },
  winter:   { ch: 'W', c: '#2da7c7', l: '#1d7f99' },
  bedrock:  { ch: 'B', c: '#21759b' },
  lumen:    { ch: 'L', c: '#f4645f', l: '#e74430' },
  yii:      { ch: 'Y', c: '#40b3d8', l: '#1f8bb0' },
  mysql:    { ch: 'M', c: '#38bdf8', l: '#00758f' },
  postgres: { ch: 'P', c: '#60a5fa', l: '#336791' },
  redis:    { ch: 'R', c: '#ff5b50', l: '#dc382d' },
  meili:    { ch: 'M', c: '#fb7185', l: '#e11d48' },
  rustfs:   { ch: 'S', c: '#fbbf24', l: '#b45309' },
  mailpit:  { ch: '@', c: '#34d399', l: '#059669' },
  mongo:    { ch: 'M', c: '#34d399', l: '#059669' },
  stripe:   { ch: '$', c: '#a78bfa', l: '#635bff' },
  elastic:  { ch: 'E', c: '#00bfb3', l: '#00857c' },
}

// `bare` drops the tile and fills the box with the mark itself, for places that
// carry their own spacing (the framework strip) rather than needing a chip.
export function glyph(name, size, opts = {}) {
  const l = LOGOS[name] || { ch: '?', c: '#888' }
  const s = size || 100
  const m = MARKS[name]
  const inset = opts.bare ? 0.04 : 0.19
  const face = m
    ? `<svg x="${s * inset}" y="${s * inset}" width="${s * (1 - inset * 2)}" height="${s * (1 - inset * 2)}"
        viewBox="${m.vb}" fill="currentColor">${m.d}</svg>`
    : `<text x="50%" y="54%" dominant-baseline="middle" text-anchor="middle"
        font-family="JetBrains Mono, monospace" font-weight="600"
        font-size="${s * (opts.bare ? 0.86 : 0.5)}" fill="currentColor">${l.ch}</text>`
  // No stroke: the rect fills the viewBox, so a stroke on it is half-clipped on
  // every edge. The tint alone carries the plate.
  const tile = opts.bare
    ? ''
    : `<rect width="${s}" height="${s}" rx="${s * 0.26}" fill="currentColor" fill-opacity="0.15"/>`
  return `<svg viewBox="0 0 ${s} ${s}" xmlns="http://www.w3.org/2000/svg" style="width:100%;height:100%;--c:${l.c};--cl:${l.l || l.c}">
      ${tile}
      ${face}
    </svg>`
}

// The hero's counts. store.data.ts reads them at build so the HTML carries a
// number, and the page reads them again in the browser, since the stores and
// the catalog move between releases while lerd.sh only rebuilds on one.
export const COUNTS = {
  frameworks: ['https://raw.githubusercontent.com/lerd-env/frameworks/main/frameworks/index.json', 'frameworks'],
  services: ['https://raw.githubusercontent.com/lerd-env/services/main/services/index.json', 'services'],
  plugins: ['https://plugins.lerd.sh/index.json', 'plugins'],
}

// A failed fetch answers 0, and a 0 is never shown.
export async function countList([url, key]) {
  try {
    const res = await fetch(url)
    if (!res.ok) return 0
    const list = (await res.json())[key]
    return Array.isArray(list) ? list.length : 0
  } catch {
    return 0
  }
}

// builtin is the default stack the binary ships, which the services store
// leaves out; it is only added to a store that answered.
export async function liveCounts(builtin) {
  const [frameworks, services, plugins] = await Promise.all([countList(COUNTS.frameworks), countList(COUNTS.services), countList(COUNTS.plugins)])
  return { frameworks, services: services ? services + builtin : 0, plugins }
}

export const INSTALL = 'curl -fsSL https://lerd.sh/install.sh | bash'
// releases/latest/download always resolves to the newest desktop release's asset.
export const WINDOWS_INSTALLER = 'https://github.com/lerd-env/lerd-desktop/releases/latest/download/Install-Lerd.exe'

/* ---- AI agents: what an assistant can do through lerd's MCP server ---- */
export const ASSISTANTS = ['Claude Code', 'Cursor', 'Codex CLI', 'Gemini CLI', 'Copilot', 'Junie', 'Antigravity', 'Windsurf', 'OpenCode']
// The MCP tool each card in i18n agents.items names, in the same order.
export const AGENT_CMDS = ['site · service · worker', 'exec', 'request lenses', 'diag browser_events', 'worktree add', 'db snapshot']

/* ---- The lineup: one card per area, copy under i18n lineup.items[id], screenshot under public/assets ---- */
export const LINEUP = [
  { id: 'sites', cli: 'lerd init', href: '/usage/sites', shot: 'landing/site-overview' },
  { id: 'dashboard', cli: 'lerd dashboard', href: '/features/web-ui', shot: 'landing/dashboard' },
  { id: 'services', cli: 'lerd service start postgres', href: '/getting-started/services', shot: 'landing/services' },
  { id: 'requests', isNew: true, cli: 'lerd profile', href: '/features/request-timing', shot: 'landing/requests-timing' },
  { id: 'worktrees', cli: 'lerd worktree add feature/checkout', href: '/features/git-worktrees', shot: 'landing/worktree-add' },
  { id: 'runtimes', cli: 'lerd isolate 8.4', href: '/usage/php', shot: 'landing/system-php' },
  { id: 'mcp', cli: 'lerd mcp:inject', href: '/features/mcp', shot: 'landing/tinker', names: ASSISTANTS },
  { id: 'terminal', cli: 'lerd tui', href: '/features/tui', shot: 'landing/tui-dashboard' },
]

/* ---- Framework grid: logo key from LOGOS, docs page under /getting-started ---- */
export const FRAMEWORKS = [
  { logo: 'laravel', name: 'Laravel', page: 'laravel' },
  { logo: 'symfony', name: 'Symfony', page: 'symfony' },
  { logo: 'wordpress', name: 'WordPress', page: 'wordpress' },
  { logo: 'drupal', name: 'Drupal', page: 'drupal' },
  { logo: 'typo3', name: 'TYPO3', page: 'typo3' },
  { logo: 'magento', name: 'Magento', page: 'magento' },
  { logo: 'cake', name: 'CakePHP', page: 'cakephp' },
  { logo: 'statamic', name: 'Statamic', page: 'statamic' },
  { logo: 'codeigniter', name: 'CodeIgniter', page: 'codeigniter' },
  { logo: 'tempest', name: 'Tempest', page: 'tempest' },
  { logo: 'winter', name: 'Winter CMS', page: 'winter' },
  { logo: 'bedrock', name: 'Bedrock', page: 'bedrock' },
  { logo: 'lumen', name: 'Lumen', page: 'lumen' },
  { logo: 'yii', name: 'Yii', page: 'yii' },
]

/* ---- Apps beyond PHP: lerd init picks the dev server, nginx and TLS go in front ---- */
// The command each card in i18n runtimes.items shows, in the same order.
export const RUNTIME_CMDS = ['npm run dev', 'manage.py runserver', 'bin/rails server', 'go run . / cargo run', 'Containerfile.lerd']

/* ---- Platforms, copy in i18n platforms.items in the same order; a status replaces STABLE ---- */
export const PLATFORMS = [
  { name: 'Linux', href: '/getting-started/installation' },
  { name: 'macOS', href: '/getting-started/installation' },
  { name: 'Windows', status: 'WSL2', href: '/getting-started/wsl2' },
]

/* ---- Services showcase ---- */
export const SVC_SHOW = [
  { logo: 'mysql',    name: 'MySQL',       port: ':3306' },
  { logo: 'postgres', name: 'PostgreSQL',  port: ':5432' },
  { logo: 'redis',    name: 'Redis',       port: ':6379' },
  { logo: 'meili',    name: 'Meilisearch', port: ':7700' },
  { logo: 'rustfs',   name: 'RustFS / S3', port: ':9000' },
  { logo: 'mailpit',  name: 'Mailpit',     port: ':1025' },
  { logo: 'mongo',    name: 'MongoDB',     port: ':27017' },
  { logo: 'stripe',   name: 'Stripe Mock', port: ':12111' },
  { logo: 'elastic',  name: 'Elasticsearch', port: ':9200' },
]

/* ---- The rest of the store, named rather than carded, so the section stays compact.
   Split the way the store itself splits them: a preset with admin_for fronts another
   service rather than being one you would reach for on its own. ---- */
export const SVC_MORE = [
  'MariaDB', 'Valkey', 'Memcached', 'Typesense', 'ClickHouse',
  'OpenSearch', 'Solr', 'RabbitMQ', 'Kafka', 'Beanstalkd', 'Soketi', 'Mercure',
  'Selenium', 'Gotenberg', 'LocalStack', 'SpamAssassin', 'pgvector', 'TimescaleDB',
]

export const SVC_ADMIN = [
  'phpMyAdmin', 'Adminer', 'pgAdmin', 'Mongo Express', 'RedisInsight',
  'Elasticvue', 'Typesense Dashboard', 'OpenSearch Dashboards', 'Kafbat UI',
]

/* ---- Comparison, row labels in i18n compare.rows in the same order ---- */
export const CMP = {
  cols: ['Lerd', 'Laravel Herd', 'Laragon', 'DDEV', 'Lando', 'Sail'],
  rows: [
    ['yes', 'no', 'no', 'yes', 'yes', 'yes'],
    ['yes', 'yes', 'no', 'yes', 'yes', 'yes'],
    ['yes', 'yes', 'yes', 'yes', 'yes', 'yes'],
    ['yes', 'yes', 'yes', 'no', 'no', 'no'],
    ['yes', 'yes', 'partial', 'partial', 'partial', 'no'],
    ['yes', 'no', 'no', 'yes', 'yes', 'yes'],
    ['yes', 'partial', 'partial', 'yes', 'yes', 'yes'],
    ['yes', 'yes', 'yes', 'yes', 'partial', 'no'],
    ['yes', 'yes', 'yes', 'no', 'no', 'no'],
    ['yes', 'yes', 'no', 'no', 'no', 'no'],
    ['yes', 'yes', 'no', 'partial', 'no', 'no'],
    ['yes', 'no', 'no', 'no', 'no', 'no'],
    ['yes', 'yes', 'no', 'no', 'no', 'no'],
    ['yes', 'yes', 'no', 'no', 'no', 'no'],
    ['yes', 'no', 'no', 'yes', 'yes', 'no'],
  ],
}

// Where each answer in i18n faq.items links out, in the same order; '' for none.
export const FAQ_LINKS = ['', '', '/getting-started/wsl2', '', '', '/features/mcp', 'https://plugins.lerd.sh/', '/getting-started/installation#uninstall']
