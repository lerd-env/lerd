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
