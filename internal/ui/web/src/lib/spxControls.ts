// Helpers for tailoring the embedded SPX profiler UI. SPX's web UI is
// upstream php-spx, reverse-proxied same-origin under /_spx/; lerd reaches
// into that iframe to collapse the control panel's Configuration form, pad
// the control panel page, and surface freshly captured reports without a
// manual refresh, and dress it in lerd's palette. The selectors, custom
// property names and the metadata URL below are the only points of coupling
// to SPX's internals.

// SPX_METADATA_URL is the endpoint the SPX control panel page itself uses to
// list captured reports. Polling it lets lerd notice new captures.
const SPX_METADATA_URL = '/_spx/?SPX_UI_URI=/data/reports/metadata';

// CONFIG_FORM is the Configuration fieldset wrapper on the control panel page.
const CONFIG_FORM = '#config';
const PAD_STYLE_ID = 'lerd-spx-pad';
const THEME_STYLE_ID = 'lerd-spx-theme';

// SPX paints itself from custom properties of its own, plus a few literal teals
// its stylesheet never routed through them. Pointing both at lerd's surfaces is
// all it takes for the profiler to wear the dashboard's colours; the flame graph
// keeps upstream's, since those are drawn into a canvas rather than styled.
//
// The palette carries only the dark surfaces, light mode being the greys app.css
// names inline, so those greys are repeated here rather than invented.
function themeCss(dark: boolean): string {
  const card = dark ? 'var(--lerd-card)' : '#ffffff';
  const bg = dark ? 'var(--lerd-bg)' : '#f9fafb';
  const border = dark ? 'var(--lerd-border)' : '#e5e7eb';
  return `
:root {
  --gradient-begin: ${card};
  --gradient-end: ${bg};
  --border-color: ${border};
  --form-element-background: ${card};
  --hover-color: var(--lerd-accent);
  --table-sort-field-background: color-mix(in srgb, var(--lerd-accent) 22%, transparent);
}
.widget { border-color: ${border}; }
#search-container button { background: var(--lerd-accent); }
#colorscheme-panel hr { border-color: ${border}; }
`;
}

// True for the SPX single-report analysis screen, false for the control panel.
export function isSpxReportView(href: string): boolean {
  return href.includes('SPX_UI_URI=/report.html');
}

// setSpxConfigHidden shows or hides the Configuration form on the SPX control
// panel page, which otherwise pushes the report list far down the page.
// Returns whether the form was found.
export function setSpxConfigHidden(doc: Document, hidden: boolean): boolean {
  const form = doc.querySelector(CONFIG_FORM);
  // The SPX page runs in the iframe's own realm, so test against that realm's
  // HTMLElement; a plain `instanceof HTMLElement` would always be false here.
  const HtmlEl = doc.defaultView?.HTMLElement ?? HTMLElement;
  if (!(form instanceof HtmlEl)) return false;
  form.style.display = hidden ? 'none' : '';
  return true;
}

// padSpxControlPanel insets the SPX control panel page, whose content
// otherwise sits flush against the iframe edges. Idempotent.
export function padSpxControlPanel(doc: Document): void {
  if (doc.getElementById(PAD_STYLE_ID)) return;
  const style = doc.createElement('style');
  style.id = PAD_STYLE_ID;
  style.textContent = 'body{padding:20px;box-sizing:border-box}';
  doc.head.appendChild(style);
}

// themeSpxDocument maps lerd's surfaces onto SPX's own variables. The style goes
// in last so it outranks both SPX's defaults and its light-mode block, whichever
// syncEmbeddedTheme left switched on, and is rewritten rather than re-added on a
// mode change. Applies to the report screen as much as the control panel.
export function themeSpxDocument(doc: Document, dark: boolean): void {
  let style = doc.getElementById(THEME_STYLE_ID) as HTMLStyleElement | null;
  if (!style) {
    style = doc.createElement('style');
    style.id = THEME_STYLE_ID;
    doc.head.appendChild(style);
  }
  style.textContent = themeCss(dark);
}

// fetchSpxReportCount returns how many SPX reports currently exist, read from
// the same metadata endpoint the control panel page uses. Returns null on any
// failure so callers can simply skip that poll.
export async function fetchSpxReportCount(): Promise<number | null> {
  try {
    const res = await fetch(SPX_METADATA_URL, { credentials: 'same-origin' });
    if (!res.ok) return null;
    const data = (await res.json()) as { results?: unknown };
    return Array.isArray(data.results) ? data.results.length : null;
  } catch {
    return null;
  }
}
