// A theme is the handful of chrome tones the dashboard paints itself with. They
// are the same custom properties Tailwind compiles the lerd-* utilities from, so
// setting them on the root element retints every button, tab and focus ring at
// once without a single class changing.
//
// A user theme arrives from ~/.config/lerd/themes as a name and an accent, with
// everything else optional. The missing tones are derived here rather than
// demanded of the author: the hover shades are the accent stepped toward black
// or white, the dark accent is the light one lifted until it reads on the dark
// card, and the surfaces fall back to the built-in ones.

import { brandTint, mix, parseHex, toHex } from './brandTint';

export interface Palette {
  id: string;
  name: string;
  accent: string;
  accentHover: string;
  accentDark: string;
  accentHoverDark: string;
  bg: string;
  card: string;
  border: string;
  muted: string;
  source: 'builtin' | 'user';
}

// PaletteFile is a theme as it comes off disk, in the snake_case the YAML uses.
export interface PaletteFile {
  id: string;
  name: string;
  accent: string;
  accent_hover?: string;
  accent_dark?: string;
  accent_hover_dark?: string;
  bg?: string;
  card?: string;
  border?: string;
  muted?: string;
}

export interface PaletteError {
  file: string;
  error: string;
}

export const DEFAULT_PALETTE_ID = 'lerd';

// How far a hover tone moves off its accent, matching the step between the
// built-in #ff2d20 and its #e02419 hover.
const HOVER_STEP = 0.12;

export const BUILTIN_PALETTES: Palette[] = [
  {
    id: 'lerd',
    name: 'lerd',
    accent: '#ff2d20',
    accentHover: '#e02419',
    accentDark: '#ff2d20',
    accentHoverDark: '#e02419',
    bg: '#0d0d0d',
    card: '#161616',
    border: '#262626',
    muted: '#404040',
    source: 'builtin'
  },
  {
    id: 'muted',
    name: 'muted',
    accent: '#b04a42',
    accentHover: '#963e37',
    accentDark: '#d98d84',
    accentHoverDark: '#e6a49c',
    bg: '#111113',
    card: '#1a1a1c',
    border: '#2a2a2d',
    muted: '#45454a',
    source: 'builtin'
  },
  {
    id: 'solarized',
    name: 'Solarized Dark',
    accent: '#1f6f9a',
    accentHover: '#1a5f84',
    accentDark: '#4aa3dd',
    accentHoverDark: '#6bb5e5',
    bg: '#002b36',
    card: '#073642',
    border: '#0f4653',
    muted: '#586e75',
    source: 'builtin'
  },
  {
    id: 'monokai',
    name: 'Monokai',
    accent: '#c9155c',
    accentHover: '#a91050',
    accentDark: '#f92672',
    accentHoverDark: '#fa4b8c',
    bg: '#272822',
    card: '#31322c',
    border: '#3e3d32',
    muted: '#75715e',
    source: 'builtin'
  },
  {
    id: 'cobalt',
    name: 'Cobalt',
    accent: '#9a6000',
    accentHover: '#7f4f00',
    accentDark: '#ff9d00',
    accentHoverDark: '#ffb133',
    bg: '#193549',
    card: '#1f4662',
    border: '#27536f',
    muted: '#4b6b84',
    source: 'builtin'
  },
  {
    id: 'dracula',
    name: 'Dracula',
    accent: '#7a3fd4',
    accentHover: '#6733b6',
    accentDark: '#bd93f9',
    accentHoverDark: '#cdaafb',
    bg: '#282a36',
    card: '#343746',
    border: '#44475a',
    muted: '#6272a4',
    source: 'builtin'
  },
  {
    id: 'nord',
    name: 'Nord',
    accent: '#446b8a',
    accentHover: '#3a5b76',
    accentDark: '#88c0d0',
    accentHoverDark: '#9fcfdc',
    bg: '#2e3440',
    card: '#3b4252',
    border: '#434c5e',
    muted: '#4c566a',
    source: 'builtin'
  },
  {
    id: 'gruvbox',
    name: 'Gruvbox Dark',
    accent: '#af3a03',
    accentHover: '#973203',
    accentDark: '#fe8019',
    accentHoverDark: '#fe9a4a',
    bg: '#282828',
    card: '#32302f',
    border: '#3c3836',
    muted: '#928374',
    source: 'builtin'
  },
  {
    id: 'breeze',
    name: 'Breeze',
    accent: '#17698f',
    accentHover: '#12556f',
    accentDark: '#3daee9',
    accentHoverDark: '#5fbdee',
    bg: '#232629',
    card: '#31363b',
    border: '#3f454b',
    muted: '#7f8c8d',
    source: 'builtin'
  },
  {
    id: 'adwaita',
    name: 'Adwaita',
    accent: '#1a5fb4',
    accentHover: '#164e94',
    accentDark: '#3584e4',
    accentHoverDark: '#5195e8',
    bg: '#1e1e1e',
    card: '#303030',
    border: '#3d3d3d',
    muted: '#77767b',
    source: 'builtin'
  },
  {
    id: 'macos',
    name: 'macOS',
    accent: '#0057b8',
    accentHover: '#00489b',
    accentDark: '#0a84ff',
    accentHoverDark: '#3d9dff',
    bg: '#1e1e1e',
    card: '#2c2c2e',
    border: '#38383a',
    muted: '#98989d',
    source: 'builtin'
  }
];

const DEFAULT_PALETTE = BUILTIN_PALETTES[0];

// resolvePalette fills a theme file out into the full set of tones, or returns
// null when the accent is not a plain hex. The daemon already refuses anything
// else; this is the second gate, right before the value becomes CSS.
export function resolvePalette(file: PaletteFile): Palette | null {
  const accent = hex(file.accent);
  if (!accent || !file.id || !file.name) return null;
  const accentDark = hex(file.accent_dark) || brandTint(accent)!.dark;
  return {
    id: file.id,
    name: file.name,
    accent,
    accentHover: hex(file.accent_hover) || step(accent, 0),
    accentDark,
    accentHoverDark: hex(file.accent_hover_dark) || step(accentDark, 255),
    bg: hex(file.bg) || DEFAULT_PALETTE.bg,
    card: hex(file.card) || DEFAULT_PALETTE.card,
    border: hex(file.border) || DEFAULT_PALETTE.border,
    muted: hex(file.muted) || DEFAULT_PALETTE.muted,
    source: 'user'
  };
}

// paletteById returns the named theme, falling back to the default one. A theme
// whose file the user deleted leaves its id behind in localStorage, and the
// dashboard has to keep drawing.
export function paletteById(palettes: Palette[], id: string): Palette {
  return palettes.find((p) => p.id === id) || DEFAULT_PALETTE;
}

// paletteVars maps a theme onto the custom properties app.css declares, picking
// the tone that reads on the surface the current mode paints.
export function paletteVars(palette: Palette, dark: boolean): Record<string, string> {
  return {
    '--lerd-accent': dark ? palette.accentDark : palette.accent,
    '--lerd-accent-hover': dark ? palette.accentHoverDark : palette.accentHover,
    '--lerd-bg': palette.bg,
    '--lerd-card': palette.card,
    '--lerd-border': palette.border,
    '--lerd-muted': palette.muted
  };
}

function hex(v: string | undefined): string | null {
  const rgb = parseHex(v);
  return rgb ? toHex(rgb) : null;
}

function step(color: string, target: number): string {
  return toHex(mix(parseHex(color)!, target, HOVER_STEP));
}
