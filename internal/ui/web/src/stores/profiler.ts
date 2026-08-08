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

// captureCount reports how many SPX profiles a host has for one route. Callers
// take it before firing a request and watch it rise: that is the moment the
// report exists, which is not the moment the request was sent.
export async function captureCount(host: string, route: string): Promise<number> {
  try {
    const q = new URLSearchParams({ host, route });
    const d = await apiJson<{ count: number }>(`/api/profiler/captures?${q}`);
    return d.count ?? 0;
  } catch {
    return 0;
  }
}

// waitForCapture resolves true once the route has one more profile than it had
// at baseline, false if nothing lands in time. A request that misses the
// profiler leaves the count where it was, which is the case worth reporting
// rather than opening an SPX report list that cannot contain it.
export async function waitForCapture(
  host: string,
  route: string,
  baseline: number,
  timeoutMillis = 20000,
  pollMillis = 500
): Promise<boolean> {
  const deadline = Date.now() + timeoutMillis;
  while (Date.now() < deadline) {
    await new Promise((r) => setTimeout(r, pollMillis));
    if ((await captureCount(host, route)) > baseline) return true;
  }
  return false;
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
