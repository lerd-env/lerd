import { apiFetch } from './api';

// A plugin page runs in a sandboxed frame with no origin of its own, so it asks
// the dashboard to call its endpoints for it (see the skeleton's ui/lerd.js).
// The dashboard only ever forwards to /api/plugins/<that plugin>/.

interface PluginRequest {
  type: 'lerd:api';
  id: number;
  method?: string;
  path: string;
  body?: unknown;
}

type Fetcher = (path: string, init?: RequestInit) => Promise<Response>;

// The endpoint path rule plugin.yaml is validated against; with no dot or
// percent sign allowed, nothing can climb out of the plugin's prefix.
const ENDPOINT = /^[a-z0-9][a-z0-9/_-]*$/;

// pluginApiPath resolves a page's relative path to its plugin's API, or null
// when it is not a path an endpoint could have.
export function pluginApiPath(plugin: string, path: string): string | null {
  const q = path.indexOf('?');
  const endpoint = q < 0 ? path : path.slice(0, q);
  if (!ENDPOINT.test(endpoint)) return null;
  return `/api/plugins/${plugin}/${endpoint}${q < 0 ? '' : path.slice(q)}`;
}

export async function answerPluginRequest(
  e: MessageEvent,
  plugin: string,
  frame: Window | null | undefined,
  fetcher: Fetcher = apiFetch
): Promise<void> {
  const msg = e.data as PluginRequest;
  if (!frame || e.source !== frame || !msg || msg.type !== 'lerd:api') return;
  const reply = (status: number, body: string) =>
    frame.postMessage({ type: 'lerd:api:result', id: msg.id, status, body }, '*');

  const path = pluginApiPath(plugin, msg.path);
  if (!path) {
    reply(403, 'path outside this plugin: ' + msg.path);
    return;
  }
  const init: RequestInit = { method: (msg.method || 'GET').toUpperCase() };
  if (msg.body !== undefined) {
    init.body = typeof msg.body === 'string' ? msg.body : JSON.stringify(msg.body);
    init.headers = { 'Content-Type': 'application/json' };
  }
  try {
    const res = await fetcher(path, init);
    reply(res.status, await res.text());
  } catch (err) {
    reply(502, err instanceof Error ? err.message : String(err));
  }
}

// Two versions of a file a page wants shown in the dashboard's own diff, with
// a name for each side, or with lerd:view one file on its own, read only, which
// leaves original out. Each version is capped so a page cannot stall the editor.
export interface PluginDiff {
  path: string;
  original?: string;
  modified: string;
  before?: string;
  after?: string;
}

const DIFF_MAX = 2 * 1024 * 1024;

export function pluginDiffRequest(e: MessageEvent, frame: Window | null | undefined): PluginDiff | null {
  const msg = e.data;
  if (!frame || e.source !== frame || !msg) return null;
  const text = (v: unknown, max: number) => typeof v === 'string' && v.length <= max;
  const label = (v: unknown) => (typeof v === 'string' && v ? v.slice(0, 80) : undefined);
  if (msg.type === 'lerd:view') {
    if (!text(msg.path, 512) || !msg.path || !text(msg.content, DIFF_MAX)) return null;
    return { path: msg.path, modified: msg.content, after: label(msg.label) };
  }
  if (msg.type !== 'lerd:diff') return null;
  if (!text(msg.path, 512) || !msg.path || !text(msg.original, DIFF_MAX) || !text(msg.modified, DIFF_MAX)) return null;
  return { path: msg.path, original: msg.original, modified: msg.modified, before: label(msg.before), after: label(msg.after) };
}

// A file a page hands to the dashboard's editor; the id is how the saved text,
// or null for a cancel, finds its way back to the page.
export interface PluginEdit {
  id: number;
  path: string;
  content: string;
  label?: string;
}

export function pluginEditRequest(e: MessageEvent, frame: Window | null | undefined): PluginEdit | null {
  const msg = e.data;
  if (!frame || e.source !== frame || !msg || msg.type !== 'lerd:edit') return null;
  if (!Number.isFinite(msg.id) || typeof msg.path !== 'string' || !msg.path || msg.path.length > 512) return null;
  if (typeof msg.content !== 'string' || msg.content.length > DIFF_MAX) return null;
  const label = typeof msg.label === 'string' && msg.label ? msg.label.slice(0, 80) : undefined;
  return { id: msg.id, path: msg.path, content: msg.content, label };
}

// The dashboard tokens a plugin page's base stylesheet reads, by the name the
// page knows them under.
const THEME_VARS: Record<string, string> = {
  '--lerd-accent': '--color-lerd-red',
  '--lerd-accent-hover': '--color-lerd-redhov',
  '--lerd-on-accent': '--color-lerd-onred',
  '--lerd-border': '--color-lerd-border',
  '--lerd-dark-card': '--color-lerd-card',
  // A theme's light card and border, which the dashboard keeps under these.
  '--lerd-light-card': '--color-white',
  '--lerd-light-border': '--color-gray-200'
};

// pluginTheme is what a plugin page needs to look like the dashboard around it.
export function pluginTheme(root: HTMLElement = document.documentElement) {
  const style = getComputedStyle(root);
  const vars: Record<string, string> = {};
  for (const [name, token] of Object.entries(THEME_VARS)) {
    const value = style.getPropertyValue(token).trim();
    if (value) vars[name] = value;
  }
  return { type: 'lerd:theme', dark: root.classList.contains('dark'), vars };
}
