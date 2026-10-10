import { writable } from 'svelte/store';
import { sharedJson } from '$lib/api';

// The marks a preset ships with itself, keyed by preset name. lerd-ui serves
// them from its own cache of the store, already sanitized, so the browser never
// reaches the store origin and the icons keep drawing offline and over remote
// access. A preset that ships none is simply absent here and falls back to the
// built-in glyph its YAML names.
export const serviceIcons = writable<Record<string, string>>({});

export async function loadServiceIcons() {
  try {
    serviceIcons.set((await sharedJson<Record<string, string>>('/api/services/icons')) || {});
  } catch {
    /* keep whatever we already drew */
  }
}
