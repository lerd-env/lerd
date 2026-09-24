// Mailpit themes itself from a preference of its own, read once when its app
// boots. It boots after the frame's load event, since it fetches its config
// first, so the attribute the overlay sets is overwritten a moment later and the
// view keeps whatever the browser preferred. Served same-origin that preference
// sits in lerd's own storage, which is what lets the dashboard hand Mailpit the
// mode to start in rather than fight it afterwards.
import { embeddedSurfaces } from './embeddedTheme';
import { parseHex } from './brandTint';
import { onAccent } from './palettes';

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

// Mailpit marks its header dark in the markup, the header having been a navy
// band. On the light surface it gets instead, the placeholder and the select
// chevron need Bootstrap's light-mode tones back.
const LIGHT_HEADER = `
.navbar.bg-primary[data-bs-theme="dark"] {
  --bs-body-color: #111827;
  --bs-emphasis-color: #000;
  --bs-secondary-color: rgba(17, 24, 39, .6);
}
.navbar.bg-primary[data-bs-theme="dark"] .form-select {
  --bs-form-select-bg-img: url("data:image/svg+xml,%3csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 16 16'%3e%3cpath fill='none' stroke='%23343a40' stroke-linecap='round' stroke-linejoin='round' stroke-width='2' d='m2 5 6 6 6-6'/%3e%3c/svg%3e");
}
`;

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
  const label = onAccent(s.accent);
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
  --bs-focus-ring-color: rgba(${rgbParts(s.accent)}, .25);
}
/* Bootstrap compiled its primary into each component's own variables, so the
   ones Mailpit draws are handed the accent one by one. */
.list-group {
  --bs-list-group-active-bg: ${s.accent};
  --bs-list-group-active-border-color: ${s.accent};
  --bs-list-group-active-color: ${label};
}
.btn-primary {
  --bs-btn-bg: ${s.accent};
  --bs-btn-border-color: ${s.accent};
  --bs-btn-color: ${label};
  --bs-btn-hover-bg: ${s.accentHover};
  --bs-btn-hover-border-color: ${s.accentHover};
  --bs-btn-hover-color: ${label};
  --bs-btn-active-bg: ${s.accentHover};
  --bs-btn-active-border-color: ${s.accentHover};
  --bs-btn-active-color: ${label};
  --bs-btn-disabled-bg: ${s.accent};
  --bs-btn-disabled-border-color: ${s.accent};
  --bs-btn-disabled-color: ${label};
}
.btn-outline-primary {
  --bs-btn-color: ${s.accent};
  --bs-btn-border-color: ${s.accent};
  --bs-btn-hover-bg: ${s.accent};
  --bs-btn-hover-border-color: ${s.accent};
  --bs-btn-hover-color: ${label};
  --bs-btn-active-bg: ${s.accent};
  --bs-btn-active-border-color: ${s.accent};
  --bs-btn-active-color: ${label};
}
.nav-pills {
  --bs-nav-pills-link-active-bg: ${s.accent};
  --bs-nav-pills-link-active-color: ${label};
}
.dropdown-menu, .dropdown-menu-dark {
  --bs-dropdown-link-active-bg: ${s.accent};
  --bs-dropdown-link-active-color: ${label};
}
.progress, .progress-stacked {
  --bs-progress-bar-bg: ${s.accent};
  --bs-progress-bar-color: ${label};
}
.form-check-input:checked {
  background-color: ${s.accent};
  border-color: ${s.accent};
}
.form-control:focus, .form-select:focus, .form-check-input:focus {
  border-color: ${s.accent};
  box-shadow: 0 0 0 .25rem rgba(${rgbParts(s.accent)}, .25);
}
.navbar.bg-primary {
  background-color: ${s.card} !important;
  border-bottom: 1px solid ${s.border};
}
.navbar.bg-primary, .navbar.bg-primary * { color: ${dark ? '#e7eaed' : '#111827'} !important; }
.navbar.bg-primary .btn { border-color: ${s.border} !important; }
${dark ? '' : LIGHT_HEADER}`;
}
