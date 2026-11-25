import { get, writable } from 'svelte/store';
import { apiJson } from '$lib/api';
import { idleEnabled, idleTimeoutMinutes, loadIdle, saveIdle } from '$stores/idle';

// IdleSuggestion mirrors ui.IdleServicesSuggestion: the running services that
// sleeping services would stop, empty when there is nothing worth asking.
export interface IdleSuggestion {
  services: string[];
  mem_bytes: number;
}

const none: IdleSuggestion = { services: [], mem_bytes: 0 };

export const idleSuggestion = writable<IdleSuggestion>(none);

export async function loadIdleSuggestion() {
  try {
    idleSuggestion.set(await apiJson<IdleSuggestion>('/api/idle/services-suggestion'));
  } catch {
    /* keep previous */
  }
}

// Either answer is saved in the global config, so no other window asks again.
// The policy is read first: the save carries all of it, and this window may
// have been open since before it changed.
export async function answerIdleSuggestion(sleep: boolean) {
  await loadIdle();
  if (await saveIdle(get(idleEnabled), get(idleTimeoutMinutes), sleep)) idleSuggestion.set(none);
}
