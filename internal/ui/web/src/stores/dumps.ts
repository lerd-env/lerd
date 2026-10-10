import { writable } from 'svelte/store';
import { apiFetch, apiJson } from '$lib/api';
import { wsMessage } from '$lib/ws';

export interface DumpsStatus {
  enabled: boolean;
  passthrough: boolean;
  listening: boolean;
  addr: string;
  count: number;
  // capacity is how many events the buffer keeps before the oldest go.
  capacity?: number;
  subscribers: number;
  last_ts: string;
}

export const status = writable<DumpsStatus | null>(null);
export const filterSite = writable<string>('');
export const filterCtx = writable<'' | 'fpm' | 'cli'>('');
export const filterText = writable<string>('');

// lastFlashId names the dump that just arrived, which DumpEntry highlights
// for a moment; the Dumps lens sets it when a refresh brings a new one.
export const lastFlashId = writable<string>('');

const FLASH_DURATION_MS = 2500;
let flashTimer: ReturnType<typeof setTimeout> | null = null;

export function flashDump(id: string): void {
  lastFlashId.set(id);
  if (flashTimer) clearTimeout(flashTimer);
  flashTimer = setTimeout(() => lastFlashId.set(''), FLASH_DURATION_MS);
}

export async function refreshStatus(): Promise<void> {
  try {
    const data = await apiJson<DumpsStatus>('/api/dumps/status');
    status.set(data);
  } catch {
    status.set(null);
  }
}

// Live-update from WS so any out-of-band toggle (CLI, tray, MCP, another
// browser tab) is reflected without a manual refresh.
wsMessage.subscribe((msg) => {
  const fresh = msg?.dumps_status as DumpsStatus | undefined;
  if (fresh) status.set(fresh);
});

// clearDumps empties the buffer, or with a kind only that kind's events.
export async function clearDumps(kind?: string): Promise<void> {
  await apiFetch(kind ? `/api/dumps/clear?kind=${encodeURIComponent(kind)}` : '/api/dumps/clear', { method: 'POST' });
  void refreshStatus();
}

export async function toggleDumps(enable: boolean): Promise<void> {
  await apiFetch('/api/dumps/toggle', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ enable })
  });
  void refreshStatus();
}

export interface PassthroughResult {
  passthrough: boolean;
  no_change?: boolean;
  restarted?: string[];
}

// togglePassthrough flips the response-passthrough flag and triggers a
// restart of every installed PHP-FPM container so the new ini value
// takes effect. This is the only dumps path that intentionally
// restarts FPM; enable/disable is restart-free.
export async function togglePassthrough(enable: boolean): Promise<PassthroughResult> {
  const res = await apiFetch('/api/dumps/passthrough', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ enable })
  });
  const out = (await res.json()) as PassthroughResult;
  void refreshStatus();
  return out;
}

// setDumpsBuffer resizes the buffer lerd-ui keeps for the Debug window, in
// place: the newest events that fit stay.
export async function setDumpsBuffer(size: number): Promise<void> {
  const res = await apiFetch('/api/dumps/buffer', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ size })
  });
  if (!res.ok) throw new Error((await res.text()) || `buffer resize failed (${res.status})`);
  status.set((await res.json()) as DumpsStatus);
}

// Derived list of unique site names seen in the buffered events, for the
// filter dropdown. Sites without explicit names (e.g. when DOCUMENT_ROOT is
// unusual) appear as "(unknown)".
