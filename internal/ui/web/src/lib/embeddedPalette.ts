import { embeddedSurfaces } from './embeddedTheme';
import { parseHex, toHex } from './brandTint';
import { type Palette } from './palettePaint';

// A dashboard with a dark design of its own is brought onto the theme's by
// laying its scale over lerd's: its darkest grey is the page, its lightest is
// the tone text reads in, and the steps between land in the order it drew them.
// Only the scale is the dashboard's own business, so each one names it and the
// arithmetic lives here rather than three times over.

// The tone text reads in on a dark surface. The palette carries surfaces and an
// accent, not a text colour, so one is named here for all of them.
export const DARK_TEXT = '#e5e7eb';

// A design's own colours: the greys it stacks, which of them are the panels it
// puts on the page, and whatever it uses to draw attention.
export interface DesignRamp {
  // Darkest first, the page at one end and its text tone at the other.
  greys: string[];
  // Greys that stand for a panel rather than a step, so they take the theme's
  // card outright and go on reading as a panel.
  cards?: string[];
  accents?: string[];
  accentHovers?: string[];
}

type Rgb = [number, number, number];

function lerp(from: Rgb, to: Rgb, amount: number): string {
  return toHex(from.map((c, i) => Math.round(c + (to[i] - c) * amount)) as Rgb);
}

// rampPalette maps one design's colours onto the theme's, ready for the sweep.
// White is deliberately absent: it is the label on a filled button, and dimming
// it only costs contrast against a fill the theme has already chosen.
export function rampPalette(design: DesignRamp, dark: boolean): Palette {
  const s = embeddedSurfaces(dark);
  const page = parseHex(s.bg)!;
  const text = parseHex(DARK_TEXT)!;
  const out: Palette = {};
  design.greys.forEach((grey, i) => {
    out[grey] = lerp(page, text, i / Math.max(design.greys.length - 1, 1));
  });
  for (const card of design.cards ?? []) out[card] = s.card;
  for (const accent of design.accents ?? []) out[accent] = s.accent;
  for (const hover of design.accentHovers ?? []) out[hover] = s.accentHover;
  return out;
}
