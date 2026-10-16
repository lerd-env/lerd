// Renders the per-version social card behind each dev digest's og:image.
// Reads the tagline and date straight out of the digest page so the card can
// never drift from what it links to. Requires rsvg-convert (librsvg) and the
// Instrument Sans and JetBrains Mono fonts installed, which it draws with.
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

// The landing page's editorial palette and type.
const BG = '#0a0a0b'
const RED = '#ff2d20'
const TEXT = '#f4f4f5'
const INK2 = '#c4c4cc'
const MUTED = '#8b8b95'
const FAINT = '#5d5d66'
const LINE = '#26262c'

const SANS = 'Instrument Sans, sans-serif'
const MONO = 'JetBrains Mono, monospace'

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
  const taglines = wrap(tagline, 50).slice(0, 2)
  const summaries = wrap(summary.replace(/\.$/, ''), 78)
  if (summaries.length > 3) {
    summaries.length = 3
    summaries[2] = summaries[2].replace(/[,;:]?$/, '') + '…'
  }

  // The summary follows however many tagline lines there are.
  const tagY = 392
  const sumY = tagY + taglines.length * 46 + 22

  return `<svg xmlns="http://www.w3.org/2000/svg" width="1280" height="640" viewBox="0 0 1280 640">
  <rect width="1280" height="640" fill="${BG}"/>

  <rect x="80" y="72" width="40" height="40" rx="9" fill="${RED}"/>
  <text x="100" y="101" font-family="${SANS}" font-size="25" font-weight="700"
        fill="#ffffff" text-anchor="middle">L</text>
  <text x="138" y="98" font-family="${MONO}" font-size="16" letter-spacing="1.6"
        fill="${MUTED}">ENGINEERING DIGEST · ${esc(date)}</text>
  <text x="1200" y="98" font-family="${MONO}" font-size="16" fill="${MUTED}"
        text-anchor="end">lerd.sh</text>

  <text x="72" y="296" font-family="${SANS}" font-size="188" font-weight="700"
        fill="${TEXT}" letter-spacing="0">v${maj}.<tspan fill="${RED}">${min}</tspan>.${patch}</text>

  ${taglines
    .map(
      (l, i) =>
        `<text x="80" y="${tagY + i * 46}" font-family="${SANS}" font-size="40" font-weight="600"
        letter-spacing="-0.8" fill="${FAINT}">${esc(l)}</text>`,
    )
    .join('\n  ')}

  ${summaries
    .map(
      (l, i) =>
        `<text x="80" y="${sumY + i * 32}" font-family="${SANS}" font-size="23"
        fill="${INK2}">${esc(l)}</text>`,
    )
    .join('\n  ')}

  <rect x="80" y="${Math.min(sumY + summaries.length * 32 + 8, 600)}" width="1120" height="1" fill="${LINE}"/>
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
