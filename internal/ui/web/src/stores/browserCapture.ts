import { writable } from 'svelte/store';
import { apiFetch, apiJson } from '$lib/api';

// siteCaptureOn holds whether each site opted into browser capture, as last
// read or saved, so the lens and the settings modal agree.
export const siteCaptureOn = writable<Record<string, boolean>>({});

// BrowserCaptureEvent is a DOM event a site reports; message is a dot path
// into the event whose value becomes the message.
export interface BrowserCaptureEvent {
  event: string;
  label: string;
  message: string;
}

// BrowserCaptureSettings is one site's settings, kept in lerd's site registry.
export interface BrowserCaptureSettings {
  enabled: boolean;
  console: string[];
  network: string[];
  navigation: boolean;
  resources: boolean;
  events: BrowserCaptureEvent[];
  presets: Record<string, boolean>;
}

const sitePath = (site: string) => `/api/browser-capture/sites/${encodeURIComponent(site)}`;

export async function loadSiteBrowserCapture(site: string): Promise<BrowserCaptureSettings> {
  const s = await apiJson<BrowserCaptureSettings>(sitePath(site));
  siteCaptureOn.update((m) => ({ ...m, [site]: s.enabled }));
  return s;
}

export async function saveSiteBrowserCapture(site: string, s: BrowserCaptureSettings): Promise<BrowserCaptureSettings> {
  const res = await apiFetch(sitePath(site), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(s)
  });
  if (!res.ok) {
    throw new Error((await res.text()) || `saving browser capture settings failed (${res.status})`);
  }
  const saved = (await res.json()) as BrowserCaptureSettings;
  siteCaptureOn.update((m) => ({ ...m, [site]: saved.enabled }));
  return saved;
}

// BrowserCapturePreset is a store-published set of events for a frontend
// library, with whether the site uses that library and whether its events
// are reported.
export interface BrowserCapturePreset {
  name: string;
  label: string;
  events: BrowserCaptureEvent[];
  detected: boolean;
  active: boolean;
}

export function loadBrowserCapturePresets(site: string): Promise<BrowserCapturePreset[]> {
  return apiJson<BrowserCapturePreset[]>(`/api/browser-capture/presets?site=${encodeURIComponent(site)}`);
}

export async function setBrowserCapturePreset(site: string, name: string, on: boolean): Promise<BrowserCapturePreset[]> {
  const res = await apiFetch('/api/browser-capture/presets', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ site, name, on })
  });
  if (!res.ok) {
    throw new Error((await res.text()) || `applying preset failed (${res.status})`);
  }
  return (await res.json()) as BrowserCapturePreset[];
}
