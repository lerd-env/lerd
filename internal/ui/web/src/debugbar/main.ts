import { mount } from 'svelte';
import appCss from '../app.css?inline';
import barCss from './bar.css?inline';
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
  const host = document.createElement('lerd-debugbar');
  const shadow = host.attachShadow({ mode: 'open' });
  const sheet = new CSSStyleSheet();
  sheet.replaceSync(`:host { all: initial !important; position: fixed !important; inset: 0 !important; z-index: 2147483647 !important; pointer-events: none !important; }\n.lerd-root > * { pointer-events: auto; }\n${shadowSafe(appCss)}\n${barCss}`);
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
    if (p.startsWith('/api/annotations')) return barConfig.base + 'annotations' + p.slice('/api/annotations'.length);
    return barConfig.base + 'none';
  });
  editorAvailable.set(barConfig.local);
  sourceAvailable.set(barConfig.local);
  sites.set((barConfig.roots.length ? barConfig.roots : [barConfig.path]).map((path) => ({ name: path, path }) as Site));
  mount(Bar, { target: root, props: { rid, config: barConfig } });
}

if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', start, { once: true });
else start();
