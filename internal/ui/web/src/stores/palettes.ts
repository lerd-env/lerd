import { apiFetch, apiJson } from '$lib/api';
import {
  BUILTIN_PALETTES,
  DEFAULT_PALETTE_ID,
  asDesktopStandIn,
  resolvePalette,
  type PaletteError,
  type PaletteFile
} from '$lib/palettes';
import { derived, get, writable } from 'svelte/store';
import { adoptTheme, palette, palettes, saveTheme } from '$stores/theme';
import { wsMessage } from '$lib/ws';
import { m } from '../paraglide/messages.js';

// Theme files the daemon could not parse. They stay visible so the author of a
// hand-written theme is told what is wrong with it rather than left wondering
// why it never showed up.
export const paletteErrors = writable<PaletteError[]>([]);

// The theme the config holds: empty while nobody has ever picked one, null until
// the daemon has said.
export const configTheme = writable<string | null>(null);

// desktopSuggestion is the desktop's own theme, offered once to an install that
// never chose a theme and still wears the default. A choice already made, even
// the default one, is never second guessed.
export const desktopSuggestion = derived([palettes, palette, configTheme], ([$palettes, $palette, $config]) => {
  // An empty id is what a cleared config broadcasts, and it paints the default.
  if ($config !== '' || ($palette || DEFAULT_PALETTE_ID) !== DEFAULT_PALETTE_ID) return null;
  return $palettes.find((p) => p.source === 'desktop' && p.id !== DEFAULT_PALETTE_ID) ?? null;
});

export function useSuggestedTheme() {
  const suggested = get(desktopSuggestion);
  if (!suggested) return;
  configTheme.set(suggested.id);
  palette.set(suggested.id);
}

// Declining is recorded as a choice of the current theme, so the question is
// answered for every device that opens the dashboard, not once per browser.
export function keepCurrentTheme() {
  const current = get(palette);
  configTheme.set(current);
  void saveTheme(current);
}

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
    configTheme.set(chosen.theme ?? '');
  } catch {
    /* keep whatever the browser remembered */
  }
  try {
    const res = await apiJson<ThemesResponse>('/api/themes');
    const user = (res.themes || []).map(asDesktopStandIn).map(resolvePalette).filter((p) => p !== null);
    // A file named after a built-in replaces it rather than sitting beside it as
    // a second entry with the same name. The file is the more specific answer,
    // and the picker has to stay unambiguous. A desktop entry claims a built-in's
    // id the same way, and the daemon lists it last, so it wins over a file that
    // claimed the same one.
    const offered = [...new Map(user.map((p) => [p.id, p])).values()];
    const shadowed = new Set(offered.map((p) => p.id));
    palettes.set([...BUILTIN_PALETTES.filter((p) => !shadowed.has(p.id)), ...offered]);
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

// A switch on one device reaches the others over the socket they already hold
// open, so two dashboards side by side never disagree about what lerd looks
// like. adoptTheme rather than palette.set, so the value that arrived is not
// written straight back to the config it came from.
export function watchThemeChanges() {
  return wsMessage.subscribe((msg) => {
    if (msg?.theme !== undefined) {
      adoptTheme(msg.theme);
      configTheme.set(msg.theme);
    }
    // The desktop theme keeps its id when its colours change, so the list has to
    // be refetched rather than reapplied from what is already in hand.
    if (msg?.type === 'theme_list') void loadPalettes();
  });
}
