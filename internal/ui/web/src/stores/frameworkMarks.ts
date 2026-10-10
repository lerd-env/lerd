import { writable } from 'svelte/store';
import { sharedJson } from '$lib/api';

export interface FrameworkMark {
  svg?: string;
  color?: string;
}

// The mark and brand colour a framework definition carries, keyed by framework
// name. lerd-ui serves them from its own cache of the store, already sanitized,
// so the browser never reaches the store origin and a site's framework keeps
// drawing offline and over remote access. A framework that declares neither is
// simply absent here and renders as its label alone.
export const frameworkMarks = writable<Record<string, FrameworkMark>>({});

export async function loadFrameworkMarks() {
  try {
    frameworkMarks.set((await sharedJson<Record<string, FrameworkMark>>('/api/frameworks/marks')) || {});
  } catch {
    /* keep whatever we already drew */
  }
}
