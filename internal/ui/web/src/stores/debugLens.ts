import { writable } from 'svelte/store';
import type { TabItem } from '$components/DetailTabs.svelte';
import { m } from '../paraglide/messages.js';

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
  | 'messages'
  | 'browser';

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
  'messages',
  'browser'
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

// debugLensTabs is the lens bar shared by the System Debug panel and the site
// Debug tab, grouped so related lenses sit together. counts is keyed by event
// kind; showCache is false when no Laravel site can feed the cache lens.
export function debugLensTabs(counts: Record<string, number>, showCache: boolean): TabItem<DebugLens>[] {
  return [
    { id: 'dumps', label: m.debug_tab_dumps(), count: counts['dump'], group: 'dumps' },
    { id: 'exceptions', label: m.debug_tab_exceptions(), count: counts['exception'], group: 'errors' },
    { id: 'logs', label: m.debug_tab_logs(), count: counts['log'], group: 'errors' },
    { id: 'browser', label: m.debug_tab_browser(), count: counts['browser'], group: 'errors' },
    { id: 'queries', label: m.debug_tab_queries(), count: counts['query'], group: 'request' },
    { id: 'views', label: m.debug_tab_views(), count: counts['view'], group: 'request' },
    { id: 'cache', label: m.debug_tab_cache(), hidden: !showCache, count: counts['cache'], group: 'request' },
    { id: 'http', label: m.debug_tab_http(), count: counts['http'], group: 'request' },
    { id: 'jobs', label: m.debug_tab_jobs(), count: counts['job'], group: 'background' },
    { id: 'messages', label: m.debug_tab_messages(), count: counts['message'], group: 'background' },
    { id: 'events', label: m.debug_tab_events(), count: counts['event'], group: 'background' },
    { id: 'mail', label: m.debug_tab_mail(), count: counts['mail'], group: 'background' }
  ];
}
