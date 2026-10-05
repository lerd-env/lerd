import { writable } from 'svelte/store';
import { apiJson } from '$lib/api';

// How the debug bar looks on every site that shows it, and whether one site does.
export interface DebugbarSettings {
  style: 'dock' | 'compact';
  edge: 'bottom' | 'top';
  corner: 'bottom-right' | 'bottom-left' | 'top-right' | 'top-left';
  theme: 'auto' | 'light' | 'dark';
}

export const debugbarSettings = writable<DebugbarSettings | null>(null);

export async function loadDebugbarSettings(): Promise<void> {
  try {
    debugbarSettings.set(await apiJson<DebugbarSettings>('/api/debugbar/settings'));
  } catch {
    /* keep what we had */
  }
}

export async function saveDebugbarSettings(s: DebugbarSettings): Promise<void> {
  debugbarSettings.set(await apiJson<DebugbarSettings>('/api/debugbar/settings', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(s) }));
}

export interface DebugbarSite {
  enabled: boolean;
  source: 'registry' | 'lerd.yaml';
}

export const loadDebugbarSite = (site: string) => apiJson<DebugbarSite>(`/api/debugbar/sites/${encodeURIComponent(site)}`);

export const setDebugbarSite = (site: string, enable: boolean) =>
  apiJson<DebugbarSite>(`/api/debugbar/sites/${encodeURIComponent(site)}`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ enable }) });
