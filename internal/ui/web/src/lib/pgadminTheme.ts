import { embeddedSurfaces } from './embeddedTheme';
import { parseHex, toHex } from './brandTint';
import { palettePairs, repaintPalette, watchPaletteRules, type Palette } from './palettePaint';

// pgAdmin has a dark design of its own and takes it the moment lerd answers its
// question about the colour scheme, which is what the proxy's script is for. The
// design it takes is pgAdmin's though, its own greys and its own blue, so the
// view sits in the dashboard wearing a different theme from everything around it.
//
// Its palette is built in JavaScript, so there is neither a switch to move nor a
// variable to set, but what MUI builds it writes into stylesheets on the page,
// and those are the same origin. So the colours are mapped onto the theme's and
// written back where it wrote them, the way Meilisearch's are.
const THEME_STYLE_ID = 'lerd-pgadmin-theme';

// pgAdmin's dark greys, darkest first: the page at one end, the tone it writes
// text in at the other.
const PG_GREYS = [
  '#1e1e1e',
  '#282828',
  '#333333',
  '#424242',
  '#4a4a4a',
  '#616161',
  '#6b6b6b',
  '#8a8a8a',
  '#d4d4d4'
];
// The panels it stacks on the page. Second and third in its scale, near enough
// to the page to read as one surface once the theme's own card sits there.
//
// Its white stays white: it is the label on a filled button or a menu bar, and
// dimming it only costs contrast against a fill the theme has already chosen.
const PG_CARDS = ['#282828', '#333333'];
// Its primary, the navy the menu bar and the buttons carry, the brand blue its
// icons are drawn in, and the lighter blues its links and hovers use.
//
// PostgreSQL's own blue is not in here on purpose: it is the elephant, and a
// logo keeps the colour it is.
const PG_PRIMARY = '#234d6e';
const PG_PRIMARY_LIGHT = '#323e43';
const PG_BRAND = '#1b71b5';
const PG_LINK = '#6cb4ee';
const PG_LINK_LIGHT = '#7dc9f1';
const PG_LINK_PALE = '#88c7f4';

// The tone text reads in on a dark surface. The palette carries surfaces and an
// accent, not a text colour, so the dashboard's own is repeated here.
const DARK_TEXT = '#e5e7eb';

type Rgb = [number, number, number];

function lerp(from: Rgb, to: Rgb, amount: number): string {
  return toHex(from.map((c, i) => Math.round(c + (to[i] - c) * amount)) as Rgb);
}

// pgadminPalette maps every colour pgAdmin paints its dark design with onto the
// theme's, in the order its own scale runs.
export function pgadminPalette(dark: boolean): Palette {
  const s = embeddedSurfaces(dark);
  const page = parseHex(s.bg)!;
  const text = parseHex(DARK_TEXT)!;
  const out: Palette = {};
  PG_GREYS.forEach((grey, i) => {
    out[grey] = lerp(page, text, i / (PG_GREYS.length - 1));
  });
  // The panels take the theme's card outright, so they keep standing off the
  // page rather than landing a step along the scale.
  for (const card of PG_CARDS) out[card] = s.card;
  out[PG_PRIMARY] = s.accent;
  out[PG_PRIMARY_LIGHT] = s.accentHover;
  out[PG_BRAND] = s.accent;
  out[PG_LINK] = s.accent;
  out[PG_LINK_LIGHT] = s.accent;
  out[PG_LINK_PALE] = s.accent;
  return out;
}

// repaintPgadmin rewrites its palette wherever MUI wrote it.
export function repaintPgadmin(doc: Document, dark: boolean): void {
  if (!dark) return; // Light mode is pgAdmin's own, and the theme's greys are its.
  repaintPalette(doc, palettePairs(pgadminPalette(dark)));
}

// watchPgadminRules catches the rules MUI adds as panels and dialogs mount,
// which arrive through the stylesheet long after the sweep has run.
export function watchPgadminRules(win: Window & typeof globalThis, dark: () => boolean): void {
  watchPaletteRules(win, () => palettePairs(pgadminPalette(dark())));
}

// themePgadminDocument paints what the sweep cannot reach: the page behind the
// app, and the scrollbars the browser draws for it.
export function themePgadminDocument(doc: Document, dark: boolean): void {
  let style = doc.getElementById(THEME_STYLE_ID) as HTMLStyleElement | null;
  if (!style) {
    style = doc.createElement('style');
    style.id = THEME_STYLE_ID;
    doc.head?.appendChild(style);
  }
  const s = embeddedSurfaces(dark);
  style.textContent = dark
    ? `html, body { background: ${s.bg}; }
html { color-scheme: dark; }`
    : '';
}
