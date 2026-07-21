import type { DumpEvent, QueryData } from '$lib/dumpsStream';

// Search haystacks, computed once per event and cached by event identity.
// Events are immutable once received, so the string can never go stale, and a
// WeakMap lets an evicted event's haystack be collected with it. Without this
// every lens re-lowercased (and for kind lenses re-JSON.stringify'd) the whole
// buffer on every rebuild.

const dumpCache = new WeakMap<DumpEvent, string>();
const kindCache = new WeakMap<DumpEvent, string>();
const queryCache = new WeakMap<DumpEvent, string>();

function memo(cache: WeakMap<DumpEvent, string>, ev: DumpEvent, build: () => string): string {
  const hit = cache.get(ev);
  if (hit !== undefined) return hit;
  const hay = build().toLowerCase();
  cache.set(ev, hay);
  return hay;
}

// dumpHaystack covers what the Dumps lens searches: label, text, request,
// source file and branch.
export function dumpHaystack(ev: DumpEvent): string {
  return memo(dumpCache, ev, () =>
    [ev.label ?? '', ev.text ?? '', ev.ctx.request ?? '', ev.src.file ?? '', ev.ctx.branch ?? ''].join(' ')
  );
}

// kindHaystack covers the generic lenses (jobs, views, mail, cache, events),
// whose payload shape varies per kind so the whole `data` object is searched.
export function kindHaystack(ev: DumpEvent): string {
  return memo(kindCache, ev, () =>
    [JSON.stringify(ev.data ?? {}), ev.ctx.request ?? '', ev.ctx.worker ?? '', ev.ctx.branch ?? ''].join(' ')
  );
}

// queryHaystack covers the Queries lens: the SQL plus the request context.
export function queryHaystack(ev: DumpEvent, data: QueryData): string {
  return memo(queryCache, ev, () =>
    [data.sql, ev.ctx.request ?? '', ev.src.file ?? '', ev.ctx.worker ?? '', ev.ctx.branch ?? ''].join(' ')
  );
}
