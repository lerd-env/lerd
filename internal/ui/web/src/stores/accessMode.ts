import { writable } from 'svelte/store';
import { apiJson } from '$lib/api';

// localControl means this request has full dashboard-control authority.
// Authenticated remote sessions and direct local sessions both set it to true.
// local is narrower: the dashboard is open on the lerd host itself, which is what
// controls acting on its desktop, such as the editor, need.
export interface AccessMode {
  localControl: boolean;
  local: boolean;
  lanExposed: boolean;
  checked: boolean;
}

export const accessMode = writable<AccessMode>({
  localControl: false,
  local: false,
  lanExposed: false,
  checked: false
});
interface AccessModeResponse {
  local_control?: boolean;
  local?: boolean;
  lan_exposed?: boolean;
}

export async function loadAccessMode() {
  try {
    const res = await apiJson<AccessModeResponse>('/api/access-mode');
    accessMode.set({
      localControl: Boolean(res.local_control),
      local: Boolean(res.local),
      lanExposed: Boolean(res.lan_exposed),
      checked: true
    });
  } catch {
    accessMode.update((a) => ({ ...a, checked: true }));
  }
}
