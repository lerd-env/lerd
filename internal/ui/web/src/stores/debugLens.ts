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

// debugSearch is the one text filter shared across a site's Debug lenses (Queries,
// Dumps, Kinds), so a search set in one lens (or seeded by a deep link like the
// timing view's Inspect queries) carries over when you switch lenses.
export const debugSearch = writable<string>('');

// showTests governs whether events captured inside a PHPUnit/Pest run are
// rendered. Off by default (a suite buries everything else), shared across
// every lens, and remembered so it isn't re-ticked on each visit.
const TESTS_KEY = 'lerd:debugShowTests';

function initialShowTests(): boolean {
  if (typeof localStorage === 'undefined') return false;
  return localStorage.getItem(TESTS_KEY) === '1';
}

export const showTests = writable<boolean>(initialShowTests());

showTests.subscribe((v) => {
  try {
    if (typeof localStorage !== 'undefined') localStorage.setItem(TESTS_KEY, v ? '1' : '0');
  } catch {
    // private mode / storage disabled — fall back to in-memory only.
  }
});

debugLens.subscribe((v) => {
  try {
    if (typeof localStorage !== 'undefined') localStorage.setItem(KEY, v);
  } catch {
    // private mode / storage disabled — fall back to in-memory only.
  }
});
