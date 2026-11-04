import { writable } from 'svelte/store';
import { apiJson, apiFetch } from '$lib/api';

export const nodeVersions = writable<string[]>([]);

export interface NodeVersionStatus {
  major: string;
  installed: string;
  latest?: string;
  update_available: boolean;
}

// Fuller than nodeVersions and slower, since it can reach nodejs.org, so it
// follows the list rather than holding it up.
export const nodeVersionStatuses = writable<NodeVersionStatus[]>([]);

async function loadNodeVersionStatuses() {
  try {
    const list = await apiJson<NodeVersionStatus[]>('/api/node-versions/status');
    nodeVersionStatuses.set(Array.isArray(list) ? list : []);
  } catch {
    /* keep previous */
  }
}

export async function loadNodeVersions() {
  try {
    const list = await apiJson<string[]>('/api/node-versions');
    nodeVersions.set(Array.isArray(list) ? list : []);
  } catch {
    /* keep previous */
  }
  void loadNodeVersionStatuses();
}

// checkNodeUpdates re-reads the published releases with the day-long cache
// bypassed, so a new patch shows up without waiting it out.
export async function checkNodeUpdates(): Promise<{ ok: boolean; versions?: NodeVersionStatus[] }> {
  try {
    const res = await apiFetch('/api/node-versions/check', { method: 'POST' });
    const out = (await res.json()) as { ok?: boolean; versions?: NodeVersionStatus[] };
    if (out.versions) nodeVersionStatuses.set(out.versions);
    return { ok: out.ok === true, versions: out.versions };
  } catch {
    return { ok: false };
  }
}

// updateNodeVersion installs the newest patch of one major. It answers rather
// than throws, so the card or dialog that asked can name what failed.
export async function updateNodeVersion(major: string): Promise<{ ok: boolean; error?: string }> {
  try {
    const res = await apiFetch('/api/node-versions/' + encodeURIComponent(major) + '/update', {
      method: 'POST'
    });
    return (await res.json()) as { ok: boolean; error?: string };
  } catch (e) {
    return { ok: false, error: e instanceof Error ? e.message : String(e) };
  }
}

export interface NodeUpdateResult {
  name: string;
  ok: boolean;
  error?: string;
}

// updateNodeVersions runs one at a time, like `lerd node:update`, and a failure
// costs only that version. The batch is already confirmed.
export async function updateNodeVersions(
  majors: string[],
  onresult?: (result: NodeUpdateResult) => void,
  onstart?: (major: string) => void
): Promise<NodeUpdateResult[]> {
  const results: NodeUpdateResult[] = [];
  for (const name of majors) {
    onstart?.(name);
    const result = { name, ...(await updateNodeVersion(name)) };
    results.push(result);
    onresult?.(result);
  }
  await loadNodeVersions();
  return results;
}

export async function setDefaultNode(v: string): Promise<boolean> {
  try {
    const res = await apiFetch('/api/node-versions/' + encodeURIComponent(v) + '/set-default', {
      method: 'POST'
    });
    // The handler answers a refused change with a 200, so the status says nothing.
    const out = (await res.json().catch(() => ({}))) as { ok?: boolean };
    if (out.ok) await loadNodeVersions();
    return out.ok === true;
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

// setNodeManager switches the Node version manager lerd drives (mise/fnm/nvm). Unlike
// manage/unmanage it parses the JSON body, because switching can legitimately
// fail (e.g. nvm not installed) and the handler reports that as { ok:false,
// error } with a 200 status, so the caller can surface the reason.
export async function setNodeManager(
  manager: 'mise' | 'fnm' | 'nvm'
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
