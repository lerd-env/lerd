import { writable } from 'svelte/store';
import { apiJson, apiFetch } from '$lib/api';

export const nodeVersions = writable<string[]>([]);

export async function loadNodeVersions() {
  try {
    const list = await apiJson<string[]>('/api/node-versions');
    nodeVersions.set(Array.isArray(list) ? list : []);
  } catch {
    /* keep previous */
  }
}

export async function setDefaultNode(v: string): Promise<boolean> {
  try {
    const res = await apiFetch('/api/node-versions/' + encodeURIComponent(v) + '/set-default', {
      method: 'POST'
    });
    if (res.ok) await loadNodeVersions();
    return res.ok;
  } catch {
    return false;
  }
}

export async function removeNode(v: string): Promise<boolean> {
  try {
    const res = await apiFetch('/api/node-versions/' + encodeURIComponent(v) + '/remove', {
      method: 'POST'
    });
    if (res.ok) await loadNodeVersions();
    return res.ok;
  } catch {
    return false;
  }
}

export async function manageNode(): Promise<boolean> {
  try {
    const res = await apiFetch('/api/node/manage', { method: 'POST' });
    return res.ok;
  } catch {
    return false;
  }
}

export async function unmanageNode(): Promise<boolean> {
  try {
    const res = await apiFetch('/api/node/unmanage', { method: 'POST' });
    return res.ok;
  } catch {
    return false;
  }
}

// setNodeManager switches the Node version manager lerd drives (fnm/nvm). Unlike
// manage/unmanage it parses the JSON body, because switching can legitimately
// fail (e.g. nvm not installed) and the handler reports that as { ok:false,
// error } with a 200 status, so the caller can surface the reason.
export async function setNodeManager(
  manager: 'fnm' | 'nvm'
): Promise<{ ok: boolean; error?: string }> {
  try {
    const res = await apiFetch('/api/node/set-manager', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ manager })
    });
    const data = (await res.json().catch(() => ({}))) as { ok?: boolean; error?: string };
    if (res.ok && data?.ok) {
      await loadNodeVersions();
      return { ok: true };
    }
    return { ok: false, error: data?.error };
  } catch (e) {
    return { ok: false, error: e instanceof Error ? e.message : undefined };
  }
}

export async function installNode(v: string): Promise<boolean> {
  try {
    const res = await apiFetch('/api/node-versions/install', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ version: v })
    });
    if (res.ok) await loadNodeVersions();
    return res.ok;
  } catch {
    return false;
  }
}
