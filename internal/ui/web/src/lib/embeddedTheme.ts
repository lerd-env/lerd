// A proxied dashboard is served same-origin under /_svc/, which is what lets the
// overlay reach into it. Most of them ship their themes behind prefers-color-scheme
// media queries, so an embedded tool follows the browser rather than the theme the
// user picked in lerd, and a dark dashboard can end up framing a white page.
// Flipping those queries settles it, on the link or inside the sheet: the page
// keeps its own design, only the switch moves.
const DARK = 'prefers-color-scheme: dark';
const LIGHT = 'prefers-color-scheme: light';

// The palette lerd is wearing, as custom properties. A dashboard whose design
// reads them follows a palette change without the overlay knowing anything about
// that design; one that ignores them is unaffected by their presence.
const PALETTE_VARS = [
  '--lerd-accent',
  '--lerd-accent-hover',
  '--lerd-bg',
  '--lerd-card',
  '--lerd-border',
  '--lerd-muted'
];

// A theme can also be gated inside the stylesheet rather than on the link to it,
// which is how php-spx ships its light mode. Same-origin sheets are mutable, so
// there the condition itself is what moves. The original is kept per rule, since
// rewriting it is what makes the rule unrecognisable on the next pass.
const gates = new WeakMap<CSSRule, string>();

function flipMediaRules(doc: Document, dark: boolean): void {
  for (const sheet of Array.from(doc.styleSheets)) {
    let rules: CSSRuleList | null = null;
    try {
      rules = sheet.cssRules;
    } catch {
      continue; // A cross-origin sheet the page pulled in. Not ours to read.
    }
    for (const rule of Array.from(rules ?? [])) {
      // instanceof would test the wrong realm's CSSMediaRule, so ask the rule
      // whether it carries a media condition at all.
      const media = (rule as CSSMediaRule).media;
      if (!media || typeof media.mediaText !== 'string') continue;
      const gate = (gates.get(rule) ?? media.mediaText).replace(/\s+/g, ' ');
      const wantsDark = gate.includes(DARK);
      const wantsLight = gate.includes(LIGHT);
      if (!wantsDark && !wantsLight) continue;
      gates.set(rule, gate);
      media.mediaText = wantsDark === dark ? 'all' : 'not all';
    }
  }
}

function copyPalette(from: HTMLElement, to: HTMLElement): void {
  const style = from.style;
  for (const name of PALETTE_VARS) {
    const value = style.getPropertyValue(name);
    if (value) to.style.setProperty(name, value);
  }
}

// syncEmbeddedTheme makes one embedded document follow lerd's own light or dark.
// It touches only stylesheets that gate themselves on the media query, so a
// dashboard that themes itself some other way is left exactly as it is. Both
// directions matter: a page can carry a light sheet and a dark sheet, and turning
// the dark one on without turning the light one off leaves them fighting.
export function syncEmbeddedTheme(
  doc: Document | null | undefined,
  dark: boolean,
  host: HTMLElement | null = typeof document === 'undefined' ? null : document.documentElement
): void {
  if (!doc) return;
  try {
    if (host && doc.documentElement) copyPalette(host, doc.documentElement);
    for (const link of Array.from(doc.querySelectorAll('link[rel~="stylesheet"]'))) {
      const el = link as HTMLLinkElement;
      // The media attribute is rewritten in place, so the original query has to be
      // kept to recognise the link again on the next pass.
      const gate = (el.dataset.lerdMedia ?? el.media).replace(/\s+/g, ' ');
      const wantsDark = gate.includes(DARK);
      const wantsLight = gate.includes(LIGHT);
      if (!wantsDark && !wantsLight) continue;
      el.dataset.lerdMedia = gate;
      el.media = wantsDark === dark ? 'all' : 'not all';
    }
    flipMediaRules(doc, dark);
  } catch {
    // Cross-origin, or a frame that navigated away mid-call. Nothing to sync.
  }
}
