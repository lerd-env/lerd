import { writable } from 'svelte/store';
import { apiJson } from '$lib/api';

// The editors a file can be opened in, and the one chosen for every site that
// has not picked its own ("custom" when config.yaml holds a command template).
export interface EditorChoice {
  id: string;
  label: string;
  installed: boolean;
}
export const editors = writable<{ editors: EditorChoice[]; global: string; template?: string }>({ editors: [], global: '' });

export async function loadEditors(): Promise<void> {
  try {
    editors.set(await apiJson('/api/editors'));
  } catch {
    /* keep what we had */
  }
}

export async function setGlobalEditor(id: string): Promise<void> {
  await apiJson('/api/editors', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ id }) });
  await loadEditors();
}
