// A preset declares its brand colour in the store, so the value cannot become a
// Tailwind class: those are full static strings picked from a fixed record and a
// class assembled at runtime never reaches the stylesheet. It arrives instead as
// a custom property on the element, and app.css derives the badge background
// from it at low alpha with the glyph taking the solid tone.
//
// The declared colour is used as given only when it reads on the card it lands
// on. A near-black brand vanishes on the dark card and a near-white one on the
// light card, so each theme gets its own tone, nudged toward the card until it
// separates from it. Both are computed here and handed over together; CSS then
// picks per theme, which keeps a theme flip a pure CSS switch.

export interface BrandTint {
  light: string;
  dark: string;
}

// Relative luminance bounds a tone has to clear to read against the card it sits
// on: the light card is white-ish, the dark card is #161616.
const MAX_ON_LIGHT = 0.5;
const MIN_ON_DARK = 0.16;

const STEP = 0.08;
const MAX_STEPS = 12;

// brandTint turns a declared hex colour into the pair of tones the dashboard
// paints with, or null when the value is not a plain hex. The daemon already
// drops anything else; this is the second gate, right before the value becomes
// CSS.
export function brandTint(color: string | undefined | null): BrandTint | null {
  const rgb = parseHex(color);
  if (!rgb) return null;
  return {
    light: toHex(darkenUntil(rgb, MAX_ON_LIGHT)),
    dark: toHex(lightenUntil(rgb, MIN_ON_DARK))
  };
}

// brandTintStyle is brandTint as an inline style declaration, empty when there
// is no usable colour so the caller falls back to its category tint.
export function brandTintStyle(color: string | undefined | null): string {
  const t = brandTint(color);
  return t ? `--mark-tint:${t.light};--mark-tint-dark:${t.dark}` : '';
}

export function parseHex(color: string | undefined | null): [number, number, number] | null {
  const v = (color || '').trim().toLowerCase();
  if (!/^#([0-9a-f]{3}|[0-9a-f]{6})$/.test(v)) return null;
  const full =
    v.length === 4 ? '#' + v[1] + v[1] + v[2] + v[2] + v[3] + v[3] : v;
  return [
    parseInt(full.slice(1, 3), 16),
    parseInt(full.slice(3, 5), 16),
    parseInt(full.slice(5, 7), 16)
  ];
}

export function toHex(rgb: [number, number, number]): string {
  return '#' + rgb.map((c) => Math.round(c).toString(16).padStart(2, '0')).join('');
}

// WCAG relative luminance, the same measure the contrast ratio is built on.
export function luminance(rgb: [number, number, number]): number {
  const [r, g, b] = rgb.map((c) => {
    const s = c / 255;
    return s <= 0.03928 ? s / 12.92 : Math.pow((s + 0.055) / 1.055, 2.4);
  });
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

export function mix(rgb: [number, number, number], target: number, amount: number): [number, number, number] {
  return rgb.map((c) => c + (target - c) * amount) as [number, number, number];
}

function darkenUntil(rgb: [number, number, number], max: number): [number, number, number] {
  let out = rgb;
  for (let i = 0; i < MAX_STEPS && luminance(out) > max; i++) out = mix(out, 0, STEP);
  return out;
}

function lightenUntil(rgb: [number, number, number], min: number): [number, number, number] {
  let out = rgb;
  for (let i = 0; i < MAX_STEPS && luminance(out) < min; i++) out = mix(out, 255, STEP);
  return out;
}
