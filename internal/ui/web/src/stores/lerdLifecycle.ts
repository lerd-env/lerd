import { get, writable } from 'svelte/store';
import { apiFetch } from '$lib/api';
import { readNDJSON } from '$lib/ndjson';
import { handOverToVhost } from '$lib/vhost';
import { loadStatus, status } from './status';

// StartEvent mirrors cli.StartEvent: a stage of the start sequence, or one unit
// that has finished starting (error set when it failed).
interface StartEvent {
  phase: 'step' | 'unit' | 'done' | 'failed';
  step?: string;
  unit?: string;
  total?: number;
  error?: string;
}

export const lerdStarting = writable<boolean>(false);
export const lerdStopping = writable<boolean>(false);

// Progress of a running start. step is the stage id the UI translates; unit is
// the last unit that came up, shown in its place while units are starting.
export const lerdStartStep = writable<string>('');
export const lerdStartUnit = writable<string>('');
export const lerdStartDone = writable<number>(0);
export const lerdStartTotal = writable<number>(0);
// Why the last start stream failed, kept after it ends so the caller can say so.
export const lerdStartError = writable<string>('');

function resetStartProgress() {
  lerdStartStep.set('');
  lerdStartUnit.set('');
  lerdStartDone.set(0);
  lerdStartTotal.set(0);
}

// lerdStart runs the same start as `lerd start` and reads its NDJSON progress
// stream, so the button reports each stage and unit instead of hanging on a
// request that can take minutes on a cold Podman machine.
export function lerdStart(): Promise<boolean> {
  return streamStart('/api/lerd/start', { method: 'POST' });
}

// lerdResizeMachine restarts the Podman Machine with mib of memory; the start
// that follows streams the same progress as lerdStart.
export function lerdResizeMachine(mib: number): Promise<boolean> {
  return streamStart('/api/machine/memory', { method: 'POST', body: JSON.stringify({ mib }) });
}

async function streamStart(url: string, init: RequestInit): Promise<boolean> {
  lerdStarting.set(true);
  resetStartProgress();
  lerdStartError.set('');
  let done = 0;
  let ok = true;
  try {
    const res = await apiFetch(url, init);
    if (!res.ok || !res.body) {
      lerdStartError.set((await res.text()).trim() || res.statusText);
      return false;
    }
    await readNDJSON<StartEvent>(res.body, (evt) => {
      switch (evt.phase) {
        case 'step':
          lerdStartStep.set(evt.step ?? '');
          lerdStartUnit.set('');
          if (evt.total) lerdStartTotal.set(evt.total);
          break;
        case 'unit':
          lerdStartDone.set(++done);
          lerdStartUnit.set(evt.unit ?? '');
          break;
        case 'failed':
          ok = false;
          lerdStartError.set(evt.error ?? '');
          break;
      }
    });
    if (ok) await handOverToVhost();
    return ok;
  } catch (e) {
    lerdStartError.set(e instanceof Error ? e.message : String(e));
    return false;
  } finally {
    lerdStarting.set(false);
    resetStartProgress();
    await loadStatus();
  }
}

export async function lerdStop(): Promise<boolean> {
  lerdStopping.set(true);
  try {
    const res = await apiFetch('/api/lerd/stop', { method: 'POST' });
    await loadStatus();
    return res.ok;
  } catch {
    return false;
  } finally {
    lerdStopping.set(false);
  }
}

// Module state, not a store: the auto-start fires once per page load, and
// re-running it on every status refresh would fight a `lerd stop` issued from
// the CLI or the tray.
let autoStartTried = false;

// autoStartOnce brings the stack up when the dashboard opens on a stopped lerd
// and the setting is on, but never one the user stopped: that waits for Start.
export async function autoStartOnce(): Promise<void> {
  if (autoStartTried || get(status).stopped) return;
  autoStartTried = true;
  await lerdStart();
}
