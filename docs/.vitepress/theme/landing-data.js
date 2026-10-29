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

/* ---- The lineup: one full-width card per area, screenshot path under public/assets ---- */
/* ---- AI agents: what an assistant can do through lerd's MCP server ---- */
export const ASSISTANTS = ['Claude Code', 'Cursor', 'Codex CLI', 'Gemini CLI', 'Copilot', 'Junie', 'Antigravity', 'Windsurf', 'OpenCode']
export const AGENTS = [
  { name: 'Manages the environment', cmd: 'site · service · worker', p: 'Links sites, switches PHP and Node versions, starts databases and caches, and runs queue workers and the scheduler.' },
  { name: 'Runs commands', cmd: 'exec', p: "artisan, composer and the project's own console commands, on the PHP version the site uses." },
  { name: 'Reads each request', cmd: 'request lenses', p: 'N+1 and slow queries, exceptions, error logs and failed jobs for one request, looked up by its id.' },
  { name: 'Sees browser errors', cmd: 'diag browser_events', p: "JavaScript errors and failed fetches from the site's pages, once browser logs are on for that site." },
  { name: 'Works in worktrees', cmd: 'worktree add', p: 'Each worktree gets its own .test domain, its dependencies installed, and a database that is shared, cloned or empty.' },
  { name: 'Snapshots the database', cmd: 'db snapshot', p: 'Before a branch switch or a migration. Each snapshot records its branch, so the right one can be restored later.' },
]

export const LINEUP = [
  { id: 'sites', title: 'Sites', h: 'Run lerd init, get a .test domain.',
    p: 'Lerd detects the framework, runs composer and npm, migrates the database, starts the workers and issues a trusted certificate. Nginx and PHP-FPM serve it at https://project.test.',
    alt: 'Lerd site page with runtime, workers, services and request timing for acme.test',
    tags: ['Linux', 'macOS', 'WSL2'], cli: 'lerd init',
    more: 'Explore sites', href: '/usage/sites', shot: 'landing/site-overview' },
  { id: 'dashboard', title: 'Dashboard', h: 'A dashboard in the browser.',
    p: 'Sites, services, workers and PHP versions on one screen. Start and stop things, tail logs, or open a shell or Tinker without leaving the tab. It installs as a PWA too.',
    alt: 'Lerd web dashboard listing sites, services, workers, system health and resources',
    tags: ['Web UI', 'PWA', 'Tray'], cli: 'lerd dashboard',
    more: 'Explore the dashboard', href: '/features/web-ui', shot: 'landing/dashboard' },
  { id: 'services', title: 'Services', h: 'Services wired into .env.',
    p: "MySQL, PostgreSQL, Redis, Meilisearch, Mailpit, S3 and about thirty more. Adding one writes its host, port and credentials into the site's .env, and it sleeps while nothing uses it.",
    alt: 'Lerd services page with installed databases, caches, search and mail services',
    tags: ['Rootless', 'On demand', 'Idle suspend'], cli: 'lerd service start postgres',
    more: 'Explore services', href: '/getting-started/services', shot: 'landing/services' },
  { id: 'requests', title: 'Requests', status: 'NEW', h: 'Find the slow request.',
    p: 'Lerd times each request and keeps its queries, logs and outgoing calls. Spot the N+1 on a slow route, profile it as an SPX flame graph, and read dump() output in a debug window.',
    alt: 'Lerd request timing with response time histogram, throughput and slowest routes',
    tags: ['Flame graphs', 'Queries', 'dump()'], cli: 'lerd profile',
    more: 'Explore the inspector', href: '/features/request-timing', shot: 'landing/requests-timing' },
  { id: 'worktrees', title: 'Worktrees', h: 'Worktrees get their own domain.',
    p: 'Add a git worktree and it gets its own domain, certificate, dependencies and database. The database is cloned from main, shared or left empty, based on the migrations the branch adds.',
    alt: 'Lerd add worktree dialog with branch, base, database and frontend asset options',
    tags: ['git', 'Per-branch DB'], cli: 'lerd worktree add feature/checkout',
    more: 'Explore worktrees', href: '/features/git-worktrees', shot: 'landing/worktree-add' },
  { id: 'runtimes', title: 'Runtimes', h: 'PHP and Node, pinned per project.',
    p: 'Each site picks its PHP version, 7.4 to 8.6, and its Node version, and Xdebug is a toggle away. Node, Python and Rails apps get dev servers and HTTPS on the same .test domains.',
    alt: 'Lerd PHP page with installed PHP-FPM versions and live PHP logs',
    tags: ['PHP 7.4–8.6', 'Node', 'Python', 'Rails'], cli: 'lerd isolate 8.4',
    more: 'Explore runtimes', href: '/usage/php', shot: 'landing/system-php' },
  { id: 'mcp', title: 'AI / MCP', h: 'An MCP server for your AI assistant.',
    p: 'Lerd ships an MCP server, so Claude, Cursor, Codex and the others can run artisan and Tinker, start services and workers, read logs and look into slow routes from the chat.',
    alt: 'Lerd Tinker REPL in the browser, the same PHP shell AI assistants drive over MCP',
    tags: ['MCP', 'artisan', 'Tinker'], cli: 'lerd mcp:inject',
    names: ASSISTANTS,
    more: 'Explore MCP', href: '/features/mcp', shot: 'landing/tinker' },
  { id: 'terminal', title: 'Terminal', h: 'The same dashboard in a terminal.',
    p: 'The TUI shows sites, services, workers and logs, and works with keyboard or mouse. ctrl+p jumps to anything or runs any action, so you can stay in the shell.',
    alt: 'Lerd terminal dashboard (TUI) with sites, services, resources and system status',
    tags: ['TUI', 'Mouse', 'ctrl+p'], cli: 'lerd tui',
    more: 'Explore the TUI', href: '/features/tui', shot: 'landing/tui-dashboard' },
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
export const RUNTIMES = [
  { name: 'Node.js', cmd: 'npm run dev', p: 'Next, Nuxt, NestJS, Vite, Angular or plain Express, on the Node version the project pins.' },
  { name: 'Python', cmd: 'manage.py runserver', p: 'Django starts with runserver on its own. Flask, FastAPI and the rest run your command.' },
  { name: 'Rails & Rack', cmd: 'bin/rails server', p: 'Rails runs bin/rails server, a config.ru runs as Rack, and a mise or rbenv Ruby is found.' },
  { name: 'Go & Rust', cmd: 'go run . / cargo run', p: 'go run . and cargo run work out of the box, or Lerd runs your own file watcher in their place.' },
  { name: 'Containers', cmd: 'Containerfile.lerd', p: "If you'd rather not install the runtime, Lerd writes a starter Containerfile and runs it rootless." },
]

/* ---- Platforms ---- */
export const PLATFORMS = [
  { name: 'Linux', status: 'STABLE', href: '/getting-started/installation',
    p: 'Rootless Podman under systemd on Ubuntu, Fedora, Arch, Omarchy and any other systemd distro, with an Omarchy bar widget. No Docker daemon.' },
  { name: 'macOS', status: 'STABLE', href: '/getting-started/installation',
    p: 'Local PHP development on a Mac without a paid tier, using the same dashboard, TUI and MCP server. PHP-FPM and workers can also run natively on the host, in beta.' },
  { name: 'Windows', status: 'WSL2', href: '/getting-started/wsl2',
    p: 'Lerd runs inside WSL2 with the same CLI, dashboard, services and MCP server. The Windows installer sets it all up, and the WSL2 guide covers each step.' },
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

/* ---- Comparison ---- */
export const CMP = {
  cols: ['Lerd', 'Laravel Herd', 'Laragon', 'DDEV', 'Lando', 'Sail'],
  rows: [
    { f: 'Runs on Linux',            v: ['yes', 'no', 'no', 'yes', 'yes', 'yes'] },
    { f: 'Runs on macOS',            v: ['yes', 'yes', 'no', 'yes', 'yes', 'yes'] },
    { f: 'Runs on Windows',          v: ['yes', 'yes', 'yes', 'yes', 'yes', 'yes'] },
    { f: 'No Docker daemon',         v: ['yes', 'yes', 'yes', 'no', 'no', 'no'] },
    { f: 'Rootless',                 v: ['yes', 'yes', 'partial', 'partial', 'partial', 'no'] },
    { f: 'Open source',              v: ['yes', 'no', 'no', 'yes', 'yes', 'yes'] },
    { f: 'Free, no paid tier',       v: ['yes', 'partial', 'partial', 'yes', 'yes', 'yes'] },
    { f: 'Automatic .test + TLS',    v: ['yes', 'yes', 'yes', 'yes', 'partial', 'no'] },
    { f: 'Zero required config',    v: ['yes', 'yes', 'yes', 'no', 'no', 'no'] },
    { f: 'Git worktrees as sites',   v: ['yes', 'yes', 'no', 'no', 'no', 'no'] },
    { f: 'Built-in Web UI',          v: ['yes', 'yes', 'no', 'partial', 'no', 'no'] },
    { f: 'Terminal dashboard',       v: ['yes', 'no', 'no', 'no', 'no', 'no'] },
    { f: 'MCP server for AI agents', v: ['yes', 'yes', 'no', 'no', 'no', 'no'] },
    { f: 'Profiler & debug window',  v: ['yes', 'yes', 'no', 'no', 'no', 'no'] },
    { f: 'Plugins',                  v: ['yes', 'no', 'no', 'yes', 'yes', 'no'] },
  ],
}

/* ---- Principles ---- */
export const PRINCIPLES = [
  { h: 'Rootless Podman', p: 'Containers run rootless under your own systemd user, with no daemon behind them.' },
  { h: 'Runs as you', p: 'Installs to ~/.local/bin. DNS, TLS and ports are set up without touching system files.' },
  { h: 'Detects your project', p: 'Frameworks are detected from the repo. An optional .lerd.yaml adds services or workers.' },
  { h: 'Updates within a day', p: 'Frameworks and services are versioned definitions that arrive without a release.' },
  { h: 'Open source, MIT', p: 'All of it is free and in one MIT-licensed binary, with no paid tier and no account.' },
  { h: 'Stays in your home', p: 'Everything lives under your home directory, so uninstalling leaves the machine as it was.' },
]

// Answers stay plain text; a question that needs a page links out through href.
export const FAQ = [
  { q: 'Is Lerd free?', a: 'Yes. Lerd is MIT licensed and all of it is in the one binary, with no paid tier and no account.' },
  { q: 'Do I need Docker?', a: 'No. Lerd runs everything as rootless Podman containers under your own user, with no daemon. The installer offers to install Podman if it is missing.' },
  { q: 'Does it work on Windows?', a: 'Yes, inside WSL2. The Windows installer sets up WSL2, an Ubuntu distro with your Linux user, Lerd and the Lerd desktop app.', href: '/getting-started/wsl2', link: 'Windows (WSL2) guide' },
  { q: 'Do I have to change my project?', a: 'No. Lerd detects the framework from the repository and serves it as it is. An optional .lerd.yaml pins PHP and Node versions, services and workers.' },
  { q: 'Can it run next to a MySQL or PostgreSQL I already have?', a: 'Yes. When a database installed on the host already holds the default port, Lerd publishes its container on the next free port and writes that into the site .env.' },
  { q: 'Does Lerd work with Claude Code or Cursor?', a: "Yes. Run lerd mcp:enable-global and Lerd's MCP server is registered with Claude Code, Cursor, Codex CLI, Gemini CLI, GitHub Copilot in VS Code, Junie, Antigravity, Windsurf and OpenCode. The assistant can then manage sites, services, workers and databases, and run artisan and composer.", href: '/features/mcp', link: 'Set up MCP' },
  { q: 'Can I extend it?', a: 'Yes, with community plugins that add commands, dashboard pages, AI tools and services. Each one is reviewed at the exact commit Lerd installs.', href: 'https://plugins.lerd.sh/', link: 'Browse community plugins' },
  { q: 'How do I remove it?', a: 'Run lerd uninstall. It stops the containers and removes the units, the binary and the network, then asks whether to remove your data and the trusted certificate as well.', href: '/getting-started/installation#uninstall', link: 'What uninstall removes' },
]
