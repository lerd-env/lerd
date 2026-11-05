import { writable } from 'svelte/store';

export const FONT_MIN = 10;
export const FONT_MAX = 20;
const FONT_DEFAULT = 12;
const KEY = 'lerd-editor-font-size';
// Where builds before the shared setting kept tinker's size.
const LEGACY_KEY = 'tinker:fontSize';

// fontSize turns the stored text-size preference into a usable size, so a
// missing or hand-edited value never reaches an editor out of range.
export function fontSize(stored: string | null): number {
  const n = Number.parseInt(stored ?? '', 10);
  if (Number.isNaN(n)) return FONT_DEFAULT;
  return Math.min(FONT_MAX, Math.max(FONT_MIN, n));
}

// One size for every code surface: tinker, the env and config editors, and diffs.
export const editorFontSize = writable(fontSize(localStorage.getItem(KEY) ?? localStorage.getItem(LEGACY_KEY)));

editorFontSize.subscribe((size) => {
  try {
    localStorage.setItem(KEY, String(size));
  } catch {
    // Storage full or blocked: the size still applies until the page reloads.
  }
});
