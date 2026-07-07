import { writable } from 'svelte/store';
import { apiJson } from '$lib/api';

export interface VersionInfo {
  current: string;
  latest: string;
  hasUpdate: boolean;
  checked: boolean;
  checking: boolean;
  changelog: string;
}

const empty: VersionInfo = {
  current: '...',
  latest: '',
  hasUpdate: false,
  checked: false,
  checking: false,
  changelog: ''
};

export const version = writable<VersionInfo>(empty);

interface VersionResponse {
  current?: string;
  latest?: string;
  has_update?: boolean;
  changelog?: string;
}

// loadVersion refreshes the version store. Pass force=true for a user-initiated
// check: it flips `checking` so the button shows a spinner and adds ?refresh so
// the backend queries GitHub live instead of returning the 24h-cached answer.
// The passive on-mount call omits force and stays on the cache.
export async function loadVersion(force = false) {
  version.update((v) => ({ ...v, checking: true }));
  try {
    const res = await apiJson<VersionResponse>(force ? '/api/version?refresh=1' : '/api/version');
    version.set({
      current: res.current ?? '...',
      latest: res.latest ?? '',
      hasUpdate: Boolean(res.has_update),
      checked: true,
      checking: false,
      changelog: res.changelog ?? ''
    });
  } catch {
    version.update((v) => ({ ...v, checking: false }));
  }
}
