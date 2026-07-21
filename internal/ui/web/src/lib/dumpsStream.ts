import { writable, type Writable } from 'svelte/store';
import { apiUrl } from './api';

// DumpEvent mirrors internal/dumps.Event verbatim. Keep field names in sync
// with the Go struct's json tags — TypeScript validates wire shape, Go owns
// the source of truth.
export interface DumpSource {
  file: string;
  line: number;
}

export interface DumpContext {
  type: 'fpm' | 'cli' | string;
  site?: string;
  branch?: string;
  domain?: string;
  request?: string;
  pid?: number;
  // rid is a unique per-request id from the lerd_devtools extension. When
  // present it's the precise grouping boundary; dump-bridge events lack it.
  rid?: string;
  // worker names the queue/scheduler command an event came from (e.g.
  // "queue:work", "scrape:rtb-data"). Set only for worker-process events.
  worker?: string;
  // test marks an event captured inside a PHPUnit/Pest run. The Debug lenses
  // hide these by default so a suite can't bury genuine dumps.
  test?: boolean;
}

export interface DumpEvent {
  v: number;
  id: string;
  ts: string;
  kind: string;
  ctx: DumpContext;
  src: DumpSource;
  label?: string;
  text?: string;
  // tree is opaque JSON the receiver passes through unchanged. Deferred to
  // a later PR; the current bridge only ships `text`.
  tree?: unknown;
  // data carries kind-specific structured fields (e.g. QueryData for
  // kind === 'query'). Dumps leave it unset and use text/tree.
  data?: unknown;
  trunc?: boolean;
}

// QueryData is the `data` payload on kind === 'query' events. Mirrors
// internal/dumps.QueryData; the lerd_devtools extension fills sql/bindings/
// time_ms, the Laravel adapter additionally sets connection/rw_type.
export interface QueryFrame {
  file: string;
  line: number;
  func: string;
}

export interface QueryData {
  sql: string;
  bindings?: unknown[];
  time_ms: number;
  connection?: string;
  rw_type?: string;
  // trace is the full call stack (innermost first) so vendor-heavy apps where
  // the single src line is unhelpful can still be traced to the real origin.
  trace?: QueryFrame[];
}

export interface DumpsStream {
  events: Writable<DumpEvent[]>;
  connected: Writable<boolean>;
  connect: () => void;
  close: () => void;
  clear: () => void;
}

// The receiver ring is shared across every kind (dump, query, mail, view,
// event, job, http), so a single request now emits ~7+ events. The UI keeps a
// far larger window than the server's replay ring so the dashboard accumulates
// a full session's history — events stay until the page is refreshed rather
// than getting evicted the moment newer traffic of any kind flows in. The cap
// is a memory safety ceiling, not the working size; it sits well above the
// server ring so an evicted event can never still be in a replay (which is
// what made stale events reappear when the two limits were equal).
const DEFAULT_MAX = 10000;

export function createDumpsStream(query: Record<string, string> = {}, maxEvents = DEFAULT_MAX): DumpsStream {
  const events = writable<DumpEvent[]>([]);
  const connected = writable<boolean>(false);
  let source: EventSource | null = null;
  // Mirrors the ids currently in `events` for O(1) de-dup. Kept in sync on
  // every push/evict so reconnect replays never double-add an event we already
  // show, no matter how large the list grows.
  const seen = new Set<string>();

  function close() {
    if (source) {
      source.close();
      source = null;
    }
    connected.set(false);
  }

  function clear() {
    events.set([]);
    seen.clear();
  }

  function buildPath() {
    const params = new URLSearchParams();
    for (const [k, v] of Object.entries(query)) {
      if (v) params.set(k, v);
    }
    const qs = params.toString();
    return qs ? `/api/dumps/stream?${qs}` : '/api/dumps/stream';
  }

  function append(ev: DumpEvent) {
    // De-dupe by id against everything currently held. The replay-on-reconnect
    // path resends the server ring, so without this a reconnect would re-add
    // events the dashboard already shows.
    if (seen.has(ev.id)) return;
    events.update((list) => {
      let next: DumpEvent[];
      if (list.length >= maxEvents) {
        const drop = list.length - maxEvents + 1;
        for (let i = 0; i < drop; i++) seen.delete(list[i].id);
        next = list.slice(drop);
      } else {
        next = list.slice();
      }
      seen.add(ev.id);
      next.push(ev);
      return next;
    });
  }

  function connect() {
    close();
    try {
      const es = new EventSource(apiUrl(buildPath()));
      source = es;
      es.addEventListener('open', () => connected.set(true));
      es.addEventListener('error', () => connected.set(false));
      es.addEventListener('message', (e) => {
        try {
          const data = JSON.parse((e as MessageEvent).data) as DumpEvent;
          if (data && typeof data === 'object' && data.id) append(data);
        } catch {
          // Malformed payload — ignore. The Go server only emits valid JSON,
          // but proxies could rewrite the stream and we'd rather skip a line
          // than crash the tab.
        }
      });
    } catch {
      connected.set(false);
    }
  }

  return { events, connected, connect, close, clear };
}
