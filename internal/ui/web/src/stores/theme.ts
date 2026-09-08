import { get, writable } from 'svelte/store';
import {
  BUILTIN_PALETTES,
  DEFAULT_PALETTE_ID,
  paletteById,
  paletteVars,
  type Palette
} from '$lib/palettes';

export type Theme = 'light' | 'dark' | 'auto';

const KEY = 'lerd-theme';
const PALETTE_KEY = 'lerd-palette';

function read(): Theme {
  const v = localStorage.getItem(KEY);
  return v === 'light' || v === 'dark' || v === 'auto' ? v : 'auto';
}

// The theme's tones are written as inline custom properties on the root
// element, which every lerd-* utility resolves through (see the @theme block in
// app.css). Inline wins over the stylesheet unconditionally, and app.css names
// the default theme as the fallback, so the page is correct before this runs and
// nothing but these two files has to know a theme exists.
function apply(theme: Theme) {
  const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
  const dark = theme === 'dark' || (theme === 'auto' && prefersDark);
  document.documentElement.classList.toggle('dark', dark);

  const vars = paletteVars(paletteById(get(palettes), get(palette)), dark);
  for (const [name, value] of Object.entries(vars)) {
    document.documentElement.style.setProperty(name, value);
  }
}

export const theme = writable<Theme>('auto');

// The chosen theme is a per-browser preference like the light/dark mode; the
// definitions it names come from the daemon.
export const palette = writable<string>(DEFAULT_PALETTE_ID);
export const palettes = writable<Palette[]>(BUILTIN_PALETTES);

export function initTheme() {
  const initial = read();
  theme.set(initial);
  palette.set(localStorage.getItem(PALETTE_KEY) || DEFAULT_PALETTE_ID);
  apply(initial);
  theme.subscribe((t) => {
    localStorage.setItem(KEY, t);
    apply(t);
  });
  palette.subscribe((p) => {
    localStorage.setItem(PALETTE_KEY, p);
    apply(get(theme));
  });
  // Themes arrive after the first paint, and one of them may be the chosen
  // one, so a new list has to repaint rather than wait for the next mode flip.
  palettes.subscribe(() => apply(get(theme)));
  // On a system light/dark change, re-apply the current theme so 'auto' follows
  // live. Re-applying directly (not theme.update) because setting the store to
  // its current value is a no-op that never notifies subscribers.
  window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => {
    apply(get(theme));
  });
}
