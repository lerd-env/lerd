import { writable, derived } from 'svelte/store';
import { apiJson, apiFetch, apiUrl } from '$lib/api';
import { loadServices } from './services';
import { goToTab } from './route';
import { m } from '../paraglide/messages.js';

export interface PresetVersion {
  tag: string;
  label?: string;
}

export interface Preset {
  name: string;
  description?: string;
  image?: string;
  dashboard?: string;
  depends_on?: string[];
  missing_deps?: string[];
  installed?: boolean;
  installed_tags?: string[];
  versions?: PresetVersion[];
  default_version?: string;
  selected_version?: string;
  installing?: boolean;
  installingPhase?: string;
  installingMessage?: string;
  installingDep?: string;
  error?: string;
}

export const presets = writable<Preset[]>([]);
export const presetsLoaded = writable<boolean>(false);

export async function loadPresets() {
  try {
    const data = await apiJson<Preset[]>('/api/services/presets');
    const prev = pget(presets);
    const next = (data || []).map((p) => {
      const old = prev.find((x) => x.name === p.name) || ({} as Preset);
      const enriched: Preset = { ...p, installing: old.installing || false, error: old.error || '' };
      if ((enriched.versions || []).length > 0) {
        const installed = new Set(enriched.installed_tags || []);
        const avail = (enriched.versions || []).filter((v) => !installed.has(v.tag));
        const stillValid = avail.find((v) => v.tag === old.selected_version);
        const fallback = avail.find((v) => v.tag === enriched.default_version) || avail[0];
        enriched.selected_version = (stillValid || fallback || { tag: '' }).tag;
      }
      return enriched;
    });
    presets.set(next);
    presetsLoaded.set(true);
  } catch {
    /* keep previous */
  }
}

function pget<T>(s: import('svelte/store').Readable<T>): T {
  let v!: T;
  const u = s.subscribe((x) => (v = x));
  u();
  return v;
}

export const installablePresets = derived(presets, ($p) =>
  $p.filter((p) => {
    if ((p.missing_deps || []).length > 0) return false;
    if ((p.versions || []).length > 0) {
      return (p.versions || []).length > (p.installed_tags || []).length;
    }
    return !p.installed;
  })
);

// Discovery promotes services you don't run at all yet. Unlike
// installablePresets it hides any preset with an installed instance, so an
// existing mysql / mariadb / postgres install never reappears here just to
// offer its other versions — adding an alternate version stays in the modal.
export const discoverablePresets = derived(presets, ($p) =>
  $p.filter((p) => (p.missing_deps || []).length === 0 && !p.installed)
);

export function availableVersions(p: Preset): PresetVersion[] {
  const installed = new Set(p.installed_tags || []);
  return (p.versions || []).filter((v) => !installed.has(v.tag));
}

// Text match for a preset against a free-text query (name, description, image).
// Shared by the preset modal search and the command palette so both filter the
// same way. Empty query matches everything; terms are ANDed.
export function matchesPresetQuery(p: Preset, query: string): boolean {
  const terms = query.trim().toLowerCase().split(/\s+/).filter(Boolean);
  if (terms.length === 0) return true;
  const hay = (p.name + ' ' + (p.description || '') + ' ' + (p.image || '')).toLowerCase();
  return terms.every((t) => hay.includes(t));
}

// Localized label for a preset's Add button, reflecting the live install
// phase. Shared by the modal, the discovery cards, and the suggestion banner.
export function presetAddLabel(p: Preset): string {
  if (!p.installing) return m.services_preset_phase_add();
  switch (p.installingPhase) {
    case 'installing_config':
      return m.services_preset_phase_installingConfig();
    case 'starting_deps':
      return p.installingDep
        ? m.services_preset_phase_startingDep({ dep: p.installingDep })
        : m.services_preset_phase_startingDeps();
    case 'pulling_image':
      return m.services_preset_phase_pullingImage();
    case 'starting_unit':
      return m.services_preset_phase_startingUnit();
    case 'waiting_ready':
      return m.services_preset_phase_waitingReady();
    default:
      return m.services_preset_phase_adding();
  }
}

function updatePreset(name: string, mut: (p: Preset) => Preset) {
  presets.update((list) => list.map((p) => (p.name === name ? mut(p) : p)));
}

export interface InstallResult {
  ok: boolean;
  name?: string;
  error?: string;
}

export async function installPreset(p: Preset): Promise<InstallResult> {
  if ((p.missing_deps || []).length > 0) {
    const err = 'Install ' + (p.missing_deps || []).join(', ') + ' first';
    updatePreset(p.name, (x) => ({ ...x, error: err }));
    return { ok: false, error: err };
  }

  updatePreset(p.name, (x) => ({
    ...x,
    installing: true,
    installingPhase: '',
    installingMessage: '',
    installingDep: '',
    error: ''
  }));

  let url = '/api/services/presets/' + encodeURIComponent(p.name);
  if ((p.versions || []).length > 0 && p.selected_version) {
    url += '?version=' + encodeURIComponent(p.selected_version);
  }

  let finalEvent: { phase?: string; name?: string; error?: string } | null = null;

  try {
    const res = await apiFetch(url, { method: 'POST' });
    if (!res.ok || !res.body) {
      const text = (await res.text()).trim() || 'install failed';
      updatePreset(p.name, (x) => ({ ...x, installing: false, error: text }));
      return { ok: false, error: text };
    }
    const reader = res.body.getReader();
    const decoder = new TextDecoder();
    let buffer = '';
    while (true) {
      const { value, done } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });
      let nl: number;
      while ((nl = buffer.indexOf('\n')) !== -1) {
        const line = buffer.slice(0, nl).trim();
        buffer = buffer.slice(nl + 1);
        if (!line) continue;
        let evt: { phase?: string; dep?: string; image?: string; message?: string; name?: string; error?: string };
        try {
          evt = JSON.parse(line);
        } catch {
          continue;
        }
        if (evt.phase === 'done' || evt.phase === 'error') {
          finalEvent = evt;
          continue;
        }
        updatePreset(p.name, (x) => ({
          ...x,
          installingPhase: evt.phase || '',
          installingDep: evt.phase === 'starting_deps' ? evt.dep || '' : x.installingDep,
          installingMessage:
            evt.phase === 'pulling_image' ? evt.message || (evt.image ? 'pulling ' + evt.image : '') : ''
        }));
      }
    }
    if (!finalEvent || finalEvent.phase === 'error') {
      const err = finalEvent?.error || 'install failed without a final result';
      updatePreset(p.name, (x) => ({ ...x, installing: false, error: err }));
      return { ok: false, error: err };
    }
    updatePreset(p.name, (x) => ({
      ...x,
      installing: false,
      installingPhase: '',
      installingMessage: '',
      installingDep: ''
    }));
    return { ok: true, name: finalEvent.name || p.name };
  } catch (e) {
    const err = e instanceof Error ? e.message : 'Request failed';
    updatePreset(p.name, (x) => ({ ...x, installing: false, error: err }));
    return { ok: false, error: err };
  }
}

// Install a preset, refresh state, and open the new service. Shared by the
// preset modal, the discovery dashboard, and the suggestion banner so the
// post-install sequence lives in one place. onSuccess runs after state is
// refreshed and before navigation (the modal closes itself there).
export async function installPresetAndOpen(
  p: Preset,
  opts: { onSuccess?: (name: string) => void } = {}
): Promise<InstallResult> {
  const r = await installPreset(p);
  if (r.ok && r.name) {
    await loadServices();
    await loadPresets();
    opts.onSuccess?.(r.name);
    goToTab('services', r.name);
  } else {
    // Refresh so a failed install still reconciles dependency / installed state.
    await loadPresets();
  }
  return r;
}

// exported for tests
export const _apiUrl = apiUrl;
