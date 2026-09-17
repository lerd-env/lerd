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

// Bootstrap 5 reads its mode off an attribute rather than a media query, so a
// page built on it (Mailpit) switches only when that attribute moves. Setting it
// costs nothing on a page that is not Bootstrap's, which is why no preset has to
// declare that it is.
// The surfaces an embedded page is dressed in. The palette carries only the dark
// ones, light mode in the dashboard being the greys app.css names inline, so
// those greys are repeated here rather than invented. Concrete values, not var()
// references, because a page whose design needs them broken into rgb parts
// (Bootstrap's translucency) cannot do that arithmetic in CSS.
export interface EmbeddedSurfaces {
  bg: string;
  card: string;
  border: string;
  accent: string;
  accentHover: string;
}

const LIGHT_SURFACES = { bg: '#f9fafb', card: '#ffffff', border: '#e5e7eb' };

export function embeddedSurfaces(
  dark: boolean,
  host: HTMLElement | null = typeof document === 'undefined' ? null : document.documentElement
): EmbeddedSurfaces {
  const read = (name: string, fallback: string) =>
    (host?.style.getPropertyValue(name) || '').trim() || fallback;
  return {
    bg: dark ? read('--lerd-bg', '#0d0d0d') : LIGHT_SURFACES.bg,
    card: dark ? read('--lerd-card', '#161616') : LIGHT_SURFACES.card,
    border: dark ? read('--lerd-border', '#262626') : LIGHT_SURFACES.border,
    accent: read('--lerd-accent', '#ff2d20'),
    accentHover: read('--lerd-accent-hover', '#e02419')
  };
}

function setBootstrapTheme(root: HTMLElement | null, dark: boolean): void {
  if (root) root.setAttribute('data-bs-theme', dark ? 'dark' : 'light');
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
    setBootstrapTheme(doc.documentElement, dark);
  } catch {
    // Cross-origin, or a frame that navigated away mid-call. Nothing to sync.
  }
}
