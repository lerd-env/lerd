import { writable } from 'svelte/store';
import { apiFetch, apiJson } from '$lib/api';

// browserCaptureEnabled mirrors the global browser capture toggle.
export const browserCaptureEnabled = writable<boolean>(false);
// browserCaptureKnown turns true once the status has been read, so a view does
// not act on the default before the real answer is in.
export const browserCaptureKnown = writable<boolean>(false);

export async function loadBrowserCaptureStatus(): Promise<void> {
  try {
    const s = await apiJson<{ enabled: boolean }>('/api/browser-capture/status');
    browserCaptureEnabled.set(Boolean(s.enabled));
    browserCaptureKnown.set(true);
  } catch {
    /* keep previous value */
  }
}

// setBrowserCapture turns the script injection on or off for every PHP-FPM site.
export async function setBrowserCapture(enable: boolean): Promise<void> {
  const res = await apiFetch('/api/browser-capture/toggle', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ enable })
  });
  if (!res.ok) {
    throw new Error((await res.text()) || `browser capture toggle failed (${res.status})`);
  }
  const data = (await res.json()) as { enabled: boolean };
  browserCaptureEnabled.set(Boolean(data.enabled));
}

// BrowserCaptureEvent is a DOM event a site reports; message is a dot path
// into the event whose value becomes the message.
export interface BrowserCaptureEvent {
  event: string;
  label: string;
  message: string;
}

// BrowserCaptureSettings is one site's settings; source says whether they are
// kept in the project's .lerd.yaml or in lerd's own site registry.
export interface BrowserCaptureSettings {
  enabled: boolean;
  console: string[];
  network: string[];
  navigation: boolean;
  resources: boolean;
  events: BrowserCaptureEvent[];
  presets: string[];
  verbose: boolean;
  route: string;
  source?: 'lerd.yaml' | 'registry';
}

const sitePath = (site: string) => `/api/browser-capture/sites/${encodeURIComponent(site)}`;

export function loadSiteBrowserCapture(site: string): Promise<BrowserCaptureSettings> {
  return apiJson<BrowserCaptureSettings>(sitePath(site));
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
  return (await res.json()) as BrowserCaptureSettings;
}

// BrowserCapturePreset is a store-published set of events for a frontend
// library, with whether the site uses that library and has its events.
export interface BrowserCapturePreset {
  name: string;
  label: string;
  events: BrowserCaptureEvent[];
  detected: boolean;
  applied: boolean;
}

export function loadBrowserCapturePresets(site: string): Promise<BrowserCapturePreset[]> {
  return apiJson<BrowserCapturePreset[]>(`/api/browser-capture/presets?site=${encodeURIComponent(site)}`);
}

export async function applyBrowserCapturePreset(site: string, name: string, add: boolean): Promise<BrowserCapturePreset[]> {
  const res = await apiFetch('/api/browser-capture/presets', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ site, name, add })
  });
  if (!res.ok) {
    throw new Error((await res.text()) || `applying preset failed (${res.status})`);
  }
  return (await res.json()) as BrowserCapturePreset[];
}
