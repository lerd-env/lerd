// Renders the per-version social card behind each dev digest's og:image.
// Reads the tagline and date straight out of the digest page so the card can
// never drift from what it links to. Requires rsvg-convert (librsvg).
//
//   node docs/scripts/digest-card.mjs            # every digest
//   node docs/scripts/digest-card.mjs v1.30.0    # just one

import { execFileSync } from 'node:child_process'
import { mkdirSync, readFileSync, readdirSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { fileURLToPath } from 'node:url'
import path from 'node:path'

const DIGEST_DIR = fileURLToPath(new URL('../public/digest', import.meta.url))
const OUT_DIR = fileURLToPath(new URL('../public/assets/digest', import.meta.url))

const BG = '#0a0a0b'
const RED = '#ff4538'
const TEXT = '#ece9e2'
const MUTED = '#9b978e'
const DIM = '#67645c'
const LINE = '#2e2e36'

const SANS = 'Fira Sans Compressed, Archivo, DejaVu Sans, sans-serif'
const MONO = 'JetBrains Mono, DejaVu Sans Mono, monospace'

const esc = (s) =>
  s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;')

// Wraps on width estimated from character count, since we cannot measure text
// before handing the SVG to librsvg.
function wrap(text, perLine) {
  const words = text.split(/\s+/)
  const lines = []
  let line = ''
  for (const w of words) {
    if (line && (line + ' ' + w).length > perLine) {
      lines.push(line)
      line = w
    } else {
      line = line ? line + ' ' + w : w
    }
  }
  if (line) lines.push(line)
  return lines
}

function read(version) {
  const html = readFileSync(path.join(DIGEST_DIR, `${version}.html`), 'utf8')
  const sub = html.match(/<div class="sub rise d2">([\s\S]*?)<\/div>/)
  const date = html.match(/lerd v[\d.]+ · ([\d-]+)/)
  const desc = html.match(/<meta name="description" content="([^"]*)"/)

  // The sub line is "TAGLINE · <b>headline</b>"; the tagline alone is the card's.
  const raw = sub ? sub[1].replace(/<[^>]+>/g, '').trim() : ''
  const tagline = raw.split('·').map((s) => s.trim()).filter(Boolean).join(' · ')

  let summary = desc ? desc[1] : ''
  summary = summary.replace(/^Engineering digest for lerd v[\d.]+:\s*/i, '')
  summary = summary.charAt(0).toUpperCase() + summary.slice(1)

  return { tagline, date: date ? date[1] : '', summary }
}

function svg(version, { tagline, date, summary }) {
  const [maj, min, patch] = version.replace(/^v/, '').split('.')
  const taglines = wrap(tagline.toUpperCase(), 46).slice(0, 2)
  const summaries = wrap(summary.replace(/\.$/, ''), 62)
  if (summaries.length > 3) {
    summaries.length = 3
    summaries[2] = summaries[2].replace(/[,;:]?$/, '') + '…'
  }

  // Bottom-align the block so any combination of line counts clears the footer.
  const sumY = 546 - (summaries.length - 1) * 32
  const tagY = sumY - 30 - (taglines.length - 1) * 34

  return `<svg xmlns="http://www.w3.org/2000/svg" width="1280" height="640" viewBox="0 0 1280 640">
  <defs>
    <radialGradient id="glow" cx="14%" cy="0%" r="70%">
      <stop offset="0%" stop-color="${RED}" stop-opacity="0.20"/>
      <stop offset="100%" stop-color="${RED}" stop-opacity="0"/>
    </radialGradient>
    <radialGradient id="glow2" cx="96%" cy="100%" r="55%">
      <stop offset="0%" stop-color="#eda33b" stop-opacity="0.09"/>
      <stop offset="100%" stop-color="#eda33b" stop-opacity="0"/>
    </radialGradient>
  </defs>

  <rect width="1280" height="640" fill="${BG}"/>
  <rect width="1280" height="640" fill="url(#glow)"/>
  <rect width="1280" height="640" fill="url(#glow2)"/>

  <rect x="80" y="72" width="52" height="52" rx="12" fill="${RED}"/>
  <text x="106" y="109" font-family="${MONO}" font-size="30" font-weight="700"
        fill="#ffffff" text-anchor="middle">L</text>
  <text x="150" y="99" font-family="${SANS}" font-size="27" font-weight="700"
        fill="${TEXT}">lerd</text>
  <text x="205" y="99" font-family="${MONO}" font-size="18"
        fill="${DIM}">lerd-env/<tspan fill="${RED}">lerd</tspan></text>

  <text x="1200" y="99" font-family="${MONO}" font-size="17" fill="${DIM}"
        text-anchor="end">${esc(date)}</text>

  <text x="80" y="290" font-family="${SANS}" font-size="182" font-weight="800"
        fill="${TEXT}" letter-spacing="-6">v${maj}.<tspan fill="${RED}">${min}</tspan>.${patch}</text>

  <rect x="80" y="336" width="1120" height="1" fill="${LINE}"/>

  ${taglines
    .map(
      (l, i) =>
        `<text x="80" y="${tagY + i * 34}" font-family="${SANS}" font-size="27" font-weight="700"
        letter-spacing="1.6" fill="${RED}">${esc(l)}</text>`,
    )
    .join('\n  ')}

  ${summaries
    .map(
      (l, i) =>
        `<text x="80" y="${sumY + i * 32}" font-family="${SANS}" font-size="24"
        fill="${MUTED}">${esc(l)}</text>`,
    )
    .join('\n  ')}

  <text x="80" y="580" font-family="${MONO}" font-size="17" letter-spacing="2.4"
        fill="${DIM}">ENGINEERING DIGEST</text>
  <text x="1200" y="580" font-family="${MONO}" font-size="17" fill="${DIM}"
        text-anchor="end">lerd.sh</text>

  <rect x="0" y="628" width="590" height="12" fill="${RED}"/>
  <rect x="590" y="628" width="154" height="12" fill="#eda33b"/>
  <rect x="744" y="628" width="90" height="12" fill="#3dd68c"/>
  <rect x="834" y="628" width="64" height="12" fill="#6cb2ff"/>
  <rect x="898" y="628" width="51" height="12" fill="#b394ff"/>
  <rect x="949" y="628" width="331" height="12" fill="${LINE}"/>
</svg>`
}

mkdirSync(OUT_DIR, { recursive: true })

const only = process.argv[2]
const versions = only
  ? [only.replace(/\.html$/, '')]
  : readdirSync(DIGEST_DIR)
      .filter((f) => f.endsWith('.html'))
      .map((f) => f.replace(/\.html$/, ''))
      .sort()

for (const version of versions) {
  const meta = read(version)
  // The SVG is an intermediate, so it stays out of public/ and off the site.
  const svgPath = path.join(tmpdir(), `lerd-digest-${version}.svg`)
  const pngPath = path.join(OUT_DIR, `${version}.png`)
  writeFileSync(svgPath, svg(version, meta))
  execFileSync('rsvg-convert', ['-w', '1280', '-h', '640', '-o', pngPath, svgPath])
  rmSync(svgPath, { force: true })
  console.log(`${version}  ${meta.date}  ${meta.tagline}`)
}
