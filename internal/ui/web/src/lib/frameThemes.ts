import { rampPalette, DARK_TEXT, type DesignRamp } from './embeddedPalette';
import { embeddedSurfaces } from './embeddedTheme';
import { palettePairs, repaintPalette, watchPaletteRules } from './palettePaint';

// The dashboards that have a dark design of their own and take it as soon as
// lerd answers their question about the colour scheme. Dark is not themed: left
// there they wear their own greys and their own accent inside an overlay wearing
// lerd's, so each one's scale is named here and laid over the theme's.
//
// Which scale belongs to which dashboard is all that differs; the sweep that
// writes it back is shared (palettePaint), as is the arithmetic (embeddedPalette).
const DESIGNS: Record<string, DesignRamp> = {
  // pgAdmin: its own greys, the navy its menu bar and buttons carry, and the
  // blue its icons are drawn in. PostgreSQL's own blue is not here on purpose,
  // being the elephant, and a logo keeps the colour it is.
  pgadmin: {
    greys: [
      '#1e1e1e',
      '#282828',
      '#333333',
      '#424242',
      '#4a4a4a',
      '#616161',
      '#6b6b6b',
      '#8a8a8a',
      '#d4d4d4'
    ],
    cards: ['#282828', '#333333'],
    accents: ['#234d6e', '#1b71b5', '#6cb4ee', '#7dc9f1', '#88c7f4'],
    accentHovers: ['#323e43']
  },
  // Kafbat UI: a clean scale and almost nothing else. The two brand colours it
  // draws its Discord and Product Hunt marks in are left where they are.
  'kafka-ui': {
    greys: [
      '#0b0d0e',
      '#171a1c',
      '#22282a',
      '#2f3639',
      '#394246',
      '#454f54',
      '#5c6970',
      '#73848c',
      '#8f9ca3',
      '#abb5ba',
      '#c7ced1',
      '#e3e6e8'
    ],
    cards: ['#22282a', '#2f3639']
  },
  // RedisInsight: its greys and the blue it carries every link and primary
  // action in. Its teal, amber and red say success, warning and danger, so they
  // keep saying it.
  redisinsight: {
    // Its surfaces are compiled rather than kept in variables, so these are the
    // values it actually paints with, read back off the rendered page.
    greys: [
      '#121212',
      '#1a1a1a',
      '#1a1c21',
      '#343741',
      '#69707d',
      '#6a717d',
      '#98a2b3',
      '#c2c3c6',
      '#cacaca',
      '#d1d5db',
      '#d3dae6',
      '#dbdbdb',
      '#f5f7fa',
      '#f6f6f6',
      '#fbfcfd'
    ],
    cards: ['#1a1a1a', '#1a1c21', '#343741'],
    accents: ['#006bb4'],
    accentHovers: ['#01645c']
  }
};

const STYLE_ID = 'lerd-frame-theme';

export function hasFrameDesign(name: string | undefined | null): boolean {
  return !!name && name in DESIGNS;
}

function pairsFor(name: string, dark: boolean) {
  return palettePairs(rampPalette(DESIGNS[name], dark));
}

// repaintFrameDesign rewrites the dashboard's palette wherever its own styles
// wrote it. Light mode is its own design and is left alone.
export function repaintFrameDesign(doc: Document, name: string, dark: boolean): void {
  if (!dark || !DESIGNS[name]) return;
  repaintPalette(doc, pairsFor(name, dark));
}

// watchFrameDesign catches the rules an app adds as panels and dialogs mount,
// which arrive through the stylesheet long after the sweep has run.
export function watchFrameDesign(
  win: Window & typeof globalThis,
  name: string,
  dark: () => boolean
): void {
  if (!DESIGNS[name]) return;
  watchPaletteRules(win, () => pairsFor(name, dark()));
}

// themeFrameDocument paints what the sweep cannot reach: the page behind the
// app, and the scrollbars the browser draws for it.
export function themeFrameDocument(doc: Document, dark: boolean): void {
  let style = doc.getElementById(STYLE_ID) as HTMLStyleElement | null;
  if (!style) {
    style = doc.createElement('style');
    style.id = STYLE_ID;
    doc.head?.appendChild(style);
  }
  const s = embeddedSurfaces(dark);
  style.textContent = dark
    ? `html, body { background: ${s.bg}; color: ${DARK_TEXT}; }
html { color-scheme: dark; }`
    : '';
}
