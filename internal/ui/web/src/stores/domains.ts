import { apiFetch } from '$lib/api';
import type { Site } from './sites';

interface ActionResult {
  ok: boolean;
  error?: string;
  warning?: string;
}

async function post(path: string): Promise<ActionResult> {
  try {
    const res = await apiFetch(path, { method: 'POST' });
    const data = (await res.json()) as ActionResult;
    return { ok: Boolean(data.ok), error: data.error, warning: data.warning };
  } catch (e) {
    return { ok: false, error: e instanceof Error ? e.message : 'Request failed' };
  }
}

// tldParam appends &tld=<tld> when a non-default ending is chosen. An empty or
// undefined tld leaves the backend on the global default.
function tldParam(tld?: string): string {
  return tld ? `&tld=${encodeURIComponent(tld)}` : '';
}

export function addDomain(site: Site, name: string, tld?: string) {
  return post(
    `/api/sites/${encodeURIComponent(site.domain)}/domain:add?name=${encodeURIComponent(name)}${tldParam(tld)}`
  );
}

export function editDomain(site: Site, oldName: string, newName: string, tld?: string) {
  return post(
    `/api/sites/${encodeURIComponent(site.domain)}/domain:edit?old=${encodeURIComponent(
      oldName
    )}&new=${encodeURIComponent(newName)}${tldParam(tld)}`
  );
}

export function removeDomain(site: Site, name: string, tld?: string) {
  return post(
    `/api/sites/${encodeURIComponent(site.domain)}/domain:remove?name=${encodeURIComponent(name)}${tldParam(tld)}`
  );
}
