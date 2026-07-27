import { writable } from 'svelte/store';

// Where a keyed fetch stands: whether one is in flight, and whether the last
// one landed. A view needs both to tell a request still on its way apart from
// one that never arrived, rather than reading missing data as an empty answer.
export interface LoadStatus {
  pending: boolean;
  failed: boolean;
}

export function createLoadStates() {
  const { subscribe, update, set } = writable<Record<string, LoadStatus>>({});
  return {
    subscribe,
    // A retry keeps the failed mark until it lands, so a view stays on the error
    // it is retrying instead of flipping back to a loading line every poll.
    start(key: string): void {
      update((all) => ({ ...all, [key]: { pending: true, failed: all[key]?.failed ?? false } }));
    },
    settle(key: string, failed: boolean): void {
      update((all) => ({ ...all, [key]: { pending: false, failed } }));
    },
    reset(): void {
      set({});
    }
  };
}
