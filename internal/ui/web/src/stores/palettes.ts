import { apiFetch, apiJson } from '$lib/api';
import {
  BUILTIN_PALETTES,
  resolvePalette,
  type PaletteError,
  type PaletteFile
} from '$lib/palettes';
import { writable } from 'svelte/store';
import { adoptTheme, palettes } from '$stores/theme';
import { m } from '../paraglide/messages.js';

// Theme files the daemon could not parse. They stay visible so the author of a
// hand-written theme is told what is wrong with it rather than left wondering
// why it never showed up.
export const paletteErrors = writable<PaletteError[]>([]);

interface ThemesResponse {
  themes?: PaletteFile[];
  errors?: PaletteError[];
}

// The chosen theme lives in the global config, so a phone on the LAN and the
// desktop next to it show the same lerd. localStorage keeps a copy only so the
// first paint has something before this answers; the config is what decides.
export async function loadPalettes() {
  try {
    const chosen = await apiJson<{ theme?: string }>('/api/settings');
    if (chosen.theme) adoptTheme(chosen.theme);
  } catch {
    /* keep whatever the browser remembered */
  }
  try {
    const res = await apiJson<ThemesResponse>('/api/themes');
    const user = (res.themes || []).map(resolvePalette).filter((p) => p !== null);
    // A file named after a built-in replaces it rather than sitting beside it as
    // a second entry with the same name. The file is the more specific answer,
    // and the picker has to stay unambiguous.
    const shadowed = new Set(user.map((p) => p.id));
    palettes.set([...BUILTIN_PALETTES.filter((p) => !shadowed.has(p.id)), ...user]);
    paletteErrors.set(res.errors || []);
  } catch {
    /* keep previous */
  }
}

export async function importPalette(id: string, content: string): Promise<string> {
  try {
    const res = await apiFetch('/api/themes', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ id, content })
    });
    const body = (await res.json()) as { ok?: boolean; error?: string };
    if (!body.ok) return body.error || m.common_failed();
    await loadPalettes();
    return '';
  } catch (e) {
    return e instanceof Error ? e.message : m.common_failed();
  }
}

export async function removePalette(id: string): Promise<boolean> {
  try {
    const res = await apiFetch(`/api/themes/${encodeURIComponent(id)}`, { method: 'DELETE' });
    if (!res.ok) return false;
    // The theme store re-applies on a new list, so dropping the one in use
    // falls the dashboard back to the default on its own.
    await loadPalettes();
    return true;
  } catch {
    return false;
  }
}
