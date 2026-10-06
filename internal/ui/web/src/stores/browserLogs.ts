import { writable } from 'svelte/store';
import { apiFetch, apiJson } from '$lib/api';

// siteCaptureOn holds whether each site opted into browser logs, as last
// read or saved, so the lens and the settings modal agree.
export const siteCaptureOn = writable<Record<string, boolean>>({});

// BrowserLogsEvent is a DOM event a site reports; message is a dot path
// into the event whose value becomes the message.
export interface BrowserLogsEvent {
  event: string;
  label: string;
  message: string;
}

// BrowserLogsSettings is one site's settings, kept in lerd's site registry.
export interface BrowserLogsSettings {
  enabled: boolean;
  console: string[];
  network: string[];
  navigation: boolean;
  resources: boolean;
  events: BrowserLogsEvent[];
  presets: Record<string, boolean>;
}

const sitePath = (site: string) => `/api/browser-logs/sites/${encodeURIComponent(site)}`;

export async function loadSiteBrowserLogs(site: string): Promise<BrowserLogsSettings> {
  const s = await apiJson<BrowserLogsSettings>(sitePath(site));
  siteCaptureOn.update((m) => ({ ...m, [site]: s.enabled }));
  return s;
}

export async function saveSiteBrowserLogs(site: string, s: BrowserLogsSettings): Promise<BrowserLogsSettings> {
  const res = await apiFetch(sitePath(site), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(s)
  });
  if (!res.ok) {
    throw new Error((await res.text()) || `saving browser logs settings failed (${res.status})`);
  }
  const saved = (await res.json()) as BrowserLogsSettings;
  siteCaptureOn.update((m) => ({ ...m, [site]: saved.enabled }));
  return saved;
}

// BrowserLogsPreset is a store-published set of events for a frontend
// library, with whether the site uses that library and whether its events
// are reported.
export interface BrowserLogsPreset {
  name: string;
  label: string;
  events: BrowserLogsEvent[];
  detected: boolean;
  active: boolean;
}

export function loadBrowserLogsPresets(site: string): Promise<BrowserLogsPreset[]> {
  return apiJson<BrowserLogsPreset[]>(`/api/browser-logs/presets?site=${encodeURIComponent(site)}`);
}

export async function setBrowserLogsPreset(site: string, name: string, on: boolean): Promise<BrowserLogsPreset[]> {
  const res = await apiFetch('/api/browser-logs/presets', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ site, name, on })
  });
  if (!res.ok) {
    throw new Error((await res.text()) || `applying preset failed (${res.status})`);
  }
  return (await res.json()) as BrowserLogsPreset[];
}
