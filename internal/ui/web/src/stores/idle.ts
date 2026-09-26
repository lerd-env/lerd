import { writable } from 'svelte/store';
import { apiJson, apiFetch } from '$lib/api';

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
    const res = await apiJson<SettingsResponse>('/api/settings');
    idleEnabled.set(Boolean(res.idle_suspend_enabled));
    idleServices.set(Boolean(res.idle_suspend_services));
    if (typeof res.idle_suspend_timeout_minutes === 'number' && res.idle_suspend_timeout_minutes > 0) {
      idleTimeoutMinutes.set(res.idle_suspend_timeout_minutes);
    }
  } catch {
    /* keep previous */
  }
}

// services is left as it is when omitted.
export async function saveIdle(enabled: boolean, timeoutMinutes: number, services?: boolean): Promise<boolean> {
  try {
    const res = await apiFetch('/api/settings/idle-suspend', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ enabled, timeout_minutes: timeoutMinutes, services })
    });
    if (res.ok) {
      idleEnabled.set(enabled);
      idleTimeoutMinutes.set(timeoutMinutes);
      if (services !== undefined) idleServices.set(services);
    }
    return res.ok;
  } catch {
    return false;
  }
}
