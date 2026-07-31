import { m } from '../paraglide/messages.js';
import { writable } from 'svelte/store';
import { apiJson, apiFetch } from '$lib/api';

export interface RemoteControl {
  enabled: boolean;
  username: string;
  fullAccess: boolean;
  fullAccessLoading: boolean;
  loading: boolean;
  error: string;
}

const empty: RemoteControl = {
  enabled: false,
  username: '',
  fullAccess: false,
  fullAccessLoading: false,
  loading: false,
  error: ''
};

export const remoteControl = writable<RemoteControl>(empty);

interface RemoteControlResponse {
  enabled?: boolean;
  username?: string;
  full_access?: boolean;
  error?: string;
}

export async function loadRemoteControl() {
  try {
    const data = await apiJson<RemoteControlResponse>('/api/remote-control');
    remoteControl.set({
      ...empty,
      enabled: Boolean(data.enabled),
      username: data.username || '',
      fullAccess: Boolean(data.full_access)
    });
  } catch (e) {
    remoteControl.update((v) => ({
      ...v,
      error: e instanceof Error ? e.message : m.system_remote_loadFailed()
    }));
  }
}

export async function enableRemoteControl(username: string, password: string): Promise<{ ok: boolean; error?: string }> {
  remoteControl.update((v) => ({ ...v, loading: true, error: '' }));
  try {
    const res = await apiFetch('/api/remote-control', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ action: 'enable', username, password })
    });
    if (!res.ok) {
      const text = await res.text();
      remoteControl.update((v) => ({ ...v, loading: false, error: text || `HTTP ${res.status}` }));
      return { ok: false, error: text || `HTTP ${res.status}` };
    }
    const data = (await res.json()) as { ok?: boolean; error?: string };
    if (data.ok) {
      remoteControl.update((v) => ({ ...v, enabled: true, username, loading: false, error: '' }));
      return { ok: true };
    }
    remoteControl.update((v) => ({ ...v, loading: false, error: data.error || m.common_failed() }));
    return { ok: false, error: data.error };
  } catch (e) {
    const err = e instanceof Error ? e.message : m.common_requestFailed();
    remoteControl.update((v) => ({ ...v, loading: false, error: err }));
    return { ok: false, error: err };
  }
}

export async function disableRemoteControl(): Promise<boolean> {
  remoteControl.update((v) => ({ ...v, loading: true, error: '' }));
  try {
    const res = await apiFetch('/api/remote-control', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ action: 'disable' })
    });
    if (!res.ok) {
      const text = await res.text();
      remoteControl.update((v) => ({ ...v, loading: false, error: text || `HTTP ${res.status}` }));
      return false;
    }
    const data = (await res.json()) as { ok?: boolean; error?: string };
    if (data.ok) {
      remoteControl.set(empty);
      return true;
    }
    remoteControl.update((v) => ({ ...v, loading: false, error: data.error || m.common_failed() }));
    return false;
  } catch (e) {
    remoteControl.update((v) => ({ ...v, loading: false, error: e instanceof Error ? e.message : m.common_requestFailed() }));
    return false;
  }
}

// setRemoteFullAccess opts authenticated remote sessions into the host actions
// that are otherwise local-only. The backend accepts it from the local
// dashboard only, so a remote session cannot widen its own authority.
export async function setRemoteFullAccess(enabled: boolean): Promise<boolean> {
  remoteControl.update((v) => ({ ...v, fullAccessLoading: true, error: '' }));
  try {
    const res = await apiFetch('/api/remote-control', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ action: 'full-access', enabled })
    });
    if (!res.ok) {
      const text = await res.text();
      remoteControl.update((v) => ({ ...v, fullAccessLoading: false, error: text || `HTTP ${res.status}` }));
      return false;
    }
    const data = (await res.json()) as { ok?: boolean; full_access?: boolean; error?: string };
    if (data.ok) {
      remoteControl.update((v) => ({ ...v, fullAccess: Boolean(data.full_access), fullAccessLoading: false }));
      return true;
    }
    remoteControl.update((v) => ({ ...v, fullAccessLoading: false, error: data.error || m.common_failed() }));
    return false;
  } catch (e) {
    remoteControl.update((v) => ({
      ...v,
      fullAccessLoading: false,
      error: e instanceof Error ? e.message : m.common_requestFailed()
    }));
    return false;
  }
}
