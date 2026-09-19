// Mailpit themes itself from a preference of its own, read once when its app
// boots. It boots after the frame's load event, since it fetches its config
// first, so the attribute the overlay sets is overwritten a moment later and the
// view keeps whatever the browser preferred. Served same-origin that preference
// sits in lerd's own storage, which is what lets the dashboard hand Mailpit the
// mode to start in rather than fight it afterwards.
import { embeddedSurfaces } from './embeddedTheme';
import { parseHex } from './brandTint';

const MAILPIT_THEME_KEY = 'mp-theme';
const THEME_STYLE_ID = 'lerd-mailpit-theme';

// rememberMailpitTheme records the mode Mailpit should boot in. Safe to call
// before the frame exists: the storage is the dashboard's own.
export function rememberMailpitTheme(dark: boolean): void {
  try {
    localStorage.setItem(MAILPIT_THEME_KEY, dark ? 'dark' : 'light');
  } catch {
    // Storage disabled. The attribute still themes a frame already open.
  }
}

// Bootstrap composes its translucent surfaces from the rgb parts of a colour, so
// a variable set to a hex alone leaves those parts on the theme's own tone.
function rgbParts(hex: string): string {
  const rgb = parseHex(hex);
  return rgb ? rgb.join(', ') : '';
}

// themeMailpitDocument paints Mailpit's Bootstrap variables with lerd's surfaces
// and accent, so the mail view wears the dashboard's theme rather than
// Bootstrap's own greys. The style goes in last and is rewritten on a mode
// change; it sets the variables on every theme root Bootstrap may be using, so
// whichever one Mailpit's own switch left in place is the one that is painted.
//
// The header is the one place the accent is kept out of: it is Bootstrap's
// primary colour by default, which is a band of brand colour across the top
// where the dashboard has a quiet rail. It gets the raised surface instead, and
// its text a readable tone per mode, since the markup asks for white.
export function themeMailpitDocument(doc: Document, dark: boolean): void {
  let style = doc.getElementById(THEME_STYLE_ID) as HTMLStyleElement | null;
  if (!style) {
    style = doc.createElement('style');
    style.id = THEME_STYLE_ID;
    doc.head?.appendChild(style);
  }
  const s = embeddedSurfaces(dark);
  style.textContent = `
:root, [data-bs-theme="dark"], [data-bs-theme="light"] {
  --bs-body-bg: ${s.bg};
  --bs-body-bg-rgb: ${rgbParts(s.bg)};
  --bs-secondary-bg: ${s.card};
  --bs-secondary-bg-rgb: ${rgbParts(s.card)};
  --bs-tertiary-bg: ${s.card};
  --bs-tertiary-bg-rgb: ${rgbParts(s.card)};
  --bs-border-color: ${s.border};
  --bs-primary: ${s.accent};
  --bs-primary-rgb: ${rgbParts(s.accent)};
  --bs-link-color: ${s.accent};
  --bs-link-color-rgb: ${rgbParts(s.accent)};
  --bs-link-hover-color: ${s.accentHover};
}
.navbar.bg-primary {
  background-color: ${s.card} !important;
  border-bottom: 1px solid ${s.border};
}
.navbar.bg-primary, .navbar.bg-primary * { color: ${dark ? '#e7eaed' : '#111827'} !important; }
.navbar.bg-primary .btn { border-color: ${s.border} !important; }
`;
}
