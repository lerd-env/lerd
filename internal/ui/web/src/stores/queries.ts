import { derived, writable, type Readable } from 'svelte/store';
import { apiFetch, apiJson } from '$lib/api';
import { toggleDumps, status as dumpsStatus, type DumpsStatus } from '$stores/dumps';
import { wsMessage } from '$lib/ws';
import { showTests } from '$stores/debugLens';

// The Queries lens's filters and the devtools capture switches. Grouping by
// request, duplicate and N+1 flags and the per-request rollups come from
// lerd-ui's /api/dumps/groups, computed where the events are stored.

// SLOW_MS tags any single query at or above this duration, matching
// Telescope's default slow-query threshold.
export const SLOW_MS = 100;

export interface DevtoolsStatus {
  enabled: boolean;
  workers: boolean;
  tests: boolean;
}

export const devtoolsStatus = writable<DevtoolsStatus | null>(null);

devtoolsStatus.subscribe((s) => {
  if (s) showTests.set(Boolean(s.tests));
});

export const queryFilterText = writable<string>('');
export const queryFilterSite = writable<string>('');
export const queryFilterWorker = writable<string>('');

export async function refreshDevtoolsStatus(): Promise<void> {
  try {
    const data = await apiJson<DevtoolsStatus>('/api/devtools/status');
    devtoolsStatus.set(data);
  } catch {
    devtoolsStatus.set(null);
  }
}

export async function toggleDevtoolsWorkers(enable: boolean): Promise<void> {
  await apiFetch('/api/devtools/workers', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ enable })
  });
  void refreshDevtoolsStatus();
}

// toggleDevtoolsTests switches whether the receiver records test-run events.
// The checkbox moves at once; the status refresh settles it on what was saved.
export async function toggleDevtoolsTests(enable: boolean): Promise<void> {
  showTests.set(enable);
  await apiFetch('/api/devtools/tests', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ enable })
  });
  await refreshDevtoolsStatus();
}

// debugCaptureEnabled is the whole Debug window's one switch. The debug bridge
// and the devtools collector share a single sentinel, so one flag (the dumps
// enable state) arms both; the sub-tabs gate on this.
export const debugCaptureEnabled: Readable<boolean> = derived(
  dumpsStatus,
  ($d) => Boolean(($d as DumpsStatus | null)?.enabled)
);

// setDebugCapture flips the shared capture flag, arming the entire Debug window
// (dumps + the devtools collector) in one call. refreshDevtoolsStatus keeps the
// worker-capture sub-toggle's state in sync.
export async function setDebugCapture(on: boolean): Promise<void> {
  await toggleDumps(on);
  await refreshDevtoolsStatus();
}

wsMessage.subscribe((msg) => {
  const fresh = msg?.devtools_status as DevtoolsStatus | undefined;
  if (fresh) devtoolsStatus.set(fresh);
});
