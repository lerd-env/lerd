import { writable } from 'svelte/store';
import { apiFetch } from '$lib/api';

// Workspaces group sites for display only. Their names and order come from
// $status.workspaces and membership from each site's `workspace` field, both
// pushed over the websocket; this store only holds the API calls and the
// collapse state, which is view-only and stays in the browser.

export interface WorkspaceLayoutEntry {
  name: string;
  sites: string[];
}

export type WorkspaceResult = { ok: boolean; error?: string };

// The section key for sites in no workspace. Not a legal workspace name (the
// server trims and rejects empty), so it can never collide with one.
export const UNGROUPED = ' ungrouped';

async function send(path: string, method: 'POST' | 'PUT', body: unknown): Promise<WorkspaceResult> {
  try {
    const res = await apiFetch(path, {
      method,
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body)
    });
    const data = (await res.json()) as { ok?: boolean; error?: string };
    return { ok: Boolean(data.ok), error: data.error };
  } catch (e) {
    return { ok: false, error: e instanceof Error ? e.message : 'Request failed' };
  }
}

export const createWorkspace = (name: string) => send('/api/workspaces', 'POST', { name });

export const renameWorkspace = (old: string, next: string) =>
  send('/api/workspaces/rename', 'POST', { old, new: next });

export const deleteWorkspace = (name: string) => send('/api/workspaces/delete', 'POST', { name });

// An empty workspace ungroups the sites. Pass create to make a workspace that
// doesn't exist yet, so the picker's "New workspace…" is a single round trip.
export const assignSiteWorkspace = (sites: string[], workspace: string, create = false) =>
  send('/api/workspaces/assign', 'POST', { sites, workspace, create });

// saveWorkspaceLayout persists workspace order and membership in one write.
// siteOrder is only sent when the drag also changed the manual site order, so
// moving a whole workspace never rewrites the site registry.
export const saveWorkspaceLayout = (workspaces: WorkspaceLayoutEntry[], siteOrder?: string[]) =>
  send('/api/workspaces/layout', 'PUT', { workspaces, site_order: siteOrder ?? [] });

const KEY = 'lerd:workspaceCollapse';

function initial(): string[] {
  if (typeof localStorage === 'undefined') return [];
  try {
    const raw = JSON.parse(localStorage.getItem(KEY) ?? '[]');
    return Array.isArray(raw) ? raw.filter((v): v is string => typeof v === 'string') : [];
  } catch {
    return [];
  }
}

export const workspaceCollapse = writable<string[]>(initial());

workspaceCollapse.subscribe((v) => {
  try {
    if (typeof localStorage !== 'undefined') localStorage.setItem(KEY, JSON.stringify(v));
  } catch {
    // private mode / storage disabled — fall back to in-memory only.
  }
});

export function toggleWorkspaceCollapse(key: string) {
  workspaceCollapse.update((keys) => (keys.includes(key) ? keys.filter((k) => k !== key) : [...keys, key]));
}
