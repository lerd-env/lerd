/* ===========================================================
   Lerd landing page, shared data
   Ported from the standalone landing mockup (data.js).
   Install commands use the canonical lerd.sh endpoints.
   =========================================================== */

/* ---- Brand glyphs: lettered tiles (no fake brand logos) ---- */
export const LOGOS = {
  laravel:  { ch: 'L', c: '#ff2d20' },
  symfony:  { ch: 'S', c: '#cbd5e1' },
  wordpress:{ ch: 'W', c: '#38bdf8' },
  drupal:   { ch: 'D', c: '#60a5fa' },
  cake:     { ch: 'C', c: '#f87171' },
  statamic: { ch: 'S', c: '#a78bfa' },
  codeigniter: { ch: 'C', c: '#f97316' },
  tempest:  { ch: 'T', c: '#22d3ee' },
  claude:   { ch: 'C', c: '#ff8a65' },
  cursor:   { ch: '⌘', c: '#e5e7eb' },
  codex:    { ch: '{', c: '#34d399' },
  gemini:   { ch: 'G', c: '#60a5fa' },
  copilot:  { ch: '⊙', c: '#cbd5e1' },
  junie:    { ch: 'J', c: '#a78bfa' },
  windsurf: { ch: '≈', c: '#38bdf8' },
  mysql:    { ch: 'M', c: '#38bdf8' },
  postgres: { ch: 'P', c: '#60a5fa' },
  redis:    { ch: 'R', c: '#ff5b50' },
  meili:    { ch: 'M', c: '#fb7185' },
  rustfs:   { ch: 'S', c: '#fbbf24' },
  mailpit:  { ch: '@', c: '#34d399' },
  mongo:    { ch: 'M', c: '#34d399' },
  stripe:   { ch: '$', c: '#a78bfa' },
  antigravity: { ch: 'A', c: '#60a5fa' },
}

export function glyph(name, size) {
  const l = LOGOS[name] || { ch: '?', c: '#888' }
  const s = size || 100
  return `<svg viewBox="0 0 ${s} ${s}" xmlns="http://www.w3.org/2000/svg" style="width:100%;height:100%">
      <rect width="${s}" height="${s}" rx="${s * 0.26}" fill="${l.c}" fill-opacity="0.13" stroke="${l.c}" stroke-opacity="0.4"/>
      <text x="50%" y="54%" dominant-baseline="middle" text-anchor="middle"
        font-family="JetBrains Mono, monospace" font-weight="600" font-size="${s * 0.5}" fill="${l.c}">${l.ch}</text>
    </svg>`
}

/* ---- Install commands per OS (canonical lerd.sh) ---- */
export const INSTALL = {
  linux: 'curl -fsSL https://lerd.sh/install.sh | bash',
  macos: 'curl -fsSL https://lerd.sh/install.sh | bash',
  wsl:   'wsl curl -fsSL https://lerd.sh/install.sh | bash',
}

/* ---- Comparison ---- */
export const CMP = {
  cols: ['Lerd', 'Laravel Herd', 'DDEV', 'Lando', 'Sail'],
  rows: [
    { f: 'Runs on Linux',            v: ['yes', 'no', 'yes', 'yes', 'yes'] },
    { f: 'No Docker daemon',         v: ['yes', 'yes', 'no', 'no', 'no'] },
    { f: 'Rootless · no sudo',       v: ['yes', 'yes', 'partial', 'partial', 'no'] },
    { f: 'Open source',              v: ['yes', 'no', 'yes', 'yes', 'yes'] },
    { f: 'Automatic .test + TLS',    v: ['yes', 'yes', 'yes', 'partial', 'no'] },
    { f: 'Zero per-project config',  v: ['yes', 'yes', 'no', 'no', 'no'] },
    { f: 'Built-in Web UI',          v: ['yes', 'yes', 'partial', 'no', 'no'] },
    { f: 'MCP server for AI agents', v: ['yes', 'yes', 'no', 'no', 'no'] },
    { f: 'Profiler & debug window',  v: ['yes', 'yes', 'no', 'no', 'no'] },
  ],
}

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
]

/* ---- Quick-start steps ---- */
export const STEPS = [
  {
    n: '01', title: 'Install Lerd',
    desc: 'One script. Rootless Podman, systemd user units, the CLI and Web UI, no sudo.',
    file: '~',
  },
  {
    n: '02', title: 'Link your project',
    desc: 'cd into any PHP repo and run lerd link. It routes through the init wizard, installs dependencies, runs migrations, starts your workers and provisions TLS, live at project.test. Then lerd open to launch it, or wire up MCP.',
    file: '~/code/acme',
  },
]
