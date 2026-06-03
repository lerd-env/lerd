import { writable } from 'svelte/store';
import { apiJson, apiFetch } from '$lib/api';

// Global "run Horizon via horizon:listen" toggle. When on, lerd runs
// `php artisan horizon:listen --poll` so Horizon restarts its workers on file
// changes — no manual stop/restart while developing.
export const horizonReload = writable<boolean>(false);
export const horizonReloadLoading = writable<boolean>(false);

interface SettingsResponse {
  horizon_reload?: boolean;
}

export async function loadHorizonReload() {
  try {
    const res = await apiJson<SettingsResponse>('/api/settings');
    horizonReload.set(Boolean(res.horizon_reload));
  } catch {
    /* keep previous value */
  }
}

// setHorizonReload POSTs the new state and, on success, restarts a running
// Horizon worker so the change applies immediately. Returns { ok, error }.
export async function setHorizonReload(
  enabled: boolean
): Promise<{ ok: boolean; error?: string }> {
  horizonReloadLoading.set(true);
  try {
    const res = await apiFetch('/api/settings/horizon-reload', {
      method: 'POST',
      body: JSON.stringify({ enabled })
    });
    const data = (await res.json().catch(() => ({}))) as { ok?: boolean; error?: string };
    if (res.ok && data.ok !== false) {
      horizonReload.set(enabled);
      return { ok: true };
    }
    return { ok: false, error: data.error || `request failed (${res.status})` };
  } catch (e) {
    return { ok: false, error: e instanceof Error ? e.message : 'request failed' };
  } finally {
    horizonReloadLoading.set(false);
  }
}
