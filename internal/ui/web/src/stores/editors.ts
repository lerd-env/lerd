import { derived, writable } from 'svelte/store';
import { apiJson } from '$lib/api';
import { sites } from './sites';
import { m } from '../paraglide/messages.js';

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
    const v = await apiJson<{ editors: EditorChoice[]; global: string; template?: string }>('/api/editors');
    if (Array.isArray(v?.editors)) editors.set(v);
  } catch {
    /* keep what we had */
  }
}

export async function setGlobalEditor(id: string): Promise<void> {
  await apiJson('/api/editors', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ id }) });
  await loadEditors();
}

// editorTitle reads, for a file, "Open in" and the editor it opens in: its
// site's own choice, else the global one, else just "editor".
export const editorTitle = derived([sites, editors], ([$sites, $editors]) => {
  if (!requested) {
    requested = true;
    void loadEditors();
  }
  return (file: string): string => {
    const site = $sites.filter((s) => s.path && file.startsWith(s.path + '/')).sort((a, b) => b.path!.length - a.path!.length)[0];
    const id = site?.editor || $editors.global;
    const label = $editors.editors.find((e) => e.id === id)?.label;
    return label ? m.editor_openIn({ editor: label }) : m.queries_openInEditor();
  };
});
let requested = false;
