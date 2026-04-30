import { writable } from 'svelte/store';
import { apiJson } from '$lib/api';

export interface ContainerStat {
  name: string;
  cpu_percent: number;
  mem_bytes: number;
  mem_limit_bytes: number;
  mem_percent: number;
}

export interface StatsResponse {
  containers: ContainerStat[];
  total_cpu_percent: number;
  total_mem_bytes: number;
  host_mem_bytes: number;
  updated_at: string;
  available: boolean;
}

const empty: StatsResponse = {
  containers: [],
  total_cpu_percent: 0,
  total_mem_bytes: 0,
  host_mem_bytes: 0,
  updated_at: '',
  available: false
};

export const stats = writable<StatsResponse>(empty);
export const statsLoaded = writable<boolean>(false);

const POLL_INTERVAL_MS = 5000;

let pollTimer: ReturnType<typeof setInterval> | null = null;
let inflight = false;

export async function loadStats(): Promise<void> {
  if (inflight) return;
  inflight = true;
  try {
    const res = await apiJson<StatsResponse>('/api/stats');
    stats.set({ ...empty, ...res });
    statsLoaded.set(true);
  } catch {
    /* keep previous */
  } finally {
    inflight = false;
  }
}

// startStatsPolling kicks off a 5-second poll loop scoped to dashboard
// visibility. The Page Visibility API gates polling so a backgrounded tab
// stops shelling out to podman; the WS pipeline already keeps the rest of
// the dashboard live, so stats can pause without losing critical signal.
export function startStatsPolling(): () => void {
  let stopped = false;
  function tick() {
    if (stopped) return;
    if (typeof document !== 'undefined' && document.hidden) return;
    void loadStats();
  }
  // Initial fetch right away so the widget hydrates without waiting 5s.
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

export function formatBytes(b: number): string {
  if (b < 1024) return b + ' B';
  const kb = b / 1024;
  if (kb < 1024) return kb.toFixed(1) + ' KB';
  const mb = kb / 1024;
  if (mb < 1024) return mb.toFixed(0) + ' MB';
  const gb = mb / 1024;
  return gb.toFixed(2) + ' GB';
}
