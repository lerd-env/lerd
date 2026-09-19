// A dashboard with a palette of its own and no way to change it can still be
// brought onto the theme's, the page being same-origin: the colours are read off
// its stylesheet and written back as lerd's, wherever it wrote them. This is the
// mechanism, shared by every dashboard dressed that way; which colour stands for
// which is each one's own business.
//
// It suits a design whose scale is already the right way round, a dark app being
// asked to wear a different dark. A design that only has a light half is turned
// around instead, which is a different job (see lightOnlyTheme).

export type Palette = Record<string, string>;
export type PalettePairs = ReadonlyArray<readonly [RegExp, string]>;

// The names a stylesheet is as likely to write as the hex, for the two colours
// that have a short one everybody uses.
const KEYWORDS: Record<string, RegExp> = {
  '#ffffff': /\bwhite\b/gi,
  '#000000': /\bblack\b/gi
};

// A colour is written three ways: the hex the author typed, the rgb() the CSSOM
// hands back, and for a couple of them a plain name.
function formsOf(hex: string): RegExp[] {
  const full = hex.length === 4 ? '#' + hex[1] + hex[1] + hex[2] + hex[2] + hex[3] + hex[3] : hex;
  const [r, g, b] = [1, 3, 5].map((i) => parseInt(full.slice(i, i + 2), 16));
  const forms = [
    new RegExp(hex, 'gi'),
    new RegExp(`rgb\\(\\s*${r},\\s*${g},\\s*${b}\\s*\\)`, 'gi')
  ];
  const keyword = KEYWORDS[full.toLowerCase()];
  if (keyword) forms.push(new RegExp(keyword.source, 'gi'));
  return forms;
}

// palettePairs turns a map of the dashboard's colours to the theme's into the
// forms a stylesheet might have written them in.
export function palettePairs(palette: Palette): PalettePairs {
  return Object.entries(palette).flatMap(([from, to]) =>
    formsOf(from).map((form) => [form, to] as const)
  );
}

// What a declaration said before lerd repainted it, so a second theme is applied
// to the dashboard's own colours rather than to the first theme's.
const originals = new WeakMap<CSSStyleDeclaration, Record<string, string>>();

export function repaintDeclaration(style: CSSStyleDeclaration, pairs: PalettePairs): void {
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

// repaintPalette sweeps every sheet the page can hand over. A generated rule has
// no selector worth targeting and no variable to set, so the values themselves
// are what move.
export function repaintPalette(doc: Document, pairs: PalettePairs): void {
  for (const sheet of Array.from(doc.styleSheets)) {
    let rules: CSSRuleList | null = null;
    try {
      rules = sheet.cssRules;
    } catch {
      continue; // A cross-origin sheet the page pulled in. Not ours to read.
    }
    repaintRules(rules, pairs);
  }
}

function repaintRules(rules: CSSRuleList | null | undefined, pairs: PalettePairs): void {
  for (const rule of Array.from(rules ?? [])) {
    const nested = (rule as CSSGroupingRule).cssRules;
    if (nested) repaintRules(nested, pairs);
    const style = (rule as CSSStyleRule).style;
    if (style && typeof style.length === 'number') repaintDeclaration(style, pairs);
  }
}

// watchPaletteRules repaints a rule as it is inserted. An app that builds its
// styles at runtime adds them through the stylesheet rather than the DOM, so
// nothing observable fires and a sweep alone leaves whatever arrived after it in
// the dashboard's own colours. Reading the palette through a callback keeps the
// patch correct across a theme change.
export function watchPaletteRules(
  win: Window & typeof globalThis,
  pairs: () => PalettePairs
): void {
  const proto = win.CSSStyleSheet?.prototype as (CSSStyleSheet & { lerdPainted?: true }) | undefined;
  if (!proto || proto.lerdPainted) return;
  const insert = proto.insertRule;
  proto.insertRule = function (rule: string, index?: number): number {
    const at = insert.call(this, rule, index);
    try {
      const style = (this.cssRules[at] as CSSStyleRule)?.style;
      if (style) repaintDeclaration(style, pairs());
    } catch {
      // A cross-origin sheet, or a rule type that carries no declaration.
    }
    return at;
  };
  proto.lerdPainted = true;
}
