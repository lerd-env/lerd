import { writable, derived } from 'svelte/store';
import { apiFetch, apiJson, decodeJSONResult } from '$lib/api';
import { wsMessage } from '$lib/ws';
import { version } from './version';

export interface PHPStatus {
  version: string;
  // Full build (e.g. "8.5.8"), filled in asynchronously by the backend. Falls
  // back to the minor `version` until the probe lands.
  patch?: string;
  running: boolean;
  xdebug_enabled: boolean;
  xdebug_mode?: string;
  ports?: string[];
  // True when the prebuilt base this image was built from has been republished
  // upstream, so a rebuild would bring in a newer PHP/Alpine.
  update_available?: boolean;
}

export interface StatusResponse {
  dns: { ok: boolean; status?: 'ok' | 'degraded' | 'down'; vpn?: boolean; enabled: boolean; tld: string };
  nginx: { running: boolean };
  php_fpms: PHPStatus[];
  php_default: string;
  node_default: string;
  node_managed_by_lerd: boolean;
  // Active Node version manager lerd drives: 'fnm' (bundled) or 'nvm' (user's).
  node_manager: 'fnm' | 'nvm';
  // True when a user-installed nvm is present (so the nvm switch can be offered).
  nvm_available: boolean;
  bun_available: boolean;
  bun_version: string;
  using_system_bun: boolean;
  watcher_running: boolean;
  frankenphp_php_versions: string[];
  home: string;
  // Workspace names in display order, empty ones included.
  workspaces?: string[];
  // Identifier of the lerd-ui process that answered. A change means the server
  // restarted, so the page is reloaded onto the assets it now serves.
  instance?: string;
  // Managed host binaries (composer, fnm, mkcert) against their pinned versions.
  tools?: ToolStatus[];
}

export interface ToolStatus {
  name: string;
  installed?: string;
  pinned: string;
  present: boolean;
  update_available: boolean;
}

const empty: StatusResponse = {
  dns: { ok: false, status: 'down', vpn: false, enabled: true, tld: 'test' },
  nginx: { running: false },
  php_fpms: [],
  php_default: '',
  node_default: '',
  node_managed_by_lerd: true,
  node_manager: 'fnm',
  nvm_available: false,
  bun_available: false,
  bun_version: '',
  using_system_bun: false,
  watcher_running: false,
  frankenphp_php_versions: [],
  home: '',
  workspaces: [],
  tools: []
};

export const status = writable<StatusResponse>(empty);
export const statusLoaded = writable<boolean>(false);

export async function loadStatus() {
  try {
    applyStatus(await apiJson<StatusResponse>('/api/status'));
  } catch {
    /* keep previous */
  }
}

let serverInstance: string | null = null;

// noteInstance reloads the page when the server that answers is a different
// process than the one this page loaded from. A restarted lerd-ui otherwise
// leaves an open dashboard running the previous build's assets against it.
function noteInstance(instance: string | undefined, reload: () => void) {
  if (!instance) return;
  if (serverInstance === null) {
    serverInstance = instance;
    return;
  }
  if (serverInstance !== instance) {
    serverInstance = instance;
    reload();
  }
}

const reloadPage = () => {
  if (typeof location !== 'undefined') location.reload();
};

export function applyStatus(data: unknown, reload: () => void = reloadPage) {
  if (!data || typeof data !== 'object') return;
  const next = data as StatusResponse;
  status.set({ ...empty, ...next });
  statusLoaded.set(true);
  noteInstance(next.instance, reload);
}

wsMessage.subscribe((msg) => {
  if (msg?.status) applyStatus(msg.status);
});

export type DnsState = 'ok' | 'degraded' | 'down';

// dnsState collapses the payload into a three-way health value. It tolerates
// older payloads without the `status` field by deriving it from `ok`, and
// treats lerd-managed DNS being disabled as healthy since the system
// resolver owns *.tld in that mode. "degraded" means lerd-dns answers fine
// but the system resolver isn't routing to it, typically a VPN client.
export function dnsState(s: StatusResponse): DnsState {
  if (s.dns.enabled === false) return 'ok';
  return s.dns.status ?? (s.dns.ok ? 'ok' : 'down');
}

export type LerdStatusColor = 'green' | 'yellow' | 'red' | 'gray';

export const lerdStatusColor = derived([status, statusLoaded, version], ([$s, $loaded, $v]): LerdStatusColor => {
  if (!$loaded) return 'gray';
  const dns = dnsState($s);
  if (dns === 'down' || !$s.nginx.running || !$s.watcher_running) return 'red';
  if (dns === 'degraded' || $v.hasUpdate) return 'yellow';
  return 'green';
});

export const allCoreRunning = derived(status, ($s): boolean => {
  return Boolean(
    dnsState($s) !== 'down' &&
      $s.nginx.running &&
      ($s.php_fpms || []).every((f) => f.running)
  );
});

// updateTool applies one managed host tool's pinned version. One tool per call:
// a download whose checksum does not match is refused deliberately, and the card
// that asked has to name that tool rather than show a failure for all of them.
export async function updateTool(name: string): Promise<{ ok: boolean; error?: string }> {
  try {
    const res = await apiFetch(`/api/tools/${encodeURIComponent(name)}/update`, { method: 'POST' });
    return await decodeJSONResult<{ ok: boolean; error?: string }>(res);
  } catch (e) {
    return { ok: false, error: e instanceof Error ? e.message : String(e) };
  }
}

// checkToolUpdates re-reads the published pins with the 24h cache bypassed.
// The pins live in one manifest, so this is a single check for every tool
// rather than one per card.
export async function checkToolUpdates(): Promise<{ ok: boolean; tools?: ToolStatus[] }> {
  try {
    const res = await apiFetch('/api/tools/check', { method: 'POST' });
    const out = await decodeJSONResult<{ ok?: boolean; tools?: ToolStatus[] }>(res);
    return { ok: out.ok === true, tools: out.tools };
  } catch {
    return { ok: false };
  }
}
