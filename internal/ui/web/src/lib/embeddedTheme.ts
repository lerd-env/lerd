// A proxied dashboard is served same-origin under /_svc/, which is what lets the
// overlay reach into it. Most of them ship their themes behind prefers-color-scheme
// media queries, so an embedded tool follows the browser rather than the theme the
// user picked in lerd, and a dark dashboard can end up framing a white page.
// Flipping those links settles it: the page keeps its own design, only the switch
// moves.
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
  } catch {
    // Cross-origin, or a frame that navigated away mid-call. Nothing to sync.
  }
}
