import { writable } from 'svelte/store';
import { apiJson, apiFetch } from '$lib/api';
import { m } from '../paraglide/messages.js';

export type PHPRuntime = 'container' | 'native';

export const phpRuntime = writable<PHPRuntime>('container');
export const phpRuntimeApplies = writable<boolean>(false);
export const phpRuntimeLoading = writable<boolean>(false);

interface SettingsResponse {
  php_runtime?: string;
  php_runtime_applies?: boolean;
}

// Anything unrecognised is treated as container, matching the daemon: a
// mistyped value must never leave the dashboard claiming a runtime nothing
// is serving.
function normalize(v: string | undefined): PHPRuntime {
  return v === 'native' ? 'native' : 'container';
}

export async function loadPHPRuntime() {
  try {
    const res = await apiJson<SettingsResponse>('/api/settings');
    phpRuntime.set(normalize(res.php_runtime));
    phpRuntimeApplies.set(Boolean(res.php_runtime_applies));
  } catch {
    phpRuntimeApplies.set(false);
  }
}

// The switch rewrites every site's .env, vhost and workers and stops or starts
// the FPM containers, so it is slow enough to need its own loading state.
export async function setPHPRuntime(mode: PHPRuntime, removeImages = false) {
  phpRuntimeLoading.set(true);
  try {
    const res = await apiFetch('/api/settings/php-runtime', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ mode, remove_images: removeImages })
    });
    const data = (await res.json()) as {
      ok?: boolean;
      error?: string;
      images_removed?: number;
      images_error?: string;
    };
    if (data.ok) phpRuntime.set(mode);
    // The reclaim runs after the switch, so it can fail on its own without the
    // switch having failed. Reported separately for that reason.
    return {
      ok: Boolean(data.ok),
      error: data.error,
      imagesRemoved: data.images_removed ?? 0,
      imagesError: data.images_error
    };
  } catch (e) {
    return { ok: false, error: e instanceof Error ? e.message : m.common_requestFailed() };
  } finally {
    phpRuntimeLoading.set(false);
  }
}
