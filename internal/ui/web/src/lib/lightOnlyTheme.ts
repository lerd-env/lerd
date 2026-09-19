import { embeddedSurfaces } from './embeddedTheme';
import { parseHex, toHex, luminance, mix } from './brandTint';

// Some dashboards ship one design and it is a light one. phpMyAdmin is the case
// in point: none of its four bundled themes carries a dark mode, so there is no
// switch to move and no variable to set, and what the overlay flips elsewhere
// finds nothing to flip. What there is, the page being same-origin, is the
// stylesheet itself.
//
// So the greys are read off it and turned around, in order: what the design used
// for its page becomes the page, what it used for text becomes the tone text
// reads in, and every step between lands where it fell. A colour carrying a hue
// keeps it, since the hue is the thing saying it, but it is moved until it reads
// on a dark page, and a pale wash behind a notice is deepened rather than left
// glowing.
//
// Inverting the whole page with a filter was the other way, and it cannot land
// on a theme: a tinted surface comes out grey, shadows come out as glows, and
// every icon has to be inverted back.
const THEME_STYLE_ID = 'lerd-light-only-theme';

// The tone text reads in on a dark surface. The palette carries surfaces and an
// accent, not a text colour, so the dashboard's own is repeated here.
const DARK_TEXT = '#e5e7eb';

// The light end of the scale the design's greys are laid back onto. It is whiter
// than the tone body text reads in, on purpose: a design's own steps have to
// keep the distance it left between them, and a narrower scale squeezes a pair
// that was comfortable in the original into one that is not.
const RAMP_LIGHT = '#ffffff';

// How far a colour's channels may spread apart and still count as a grey. A
// design's off-whites and near-blacks are never exactly neutral, and those are
// precisely the ones carrying its surfaces.
const GREY_SPREAD = 24;

// The WCAG AA floors: one for text, the lower one for the edges and glyphs held
// to it.
const AA_TEXT = 4.5;
const AA_EDGE = 3;

// Above this a hued colour is a wash rather than a fill: the pale pink behind a
// warning, not the red the warning is written in.
const TINT_LUMINANCE = 0.5;

type Rgb = [number, number, number];
type Role = 'surface' | 'edge' | 'text';

function lerp(from: Rgb, to: Rgb, amount: number): Rgb {
  return from.map((c, i) => Math.round(c + (to[i] - c) * amount)) as Rgb;
}

function spread([r, g, b]: Rgb): number {
  return Math.max(r, g, b) - Math.min(r, g, b);
}

// Perceived lightness, 0 for black and 1 for white. A design's greys are ordered
// by how light they look, and that order is what the turn is taken along.
function lightness([r, g, b]: Rgb): number {
  return (0.2126 * r + 0.7152 * g + 0.0722 * b) / 255;
}

function contrast(a: Rgb, b: Rgb): number {
  const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x);
  return (hi + 0.05) / (lo + 0.05);
}

// readableOn walks a colour away from the surface it sits on until it clears the
// floor, keeping its hue: a red stays the red it was saying, only light enough
// to be read against a dark page.
function readableOn(rgb: Rgb, surface: Rgb, floor: number): Rgb {
  const toward = luminance(surface) < 0.18 ? 255 : 0;
  let out = rgb;
  for (let i = 0; i < 24 && contrast(out, surface) < floor; i++) out = mix(out, toward, 0.06);
  return out.map(Math.round) as Rgb;
}

// A colour written as a hex or an rgb(), which is both how a stylesheet says it
// and how the CSSOM hands it back. Alpha is left out on purpose: a translucent
// colour is already sitting on whatever lerd painted underneath it.
const COLOUR = /#[0-9a-f]{3}\b|#[0-9a-f]{6}\b|rgb\(\s*\d+\s*,\s*\d+\s*,\s*\d+\s*\)/gi;

// The same, plus the names a stylesheet is allowed to use instead. Only the
// properties that paint are read this way, so a font called tan or a border that
// is merely solid is never mistaken for a colour.
const NAMED = /#[0-9a-f]{3}\b|#[0-9a-f]{6}\b|rgb\(\s*\d+\s*,\s*\d+\s*,\s*\d+\s*\)|\b[a-z]{3,20}\b/gi;
const PAINTS = /color|background|border|outline|shadow|fill|stroke/;
// Names that resolve to a colour but are not one to turn around: what they mean
// depends on where they land.
const NOT_A_COLOUR = new Set([
  'transparent',
  'currentcolor',
  'inherit',
  'initial',
  'unset',
  'revert',
  'revert-layer'
]);

const GRADIENT = /gradient\(/i;

// A custom property holding a bare triple, which is how Bootstrap 5 keeps the
// colours it later pours into rgba().
const TRIPLE = /^\s*(\d{1,3})\s*,\s*(\d{1,3})\s*,\s*(\d{1,3})\s*$/;

// One canvas per document, to ask the browser what a name means. A 2d context
// normalises whatever it is handed into a hex, which is the browser's own table
// of colour names rather than one lerd would have to keep and keep current.
const canvases = new WeakMap<Document, CanvasRenderingContext2D | null>();

function contextFor(doc: Document): CanvasRenderingContext2D | null {
  if (canvases.has(doc)) return canvases.get(doc) ?? null;
  let ctx: CanvasRenderingContext2D | null = null;
  try {
    ctx = doc.createElement('canvas').getContext('2d');
  } catch {
    ctx = null; // No canvas here. Names are then simply left alone.
  }
  canvases.set(doc, ctx);
  return ctx;
}

function nameToRgb(doc: Document, name: string): Rgb | null {
  const ctx = contextFor(doc);
  if (!ctx) return null;
  // A value the context cannot read leaves the fill where it was, so a token is
  // a colour only when it answers the same from two different starting points.
  ctx.fillStyle = '#000000';
  ctx.fillStyle = name;
  const first = String(ctx.fillStyle);
  ctx.fillStyle = '#ffffff';
  ctx.fillStyle = name;
  return first === String(ctx.fillStyle) ? readColour(first) : null;
}

function readColour(text: string): Rgb | null {
  if (text.startsWith('#')) return parseHex(text);
  const parts = text.match(/\d+/g);
  if (!parts || parts.length < 3) return null;
  const rgb = parts.slice(0, 3).map(Number) as Rgb;
  return rgb.every((c) => c >= 0 && c <= 255) ? rgb : null;
}

// roleOf reads what a declaration is painting from the property doing it, since
// a colour behaves differently as a surface than it does as the text on one.
function roleOf(prop: string): Role {
  if (prop.includes('background')) return 'surface';
  if (prop.includes('border') || prop.includes('outline')) return 'edge';
  return 'text';
}

function turnAround(rgb: Rgb, dark: boolean, role: Role): Rgb {
  const s = embeddedSurfaces(dark);
  const page = parseHex(s.bg)!;
  // A grey is placed by how light it was, the whole scale turned end for end, so
  // what the design stacked stays stacked and the gaps it left stay open.
  if (spread(rgb) <= GREY_SPREAD) return lerp(page, parseHex(RAMP_LIGHT)!, 1 - lightness(rgb));
  // A wash behind a notice is deepened to a tint of the same hue. A fill that
  // carries its own meaning, a primary button or a danger red, is left alone,
  // and anything written or drawn in a hue is lifted until it can be read.
  if (role === 'surface') {
    return luminance(rgb) > TINT_LUMINANCE ? (mix(rgb, 0, 0.82).map(Math.round) as Rgb) : rgb;
  }
  // Against the lighter of the two surfaces, since a hue written on a card has
  // to clear the floor there as well as on the page behind it.
  const card = parseHex(s.card)!;
  const lighter = luminance(card) > luminance(page) ? card : page;
  return readableOn(rgb, lighter, role === 'text' ? AA_TEXT : AA_EDGE);
}

// One map per palette and role, since a sweep meets the same handful of colours
// thousands of times over a stylesheet the size of phpMyAdmin's. The surfaces
// are part of the key, so picking a different palette in lerd builds a new map
// instead of handing back the last one's answers.
const palettes = new Map<string, Map<string, string>>();

function mappedColour(doc: Document, text: string, dark: boolean, role: Role): string | null {
  const s = embeddedSurfaces(dark);
  const key = `${dark}|${s.bg}|${s.card}|${role}`;
  let palette = palettes.get(key);
  if (!palette) {
    palette = new Map();
    palettes.set(key, palette);
  }
  const token = text.toLowerCase();
  const seen = palette.get(token);
  if (seen !== undefined) return seen || null;
  const rgb = NOT_A_COLOUR.has(token)
    ? null
    : token.startsWith('#') || token.startsWith('rgb')
      ? readColour(text)
      : nameToRgb(doc, text);
  const turned = rgb && turnAround(rgb, dark, role);
  const painted = turned && toHex(turned) !== toHex(rgb!) ? toHex(turned) : '';
  palette.set(token, painted);
  return painted || null;
}

// What a declaration said before lerd repainted it, so a second theme is applied
// to the dashboard's own colours rather than to the first theme's.
type Kept = Record<string, string>;
const originals = new WeakMap<CSSStyleDeclaration, Kept>();

// setPainted records what a declaration said before lerd changed it, so light
// mode gives the design back rather than the theme's second guess at it.
function setPainted(style: CSSStyleDeclaration, kept: Kept, prop: string, value: string): void {
  if (kept[prop] === undefined) kept[prop] = style.getPropertyValue(prop);
  style.setProperty(prop, value, style.getPropertyPriority(prop));
}

// paintValue turns around every colour one declaration carries. A property that
// paints is read for names as well, and a custom property holding a bare triple
// is turned around and written back as one.
function paintValue(doc: Document, prop: string, value: string, dark: boolean): string {
  // A light design leans on a white shadow to lift text off its surface. Turned
  // around, that same shadow is a glow behind every label, so it goes.
  if (prop === 'text-shadow') return 'none';
  // A box shadow keeps its geometry and gives up its colour: a pale one turned
  // around is a halo around the panel rather than a shadow under it.
  if (prop.includes('box-shadow')) {
    const s = embeddedSurfaces(dark);
    const under = toHex(mix(parseHex(s.bg)!, 0, 0.5).map(Math.round) as Rgb);
    return value.replace(NAMED, (m) => (mappedColour(doc, m, dark, 'surface') ? under : m));
  }
  // Its gradients are a sheen on a pale panel, a step or two apart. Turned
  // around they read as a seam across a dark one, so the panel is laid flat in
  // the colour its gradient started from.
  if (GRADIENT.test(value)) {
    if (prop.startsWith('--') || prop.includes('image')) return 'none';
    const flat = flattenGradient(doc, value, dark);
    return flat ?? value;
  }
  const triple = prop.startsWith('--') && value.match(TRIPLE);
  if (triple) {
    const rgb = [triple[1], triple[2], triple[3]].map(Number) as Rgb;
    if (rgb.some((c) => c > 255)) return value;
    // Nothing says what a bare triple will be poured into: Bootstrap uses the
    // same one for text, for a border and for a fill. Lifting it until it can be
    // read covers all three, where treating it as a surface leaves a danger red
    // unreadable wherever it is written rather than filled.
    const turned = turnAround(rgb, dark, 'text');
    return value.replace(TRIPLE, turned.join(', '));
  }
  const role = roleOf(prop);
  const form = PAINTS.test(prop) ? NAMED : COLOUR;
  return value.replace(form, (m) => mappedColour(doc, m, dark, role) ?? m);
}

// flattenGradient picks one colour to stand for the whole run: the deepest of
// its steps, once each has been turned around, so the panel settles into the
// page rather than floating a shade above it.
function flattenGradient(doc: Document, value: string, dark: boolean): string | null {
  const stops = (value.match(NAMED) ?? [])
    .map((m) => mappedColour(doc, m, dark, 'surface') ?? (readColour(m) ? m : null))
    .filter((m): m is string => !!m)
    .map((m) => readColour(m))
    .filter((c): c is Rgb => !!c);
  if (!stops.length) return null;
  const deepest = stops.reduce((a, b) => (luminance(b) < luminance(a) ? b : a));
  return toHex(deepest);
}

function firstColour(doc: Document, value: string): Rgb | null {
  const match = value.match(COLOUR);
  if (match) return readColour(match[0]);
  const name = value.match(/\b[a-z]{3,20}\b/i);
  return name && !NOT_A_COLOUR.has(name[0].toLowerCase()) ? nameToRgb(doc, name[0]) : null;
}

// A rule that fills itself with a hue deep enough to keep has already chosen the
// tone its own text reads in, and turning that tone around drops near-black text
// onto a saturated button.
function keepsItsOwnText(doc: Document, style: CSSStyleDeclaration): boolean {
  const bg = firstColour(
    doc,
    style.getPropertyValue('background-color') || style.getPropertyValue('background')
  );
  return !!bg && spread(bg) > GREY_SPREAD && luminance(bg) <= TINT_LUMINANCE;
}

// A design can put text and the surface under it in one rule and still leave the
// two close together, leaning on a shadow or a border to tell them apart. Turned
// around, that margin is all there is, so the text is walked away from its own
// surface until it clears the floor.
function guardPair(doc: Document, style: CSSStyleDeclaration, kept: Kept, dark: boolean): void {
  const fg = firstColour(doc, style.getPropertyValue('color'));
  const bg = firstColour(
    doc,
    style.getPropertyValue('background-color') || style.getPropertyValue('background')
  );
  if (!fg || !bg || contrast(fg, bg) >= AA_TEXT) return;
  const lifted = readableOn(fg, bg, AA_TEXT);
  const s = embeddedSurfaces(dark);
  const ends: Rgb[] = [lifted, parseHex(DARK_TEXT)!, parseHex(s.bg)!];
  const best = ends.reduce((a, b) => (contrast(b, bg) > contrast(a, bg) ? b : a));
  setPainted(style, kept, 'color', toHex(best));
}

// A filled button keeps the tone it chose for its own label, so it is the fill
// that moves: deep enough for that label to be read, and still the colour it was
// saying, since a green that means view and a red that means delete are carrying
// the meaning between them. Light designs pitch these fills bright, which reads
// as a lamp on a dark page as well as failing to be read.
function guardFill(doc: Document, style: CSSStyleDeclaration, kept: Kept): void {
  const prop = style.getPropertyValue('background-color') ? 'background-color' : 'background';
  const value = style.getPropertyValue(prop);
  const fg = firstColour(doc, style.getPropertyValue('color'));
  const bg = firstColour(doc, value);
  if (!fg || !bg || contrast(fg, bg) >= AA_TEXT) return;
  const seated = readableOn(bg, fg, AA_TEXT);
  const match = value.match(NAMED);
  if (!match) return;
  setPainted(style, kept, prop, value.replace(match[0], toHex(seated)));
}

function repaintDeclaration(doc: Document, style: CSSStyleDeclaration, dark: boolean): void {
  const seen = originals.get(style);
  // Light mode is the design's own, so it is given back rather than mapped onto
  // a second palette that would only be a paler copy of it.
  if (!dark) {
    for (const [prop, original] of Object.entries(seen ?? {})) {
      style.setProperty(prop, original, style.getPropertyPriority(prop));
    }
    if (seen) originals.delete(style);
    return;
  }
  const kept = seen ?? {};
  const ownText = keepsItsOwnText(doc, style);
  let touched = false;
  for (const prop of Array.from(style)) {
    if (ownText && prop === 'color') continue;
    const original = kept[prop] ?? style.getPropertyValue(prop);
    const painted = paintValue(doc, prop, original, dark);
    if (painted === original) continue;
    kept[prop] = original;
    style.setProperty(prop, painted, style.getPropertyPriority(prop));
    touched = true;
  }
  if (ownText) guardFill(doc, style, kept);
  else guardPair(doc, style, kept, dark);
  if (!touched && !Object.keys(kept).length) return;
  originals.set(style, kept);
}

// Rules nest: a design's print sheet, its responsive rules and its own media
// queries all carry their own, and the colours inside them are the same colours.
function repaintRules(doc: Document, rules: CSSRuleList | null | undefined, dark: boolean): void {
  for (const rule of Array.from(rules ?? [])) {
    const nested = (rule as CSSGroupingRule).cssRules;
    if (nested) repaintRules(doc, nested, dark);
    const style = (rule as CSSStyleRule).style;
    if (style && typeof style.length === 'number') repaintDeclaration(doc, style, dark);
  }
}

// repaintLightOnly turns the design around wherever its stylesheet wrote it, and
// hands it back untouched in light mode.
export function repaintLightOnly(doc: Document, dark: boolean): void {
  for (const sheet of Array.from(doc.styleSheets)) {
    // Lerd's own sheet is already in the theme's colours. Sweeping it would map
    // them a second time and hand the page back its own palette inverted.
    if ((sheet.ownerNode as HTMLElement | null)?.id === THEME_STYLE_ID) continue;
    try {
      repaintRules(doc, sheet.cssRules, dark);
    } catch {
      continue; // A cross-origin sheet the page pulled in. Not ours to read.
    }
  }
}

// themeLightOnlyDocument paints what the sweep cannot reach: the page itself,
// which a design often leaves to the browser, the form controls the browser
// draws, and the tone inherited by everything the stylesheet never named.
export function themeLightOnlyDocument(doc: Document, dark: boolean): void {
  let style = doc.getElementById(THEME_STYLE_ID) as HTMLStyleElement | null;
  if (!style) {
    style = doc.createElement('style');
    style.id = THEME_STYLE_ID;
    doc.head?.appendChild(style);
  }
  const s = embeddedSurfaces(dark);
  style.textContent = dark
    ? `html, body { background: ${s.bg}; color: ${DARK_TEXT}; }
html { color-scheme: dark; }
a, a:visited { color: ${s.accent}; }
a:hover { color: ${s.accentHover}; }`
    : `a, a:visited { color: ${s.accent}; }
a:hover { color: ${s.accentHover}; }`;
}
