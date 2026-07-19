import { get, writable } from 'svelte/store';

export type Theme = 'light' | 'dark' | 'auto';

const KEY = 'lerd-theme';

function read(): Theme {
  const v = localStorage.getItem(KEY);
  return v === 'light' || v === 'dark' || v === 'auto' ? v : 'auto';
}

function apply(theme: Theme) {
  const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
  const dark = theme === 'dark' || (theme === 'auto' && prefersDark);
  document.documentElement.classList.toggle('dark', dark);
}

export const theme = writable<Theme>('auto');

export function initTheme() {
  const initial = read();
  theme.set(initial);
  apply(initial);
  theme.subscribe((t) => {
    localStorage.setItem(KEY, t);
    apply(t);
  });
  // On a system light/dark change, re-apply the current theme so 'auto' follows
  // live. Re-applying directly (not theme.update) because setting the store to
  // its current value is a no-op that never notifies subscribers.
  window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => {
    apply(get(theme));
  });
}
