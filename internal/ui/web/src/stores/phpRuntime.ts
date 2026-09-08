import { writable } from 'svelte/store';
import { apiJson, apiFetch } from '$lib/api';
import { readSSE } from '$lib/sse';
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
export async function setPHPRuntime(
  mode: PHPRuntime,
  removeImages = false,
  onLine?: (line: string) => void
) {
  phpRuntimeLoading.set(true);
  try {
    const res = await apiFetch('/api/settings/php-runtime', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ mode, remove_images: removeImages })
    });
    // Streamed: a switch rewrites every site and can spend minutes rebuilding
    // images that were removed, so the progress is shown rather than hidden
    // behind a spinner.
    let result: {
      ok?: boolean;
      error?: string;
      images_removed?: number;
      images_error?: string;
    } = {};
    await readSSE(res, (event, data) => {
      if (event === 'done') {
        try {
          result = JSON.parse(data);
        } catch {
          result = { ok: false, error: 'bad done payload' };
        }
        return;
      }
      onLine?.(data);
    });
    if (result.ok) phpRuntime.set(mode);
    return {
      ok: Boolean(result.ok),
      error: result.error,
      imagesRemoved: result.images_removed ?? 0,
      imagesError: result.images_error
    };
  } finally {
    phpRuntimeLoading.set(false);
  }
}
