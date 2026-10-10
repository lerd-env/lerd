import { readable, type Readable } from 'svelte/store';
import { getContext, setContext } from 'svelte';
import type { Counts } from '$lib/lens';

// What a Debug tab hands the lenses below it. The lenses read their rows from
// lerd-ui (see $lib/lens), so all that travels down is how they are narrowed
// and the badge counts the tab keeps.

// LensContext narrows every lens to the request rid names, as an inspector or
// a picked request id does, or to the route a search like "GET /path" names.
export interface LensContext {
  rid: string;
  route: string;
}

const SCOPE = Symbol('lensScope');

export function provideLensScope(scope: Readable<LensContext>): void {
  setContext(SCOPE, scope);
}

export function lensScope(): Readable<LensContext> {
  return getContext<Readable<LensContext> | undefined>(SCOPE) ?? readable({ rid: '', route: '' });
}

const COUNTS = Symbol('lensCounts');

// provideLensCounts shares the tab's badge counts, which also carry how many
// test-run events the lenses are hiding.
export function provideLensCounts(counts: Counts): void {
  setContext(COUNTS, counts);
}

export function lensCounts(): Counts | undefined {
  return getContext<Counts | undefined>(COUNTS);
}

const PICK = Symbol('pickRequest');

// providePickRequest lets a request id shown in a lens below the caller narrow
// the view to that request; pickRequest is undefined where nothing can.
export function providePickRequest(pick: (rid: string) => void): void {
  setContext(PICK, pick);
}

export function pickRequest(): ((rid: string) => void) | undefined {
  return getContext<((rid: string) => void) | undefined>(PICK);
}
