import { writable, derived, get } from 'svelte/store';
import { presets, type Preset } from './presets';
import type { Service } from './services';

const STORAGE_KEY = 'lerd-dismissed-preset-suggestions';

function readDismissed(): string[] {
  try {
    const v = JSON.parse(localStorage.getItem(STORAGE_KEY) || '[]');
    return Array.isArray(v) ? v : [];
  } catch {
    return [];
  }
}

export const dismissedSuggestions = writable<string[]>(readDismissed());

dismissedSuggestions.subscribe((v) => {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(v));
  } catch {
    /* no-op */
  }
});

export function dismissSuggestion(name: string) {
  dismissedSuggestions.update((list) => (list.includes(name) ? list : [...list, name]));
}

// The key a preset's admin_for is matched against: the preset a service was
// installed from, so a versioned member like "mariadb-11-8" matches "mariadb".
export function adminKeyFor(svc: Service | null | undefined): string | null {
  if (!svc || !svc.name) return null;
  return svc.preset || svc.name;
}

function administers(candidate: { admin_for?: string[] }, key: string): boolean {
  return (candidate.admin_for || []).includes(key);
}

export function adminServiceFor(svc: Service, services: Service[]): Service | null {
  const key = adminKeyFor(svc);
  if (!key) return null;
  return services.find((s) => administers(s, key)) || null;
}

function pickSuggestion(presetList: Preset[], dismissed: string[], key: string | null): Preset | null {
  if (!key) return null;
  const p = presetList.find((x) => administers(x, key));
  if (!p || dismissed.includes(p.name)) return null;
  // missing_deps: install preflight must already be clear (family / env_role
  // drop-ins count), otherwise suggesting the preset would only fail on Add.
  if (p.installed || (p.missing_deps || []).length > 0) return null;
  return p;
}

export function suggestedPresetFor(svc: Service): Preset | null {
  return pickSuggestion(get(presets), get(dismissedSuggestions), adminKeyFor(svc));
}

// Reactive helper so UIs can bind to it
export const suggestionFor = (svc: Service | null | undefined) =>
  derived([presets, dismissedSuggestions], ([$presets, $dismissed]): Preset | null =>
    pickSuggestion($presets, $dismissed, adminKeyFor(svc))
  );
