import { m } from '../paraglide/messages.js';
import { writable } from 'svelte/store';
import { apiJson, apiFetch } from '$lib/api';

export interface DiskImage {
  id: string;
  desc: string;
  bytes: number;
}

export interface DiskSnapshot {
  available: boolean;
  reclaimable_bytes: number;
  images: DiskImage[];
  held_bytes: number;
  held_count: number;
}

const empty: DiskSnapshot = {
  available: false,
  reclaimable_bytes: 0,
  images: [],
  held_bytes: 0,
  held_count: 0
};

export const disk = writable<DiskSnapshot>(empty);
export const diskLoaded = writable<boolean>(false);

// Reclaimable disk comes from a full podman image scan, far heavier than the
// per-container stats poll, so it refreshes on its own slower cadence.
const POLL_INTERVAL_MS = 30000;

let pollTimer: ReturnType<typeof setInterval> | null = null;
let inflight = false;

export async function loadDisk(): Promise<void> {
  if (inflight) return;
  inflight = true;
  try {
    const res = await apiJson<DiskSnapshot>('/api/disk');
    disk.set({ ...empty, ...res });
    diskLoaded.set(true);
  } catch {
    /* keep previous */
  } finally {
    inflight = false;
  }
}

// startDiskPolling mirrors the stats loop: an immediate fetch, then a slow poll
// gated by the Page Visibility API so a backgrounded tab stops scanning images.
export function startDiskPolling(): () => void {
  let stopped = false;
  function tick() {
    if (stopped) return;
    if (typeof document !== 'undefined' && document.hidden) return;
    void loadDisk();
  }
  tick();
  pollTimer = setInterval(tick, POLL_INTERVAL_MS);

  const onVisibility = () => {
    if (typeof document === 'undefined') return;
    if (!document.hidden) tick();
  };
  if (typeof document !== 'undefined') {
    document.addEventListener('visibilitychange', onVisibility);
  }

  return () => {
    stopped = true;
    if (pollTimer) {
      clearInterval(pollTimer);
      pollTimer = null;
    }
    if (typeof document !== 'undefined') {
      document.removeEventListener('visibilitychange', onVisibility);
    }
  };
}

export interface CleanupResult {
  ok: boolean;
  removed: number;
  reclaimedBytes: number;
  error?: string;
}

// runCleanup reclaims the reclaimable disk. The daemon re-inspects and applies
// its own fresh plan, so the client never sends a target list. A successful run
// refreshes the snapshot so the widget reflects the freed space at once.
export async function runCleanup(): Promise<CleanupResult> {
  try {
    const res = await apiFetch('/api/disk', { method: 'POST' });
    const data = (await res.json()) as {
      ok?: boolean;
      removed?: number;
      reclaimed_bytes?: number;
      error?: string;
    };
    if (data.ok) await loadDisk();
    return {
      ok: Boolean(data.ok),
      removed: data.removed ?? 0,
      reclaimedBytes: data.reclaimed_bytes ?? 0,
      error: data.error
    };
  } catch (e) {
    return {
      ok: false,
      removed: 0,
      reclaimedBytes: 0,
      error: e instanceof Error ? e.message : m.common_requestFailed()
    };
  }
}
