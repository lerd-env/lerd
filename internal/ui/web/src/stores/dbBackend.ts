import { writable } from 'svelte/store';
import { apiJson, apiFetch } from '$lib/api';
import { hostMysqlProbe, loadServices, type HostMySQLStatus } from './services';

export type DBBackend = 'container' | 'host';

// Shared host-MySQL probe result, so the per-site BackendSwitch and the
// HostSetupCallout read one probe instead of each firing their own. null means
// "not probed yet / unknown" — callers treat that as reachable and let the
// server be the source of truth.
export const hostMysql = writable<HostMySQLStatus | null>(null);

export async function refreshHostMysql() {
  hostMysql.set(await hostMysqlProbe());
}

// setMysqlPublishedPort moves lerd-mysql's published host port so a host system
// MySQL can keep 127.0.0.1:3306. port=0 resets to the default. On success it
// refreshes the probe + service list so the coexistence UI reflects reality.
// Loopback-only server-side (it rebinds a host port).
export async function setMysqlPublishedPort(
  port: number
): Promise<{ ok: boolean; error?: string }> {
  try {
    const res = await apiFetch(
      `/api/services/mysql/port?port=${encodeURIComponent(String(port))}`,
      { method: 'POST' }
    );
    const data = (await res.json()) as { ok?: boolean; error?: string };
    if (res.ok && data.ok) {
      await Promise.all([refreshHostMysql(), loadServices()]);
    }
    return { ok: Boolean(data.ok), error: data.error };
  } catch (e) {
    return { ok: false, error: e instanceof Error ? e.message : 'Request failed' };
  }
}

// Global default DB backend: the one new sites adopt and the target of the
// MySQL service page's "switch all sites" control. 'container' is lerd's own
// MySQL; 'host' is the system MySQL reached over its unix socket.
export const defaultDBBackend = writable<DBBackend>('container');

interface SettingsResponse {
  default_db_backend?: string;
}

export async function loadDefaultBackend() {
  try {
    const res = await apiJson<SettingsResponse>('/api/settings');
    defaultDBBackend.set(res.default_db_backend === 'host' ? 'host' : 'container');
  } catch {
    /* keep previous */
  }
}

// saveDefaultBackend persists the global default. With applyAll the server also
// re-points every MySQL/MariaDB site to the chosen backend in one pass and
// reports how many were applied. Only updates the store on success.
export async function saveDefaultBackend(
  backend: DBBackend,
  applyAll = false
): Promise<{ ok: boolean; error?: string; applied?: number }> {
  try {
    const res = await apiFetch('/api/settings/default-backend', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ backend, apply_all: applyAll })
    });
    const data = (await res.json()) as {
      ok?: boolean;
      error?: string;
      applied?: number;
      default_db_backend?: string;
    };
    // Reconcile from the server's echoed value, which the handler returns on every
    // branch where the default was actually persisted — including partial-failure
    // (apply_all switched some sites then errored). This keeps the toggle honest
    // instead of leaving it stale when ok is false but the default was saved.
    if (data.default_db_backend === 'host' || data.default_db_backend === 'container') {
      defaultDBBackend.set(data.default_db_backend);
    } else if (res.ok && data.ok) {
      defaultDBBackend.set(backend);
    }
    return { ok: Boolean(data.ok), error: data.error, applied: data.applied };
  } catch (e) {
    return { ok: false, error: e instanceof Error ? e.message : 'Request failed' };
  }
}
