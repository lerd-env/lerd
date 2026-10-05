import { mount } from 'svelte';
import appCss from '../app.css?inline';
import barCss from './bar.css?inline';
import latin from '@fontsource-variable/inter/files/inter-latin-wght-normal.woff2?url';
import latinExt from '@fontsource-variable/inter/files/inter-latin-ext-wght-normal.woff2?url';
import Bar from './Bar.svelte';
import { barConfig } from './config';
import { setLayerRoot } from '$lib/portal';
import { setApiRewrite } from '$lib/api';
import { editorAvailable } from '$lib/editor';
import { sourceAvailable } from '$lib/sourceCode';
import { sites, type Site } from '$stores/sites';
import { mergePalettes, paletteById, paletteVars } from '$lib/palettes';

// Read while the script runs: document.currentScript is gone after that. A
// page no PHP request served, a proxied dev server's, has no id; the bar then
// follows the page view browser capture names.
const rid = (document.currentScript as HTMLScriptElement | null)?.dataset.rid ?? '';

// Custom properties registered with @property are ignored inside a shadow
// root, so the initial values Tailwind gives its own are set plainly instead.
function shadowSafe(css: string): string {
  const initial: string[] = [];
  const rest = css.replace(/@property\s+(--[\w-]+)\s*\{([^}]*)\}/g, (_, name: string, body: string) => {
    const v = /initial-value\s*:\s*([^;]+)/.exec(body);
    if (v) initial.push(`${name}:${v[1].trim()}`);
    return '';
  });
  return `${rest}\n:host, *, ::before, ::after, ::backdrop { ${initial.join(';')} }`;
}

// The fonts come with the bar, under a family of its own so a page's own
// Inter is never swapped. @font-face only works in the document, not in a
// shadow root.
function addFonts() {
  if (document.getElementById('lerd-debugbar-fonts')) return;
  const style = document.createElement('style');
  style.id = 'lerd-debugbar-fonts';
  const face = (src: string, range: string) => `@font-face{font-family:'lerd Inter';font-style:normal;font-display:swap;font-weight:100 900;src:url(${src}) format('woff2-variations');unicode-range:${range}}`;
  style.textContent = face(latin, 'U+0000-00FF,U+0131,U+0152-0153,U+02BB-02BC,U+02C6,U+02DA,U+02DC,U+0304,U+0308,U+0329,U+2000-206F,U+20AC,U+2122,U+2191,U+2193,U+2212,U+2215,U+FEFF,U+FFFD') + face(latinExt, 'U+0100-02BA,U+02BD-02C5,U+02C7-02CC,U+02CE-02D7,U+02DD-02FF,U+1D00-1DBF,U+1E00-1E9F,U+1EF2-1EFF,U+2020,U+20A0-20AB,U+20AD-20C0,U+2113,U+2C60-2C7F,U+A720-A7FF');
  document.head.appendChild(style);
}

// lerd's theme, not the site's: System follows the OS, Light and Dark are fixed.
// The tones go on the host element, where app.css's theme reads them; set any
// deeper and its colours would already have resolved to the defaults.
function applyTheme(host: HTMLElement, root: HTMLElement) {
  const dark = barConfig.theme === 'dark' || (barConfig.theme === 'auto' && window.matchMedia('(prefers-color-scheme: dark)').matches);
  root.classList.toggle('dark', dark);
  for (const [name, value] of Object.entries(paletteVars(paletteById(mergePalettes(barConfig.themes), barConfig.palette), dark))) host.style.setProperty(name, value);
}

// The bar lives in a shadow root on its own element, so the page's styles
// cannot reach it and its styles cannot reach the page. The element is a layer
// over the whole page that lets clicks through, so nothing the page stacks
// high, a chat widget say, can cover the bar.
function start() {
  if (document.querySelector('lerd-debugbar')) return;
  addFonts();
  const host = document.createElement('lerd-debugbar');
  const shadow = host.attachShadow({ mode: 'open' });
  const sheet = new CSSStyleSheet();
  sheet.replaceSync(`:host { all: initial !important; position: fixed !important; inset: 0 !important; z-index: 2147483647 !important; pointer-events: none !important; }\n.lerd-root > * { pointer-events: auto; }\n${shadowSafe(appCss)}\n${barCss}\n.lerd-root { --font-sans: 'lerd Inter', ui-sans-serif, system-ui, sans-serif; }`);
  shadow.adoptedStyleSheets = [sheet];
  const root = document.createElement('div');
  root.className = 'lerd-root';
  shadow.appendChild(root);
  document.documentElement.appendChild(host);
  applyTheme(host, root);
  window.matchMedia('(prefers-color-scheme: dark)').addEventListener?.('change', () => applyTheme(host, root));

  // Everything the shared components reach for, pointed at what the bar has.
  setLayerRoot(root);
  setApiRewrite((p) => {
    if (p.startsWith('/api/requests/')) return barConfig.base + 'requests/' + p.slice('/api/requests/'.length);
    if (p.startsWith('/api/source?')) return barConfig.base + 'source' + p.slice('/api/source'.length);
    if (p === '/api/open-editor') return barConfig.base + 'open-editor';
    return barConfig.base + 'none';
  });
  editorAvailable.set(barConfig.local);
  sourceAvailable.set(barConfig.local);
  sites.set((barConfig.roots.length ? barConfig.roots : [barConfig.path]).map((path) => ({ name: path, path }) as Site));
  mount(Bar, { target: root, props: { rid, config: barConfig } });
}

if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', start, { once: true });
else start();
