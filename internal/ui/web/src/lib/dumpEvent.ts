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
