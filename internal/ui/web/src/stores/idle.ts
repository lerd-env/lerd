import { writable } from 'svelte/store';
import { apiFetch, decodeJSONResult, sharedJson } from '$lib/api';

// Global idle-suspend policy (a single on/off + timeout, not per site).
export const idleEnabled = writable<boolean>(false);
export const idleTimeoutMinutes = writable<number>(30);
export const idleServices = writable<boolean>(false);

interface SettingsResponse {
  idle_suspend_enabled?: boolean;
  idle_suspend_timeout_minutes?: number;
  idle_suspend_services?: boolean;
}

export async function loadIdle() {
  try {
    const res = await sharedJson<SettingsResponse>('/api/settings');
    idleEnabled.set(Boolean(res.idle_suspend_enabled));
    idleServices.set(Boolean(res.idle_suspend_services));
    if (typeof res.idle_suspend_timeout_minutes === 'number' && res.idle_suspend_timeout_minutes > 0) {
      idleTimeoutMinutes.set(res.idle_suspend_timeout_minutes);
    }
  } catch {
    /* keep previous */
  }
}

export interface IdleChange {
  enabled?: boolean;
  timeout_minutes?: number;
  services?: boolean;
}

// Saves only what changed: the timeout read here is rounded to whole minutes, so
// sending it back would rewrite one set to the second.
export async function saveIdle(change: IdleChange): Promise<boolean> {
  try {
    const res = await apiFetch('/api/settings/idle-suspend', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(change)
    });
    // A save that failed on the host still answers 200, with ok false.
    const ok = res.ok && (await decodeJSONResult<{ ok?: boolean }>(res)).ok === true;
    if (ok) {
      if (change.enabled !== undefined) idleEnabled.set(change.enabled);
      if (change.timeout_minutes !== undefined) idleTimeoutMinutes.set(change.timeout_minutes);
      if (change.services !== undefined) idleServices.set(change.services);
    }
    return ok;
  } catch {
    return false;
  }
}
