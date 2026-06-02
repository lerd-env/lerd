import { writable } from 'svelte/store';

// Remembers which Debug sub-lens (Dumps vs Queries) the user last viewed, so a
// refresh keeps them where they were. Shared between the System Debug panel
// and the per-site Debug tab so the choice is consistent across both.
export type DebugLens =
  | 'dumps'
  | 'queries'
  | 'jobs'
  | 'views'
  | 'mail'
  | 'cache'
  | 'events'
  | 'http';

const KEY = 'lerd:debugLens';

const VALID: DebugLens[] = ['dumps', 'queries', 'jobs', 'views', 'mail', 'cache', 'events', 'http'];

function initial(): DebugLens {
  if (typeof localStorage === 'undefined') return 'dumps';
  const v = localStorage.getItem(KEY) as DebugLens | null;
  return v && VALID.includes(v) ? v : 'dumps';
}

export const debugLens = writable<DebugLens>(initial());

debugLens.subscribe((v) => {
  try {
    if (typeof localStorage !== 'undefined') localStorage.setItem(KEY, v);
  } catch {
    // private mode / storage disabled — fall back to in-memory only.
  }
});
