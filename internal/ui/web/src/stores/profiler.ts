import { writable } from 'svelte/store';
import { apiFetch, apiJson } from '$lib/api';
import { wsMessage } from '$lib/ws';

// profilerEnabled mirrors the global SPX profiler toggle.
export const profilerEnabled = writable<boolean>(false);

export async function loadProfilerStatus(): Promise<void> {
  try {
    const s = await apiJson<{ enabled: boolean }>('/api/profiler/status');
    profilerEnabled.set(Boolean(s.enabled));
  } catch {
    /* keep previous value */
  }
}

// setProfiler turns the global SPX profiler on or off. On means every
// PHP-FPM site's requests are profiled.
export async function setProfiler(enable: boolean): Promise<void> {
  const res = await apiFetch('/api/profiler/toggle', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ enable })
  });
  if (!res.ok) {
    throw new Error((await res.text()) || `profiler toggle failed (${res.status})`);
  }
  const data = (await res.json()) as { enabled: boolean };
  profilerEnabled.set(Boolean(data.enabled));
}

// clearProfilerData deletes every captured SPX report and returns how many
// were removed.
export async function clearProfilerData(): Promise<number> {
  const res = await apiFetch('/api/profiler/clear', { method: 'POST' });
  if (!res.ok) {
    throw new Error((await res.text()) || `profiler clear failed (${res.status})`);
  }
  const data = (await res.json()) as { removed: number };
  return data.removed ?? 0;
}

// Live-update from WS so a toggle from the CLI, MCP, or another browser tab
// is reflected without a manual refresh.
wsMessage.subscribe((msg) => {
  const fresh = msg?.profiler_status as { enabled: boolean } | undefined;
  if (fresh) profilerEnabled.set(Boolean(fresh.enabled));
});
