import { writable } from 'svelte/store';
import { apiJson } from '$lib/api';

// localControl means this request has full dashboard-control authority.
// Authenticated remote sessions and direct local sessions both set it to true.
export interface AccessMode {
  localControl: boolean;
  lanExposed: boolean;
  checked: boolean;
}

export const accessMode = writable<AccessMode>({
  localControl: false,
  lanExposed: false,
  checked: false
});
interface AccessModeResponse {
  local_control?: boolean;
  lan_exposed?: boolean;
}

export async function loadAccessMode() {
  try {
    const res = await apiJson<AccessModeResponse>('/api/access-mode');
    accessMode.set({
      localControl: Boolean(res.local_control),
      lanExposed: Boolean(res.lan_exposed),
      checked: true
    });
  } catch {
    accessMode.update((a) => ({ ...a, checked: true }));
  }
}
