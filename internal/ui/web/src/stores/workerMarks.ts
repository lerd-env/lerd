import { writable } from 'svelte/store';
import { sharedJson } from '$lib/api';

export interface WorkerMark {
  icon?: string;
  color?: string;
}

export interface WorkerMarkSet {
  // How one worker of one framework asks to be drawn, keyed "<framework>/<worker>".
  workers: Record<string, WorkerMark>;
  // The drawings, keyed by icon name, so every framework running Vite shares one.
  marks: Record<string, string>;
}

export const workerMarks = writable<WorkerMarkSet>({ workers: {}, marks: {} });

export async function loadWorkerMarks() {
  try {
    const set = await sharedJson<WorkerMarkSet>('/api/workers/marks');
    workerMarks.set({ workers: set?.workers ?? {}, marks: set?.marks ?? {} });
  } catch {
    /* keep whatever we already drew */
  }
}

// A worker unit knows its site, and the site knows its framework, but a
// suspended worker reaches the widget as a bare name. Falling back to any
// framework that declares the same worker keeps that one drawn like its
// siblings instead of dropping to the neutral glyph.
export function workerMarkFor(
  set: WorkerMarkSet,
  worker: string,
  framework?: string
): WorkerMark | undefined {
  if (framework) {
    const exact = set.workers[framework + '/' + worker];
    if (exact) return exact;
  }
  const suffix = '/' + worker;
  for (const [key, mark] of Object.entries(set.workers)) {
    if (key.endsWith(suffix)) return mark;
  }
  return undefined;
}
