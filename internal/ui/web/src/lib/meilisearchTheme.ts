import { embeddedSurfaces } from './embeddedTheme';
import { parseHex, toHex } from './brandTint';

// Meilisearch's mini-dashboard has one palette and no dark mode at all: its
// colours are compiled into styled-components class names, so there is neither a
// switch to move nor a variable to repaint. What there is, the page being
// same-origin, is the values themselves. Its palette is a small, ordered thing,
// a twelve step grey scale plus white and an accent, so each step is mapped onto
// the theme's own scale and written back where its stylesheet wrote it.
//
// Inverting the page with a filter was the other way, and it cannot land on a
// theme: white inverts to a neutral, so a tinted background comes out grey, a
// shadow comes out as a glow, and every logo has to be inverted back.
const THEME_STYLE_ID = 'lerd-meilisearch-theme';

// Meilisearch's grey scale, darkest first: text at one end, page background at
// the other. Its white is the colour of a panel sitting on that background.
const MEILI_GREYS = [
  '#39486e',
  '#4f5c7e',
  '#606c8b',
  '#737e99',
  '#838da5',
  '#959db3',
  '#a7aec0',
  '#bbc1cf',
  '#cbcfdb',
  '#e4e7ee',
  '#edeef7',
  '#fafbfe'
];
const MEILI_WHITE = '#ffffff';
const MEILI_ACCENT = '#e41359';
const MEILI_ACCENT_HOVER = '#ca1b53';
// The pale pinks it fills with behind the accent.
const MEILI_ACCENT_TINTS = ['#ffdbe7', '#fdeef3'];

// The tone text reads in on a dark surface. The palette carries surfaces and an
// accent, not a text colour, so the dashboard's own is repeated here.
const DARK_TEXT = '#e5e7eb';

type Rgb = [number, number, number];

function lerp(from: Rgb, to: Rgb, amount: number): string {
  return toHex(from.map((c, i) => Math.round(c + (to[i] - c) * amount)) as Rgb);
}

// themeColours maps every colour Meilisearch paints with onto the theme's, in
// the order its own scale runs: its darkest grey is text, its lightest is the
// page, and dark mode turns that order around.
function themeColours(dark: boolean): Record<string, string> {
  const s = embeddedSurfaces(dark);
  const text = parseHex(dark ? DARK_TEXT : MEILI_GREYS[0])!;
  const page = parseHex(s.bg)!;
  const out: Record<string, string> = {};
  MEILI_GREYS.forEach((grey, i) => {
    out[grey] = lerp(text, page, i / (MEILI_GREYS.length - 1));
  });
  out[MEILI_WHITE] = s.card;
  out[MEILI_ACCENT] = s.accent;
  out[MEILI_ACCENT_HOVER] = s.accentHover;
  const accent = parseHex(s.accent)!;
  MEILI_ACCENT_TINTS.forEach((tint, i) => {
    out[tint] = lerp(page, accent, dark ? 0.18 - i * 0.08 : 0.16 - i * 0.08);
  });
  return out;
}

// What a declaration said before lerd repainted it, so a second theme is applied
// to Meilisearch's colours rather than to the first theme's.
const originals = new WeakMap<CSSStyleDeclaration, Record<string, string>>();

// Its stylesheet writes a colour three ways, the hex it was given, the rgb() the
// CSSOM hands back, and the plain keyword for white, so each is matched.
function formsOf(hex: string): RegExp[] {
  const [r, g, b] = parseHex(hex)!;
  const forms = [
    new RegExp(hex.replace('#', '#'), 'gi'),
    new RegExp(`rgb\\(\\s*${r},\\s*${g},\\s*${b}\\s*\\)`, 'gi')
  ];
  if (hex === MEILI_WHITE) forms.push(/\bwhite\b/gi);
  return forms;
}

// repaintMeilisearch rewrites its palette wherever its stylesheet wrote it. The
// rules are generated, so there is no selector to target and no variable to set.
function palettePairs(dark: boolean): ReadonlyArray<readonly [RegExp, string]> {
  return Object.entries(themeColours(dark)).flatMap(([from, to]) =>
    formsOf(from).map((form) => [form, to] as const)
  );
}

export function repaintMeilisearch(doc: Document, dark: boolean): void {
  const pairs = palettePairs(dark);
  for (const sheet of Array.from(doc.styleSheets)) {
    let rules: CSSRuleList | null = null;
    try {
      rules = sheet.cssRules;
    } catch {
      continue;
    }
    for (const rule of Array.from(rules ?? [])) {
      const style = (rule as CSSStyleRule).style;
      if (!style || typeof style.length !== 'number') continue;
      repaintDeclaration(style, pairs);
    }
  }
}

function repaintDeclaration(
  style: CSSStyleDeclaration,
  pairs: ReadonlyArray<readonly [RegExp, string]>
): void {
  const seen = originals.get(style) ?? {};
  for (const prop of Array.from(style)) {
    const original = seen[prop] ?? style.getPropertyValue(prop);
    let painted = original;
    for (const [form, to] of pairs) painted = painted.replace(form, to);
    if (painted === original) continue;
    seen[prop] = original;
    style.setProperty(prop, painted, style.getPropertyPriority(prop));
  }
  if (Object.keys(seen).length) originals.set(style, seen);
}

// watchMeilisearchRules repaints a rule as it is inserted. styled-components
// adds one per component as the app mounts, through the stylesheet rather than
// the DOM, so nothing observable fires and a sweep alone leaves whatever arrived
// after it in Meilisearch's own colours. Reading the mode through a callback
// keeps the patch correct across a theme change.
export function watchMeilisearchRules(win: Window & typeof globalThis, dark: () => boolean): void {
  const proto = win.CSSStyleSheet?.prototype as (CSSStyleSheet & { lerdPatched?: true }) | undefined;
  if (!proto || proto.lerdPatched) return;
  const insert = proto.insertRule;
  proto.insertRule = function (rule: string, index?: number): number {
    const at = insert.call(this, rule, index);
    try {
      const style = (this.cssRules[at] as CSSStyleRule)?.style;
      if (style) repaintDeclaration(style, palettePairs(dark()));
    } catch {
      // A cross-origin sheet, or a rule type that carries no declaration.
    }
    return at;
  };
  proto.lerdPatched = true;
}

// themeMeilisearchDocument paints what the sweep cannot reach: the page itself,
// which the app never styles, and the scrollbars the browser draws for it.
export function themeMeilisearchDocument(doc: Document, dark: boolean): void {
  let style = doc.getElementById(THEME_STYLE_ID) as HTMLStyleElement | null;
  if (!style) {
    style = doc.createElement('style');
    style.id = THEME_STYLE_ID;
    doc.head?.appendChild(style);
  }
  const s = embeddedSurfaces(dark);
  style.textContent = `html, body { background: ${s.bg}; }
html { color-scheme: ${dark ? 'dark' : 'light'}; }`;
}
