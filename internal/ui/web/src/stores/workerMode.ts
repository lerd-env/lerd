import { writable } from 'svelte/store';
import { apiJson, apiFetch } from '$lib/api';

export type WorkerExecMode = 'exec' | 'container';

export const workerExecMode = writable<WorkerExecMode>('exec');
export const workerModeApplies = writable<boolean>(false);
export const workerModeLoading = writable<boolean>(false);

// workerModeProgress holds the last phase event the server emitted during
// a streaming setWorkerMode call. The modal renders this to show
// "Stopping lerd-horizon-parkapp", "Starting …" etc. instead of a blank
// spinner during the 30-60s migration.
export interface WorkerModeProgress {
  phase: string;
  unit?: string;
  step?: string;
  message?: string;
}
export const workerModeProgress = writable<WorkerModeProgress | null>(null);

interface SettingsResponse {
  worker_exec_mode?: string;
  worker_mode_applies?: boolean;
}

interface WorkerModePhaseEvent {
  phase: string;
  unit?: string;
  step?: string;
  message?: string;
  error?: string;
}

function normalizeMode(v: unknown): WorkerExecMode {
  return v === 'container' ? 'container' : 'exec';
}

export async function loadWorkerMode() {
  try {
    const res = await apiJson<SettingsResponse>('/api/settings');
    workerExecMode.set(normalizeMode(res.worker_exec_mode));
    workerModeApplies.set(Boolean(res.worker_mode_applies));
  } catch {
    /* keep previous */
  }
}

// setWorkerMode POSTs the new mode and reads the NDJSON progress stream.
// Returns { ok, error } once the stream completes (or fails). The
// workerModeProgress store updates on every phase event so the modal
// can show live progress.
export async function setWorkerMode(
  mode: WorkerExecMode
): Promise<{ ok: boolean; error?: string }> {
  workerModeLoading.set(true);
  workerModeProgress.set({ phase: 'starting' });
  try {
    const res = await apiFetch('/api/settings/worker-mode', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ mode })
    });
    if (!res.ok || !res.body) {
      return { ok: false, error: `${res.status} ${res.statusText}` };
    }
    const reader = res.body.getReader();
    const decoder = new TextDecoder();
    let buf = '';
    let finalError: string | undefined;
    let sawDone = false;
    for (;;) {
      const { done, value } = await reader.read();
      if (done) break;
      buf += decoder.decode(value, { stream: true });
      let nl: number;
      while ((nl = buf.indexOf('\n')) >= 0) {
        const line = buf.slice(0, nl).trim();
        buf = buf.slice(nl + 1);
        if (!line) continue;
        let evt: WorkerModePhaseEvent;
        try {
          evt = JSON.parse(line) as WorkerModePhaseEvent;
        } catch {
          continue;
        }
        if (evt.phase === 'error') {
          finalError = evt.error || 'failed';
          workerModeProgress.set({ phase: 'error', message: finalError });
          continue;
        }
        if (evt.phase === 'done') {
          sawDone = true;
          continue;
        }
        workerModeProgress.set({
          phase: evt.phase,
          unit: evt.unit,
          step: evt.step,
          message: evt.message
        });
      }
    }
    if (!finalError && sawDone) {
      workerExecMode.set(mode);
      return { ok: true };
    }
    return { ok: false, error: finalError || 'stream ended without done' };
  } catch (e) {
    return { ok: false, error: e instanceof Error ? e.message : 'request failed' };
  } finally {
    workerModeLoading.set(false);
    // Leave workerModeProgress visible briefly so the final phase shows;
    // the caller (modal) clears it on close.
  }
}
