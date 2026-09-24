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
  | 'http'
  | 'logs'
  | 'exceptions'
  | 'messages';

const KEY = 'lerd:debugLens';

const VALID: DebugLens[] = [
  'dumps',
  'queries',
  'jobs',
  'views',
  'mail',
  'cache',
  'events',
  'http',
  'logs',
  'exceptions',
  'messages'
];

// isDebugLens reports whether a route segment names a lens, so a deep link can
// open the one the notification was about.
export function isDebugLens(v: string | undefined): v is DebugLens {
  return !!v && (VALID as string[]).includes(v);
}

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
// recorded and rendered. It mirrors the server's devtools.tests setting, which
// decides whether the receiver keeps them at all: a suite fills the buffer.
export const showTests = writable<boolean>(false);

debugLens.subscribe((v) => {
  try {
    if (typeof localStorage !== 'undefined') localStorage.setItem(KEY, v);
  } catch {
    // private mode / storage disabled — fall back to in-memory only.
  }
});
