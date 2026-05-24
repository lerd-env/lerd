import { writable } from 'svelte/store';
import { apiJson, apiFetch } from '$lib/api';

export interface PhpExtension {
  name: string;
  apk_deps?: string[];
}

interface ListResponse {
  version: string;
  extensions: PhpExtension[];
}

interface ActionResponse {
  ok: boolean;
  error?: string;
}

// Per-version map so the dashboard can swap PHP tabs without a fetch.
export const phpExtensions = writable<Record<string, PhpExtension[]>>({});

export async function loadPhpExtensions(version: string): Promise<void> {
  try {
    const res = await apiJson<ListResponse>(
      '/api/php-versions/' + encodeURIComponent(version) + '/extensions'
    );
    phpExtensions.update((m) => ({ ...m, [version]: res.extensions ?? [] }));
  } catch {
    /* swallow: keep previous state */
  }
}

export async function addPhpExtension(
  version: string,
  ext: string,
  apkDeps: string[] = []
): Promise<ActionResponse> {
  try {
    const res = await apiFetch(
      '/api/php-versions/' + encodeURIComponent(version) + '/extensions',
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ extension: ext, apk_deps: apkDeps })
      }
    );
    const json = (await res.json()) as ActionResponse;
    if (json.ok) await loadPhpExtensions(version);
    return json;
  } catch (e) {
    return { ok: false, error: e instanceof Error ? e.message : String(e) };
  }
}

export async function removePhpExtension(
  version: string,
  ext: string
): Promise<ActionResponse> {
  try {
    const res = await apiFetch(
      '/api/php-versions/' +
        encodeURIComponent(version) +
        '/extensions/' +
        encodeURIComponent(ext),
      { method: 'DELETE' }
    );
    const json = (await res.json()) as ActionResponse;
    if (json.ok) await loadPhpExtensions(version);
    return json;
  } catch (e) {
    return { ok: false, error: e instanceof Error ? e.message : String(e) };
  }
}
