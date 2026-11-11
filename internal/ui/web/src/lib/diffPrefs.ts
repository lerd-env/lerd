// Full screen, the view and hidden whitespace are remembered across every diff
// the dashboard draws, so the next one opens the way the last was left.
const PREFS_KEY = 'lerd:gitDiff';

export type DiffPrefs = { fullscreen: boolean; unified: boolean; ignoreSpace: boolean };

export function readDiffPrefs(): DiffPrefs {
  try {
    const v = JSON.parse(localStorage.getItem(PREFS_KEY) ?? '{}');
    return { fullscreen: v.fullscreen === true, unified: v.unified === true, ignoreSpace: v.ignoreSpace === true };
  } catch {
    return { fullscreen: false, unified: false, ignoreSpace: false };
  }
}

export function writeDiffPrefs(prefs: DiffPrefs): void {
  try {
    localStorage.setItem(PREFS_KEY, JSON.stringify(prefs));
  } catch {
    /* localStorage may be unavailable (private mode, quota), non-fatal */
  }
}
