import { writable } from 'svelte/store';
import { apiFetch, apiJson, apiUrl, decodeJSONResult, decodeJSONText } from '$lib/api';
import { m } from '../paraglide/messages.js';

// The generic entity overview: a service's preset declares what it holds
// (buckets, keyspaces…), the columns its list prints and the actions it
// supports. The UI renders whatever arrives without knowing the engine.

export interface EntityColumn {
  key: string;
  label?: string;
  // Display hint: 'bytes', 'number' or plain text.
  format?: string;
}

export interface EntityAction {
  name: string;
  destructive?: boolean;
}

export interface EntityRow {
  name: string;
  // Column values in the declared column order, as the engine printed them.
  values?: string[];
  // Domain of the linked site that owns this entity, when one does.
  site?: string;
}

export interface EntityKind {
  kind: string;
  label?: string;
  columns: EntityColumn[];
  actions: EntityAction[];
  rows: EntityRow[];
  error?: string;
}

// entities holds the loaded kinds keyed by service name. The detail view loads
// only the service it is showing.
export const entities = writable<Record<string, EntityKind[]>>({});

// loadEntities fetches a service's kinds and merges them in. Failures keep the
// last good copy rather than blanking the view.
export async function loadEntities(service: string): Promise<void> {
  try {
    const kinds = await apiJson<EntityKind[]>(`/api/entities/${encodeURIComponent(service)}`);
    entities.update((all) => ({ ...all, [service]: kinds }));
  } catch {
    /* keep the last good copy */
  }
}

export interface EntityResult {
  ok: boolean;
  error?: string;
}

export async function runEntityAction(
  service: string,
  kind: string,
  action: string,
  name: string
): Promise<EntityResult> {
  try {
    const res = await apiFetch(
      `/api/entities/${encodeURIComponent(service)}/${encodeURIComponent(kind)}/${encodeURIComponent(action)}`,
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name })
      }
    );
    const out = await decodeJSONResult<EntityResult>(res);
    if (out.ok) await loadEntities(service);
    return out;
  } catch (e) {
    return { ok: false, error: e instanceof Error ? e.message : String(e) };
  }
}

// kindLabel is the display name for a kind: the declared label, else the kind
// slug capitalized.
export function kindLabel(kind: { kind: string; label?: string }): string {
  if (kind.label) return kind.label;
  return kind.kind.charAt(0).toUpperCase() + kind.kind.slice(1);
}

// entityExportUrl points the browser at the streaming archive endpoint so the
// file is saved through the normal download path.
export function entityExportUrl(service: string, kind: string, name: string): string {
  return apiUrl(
    `/api/entities/${encodeURIComponent(service)}/${encodeURIComponent(kind)}/export?name=${encodeURIComponent(name)}`
  );
}

// EntityImportProgress tracks an archive on its way in; the daemon streams the
// body straight into the declared import, so the bytes the browser has handed
// over are the bytes the command has taken.
export interface EntityImportProgress {
  percent: number;
  uploaded: boolean;
}

// importEntity uploads over XHR rather than fetch because only XHR reports how
// much of the archive has gone out, the same trade the database import makes.
export function importEntity(
  service: string,
  kind: string,
  name: string,
  file: File,
  onProgress?: (p: EntityImportProgress) => void
): Promise<EntityResult> {
  const form = new FormData();
  // The name goes before the file because the daemon walks the parts in order
  // and streams the file without buffering the body.
  form.append('name', name);
  form.append('file', file);
  return new Promise<EntityResult>((resolve) => {
    const finish = async (out: EntityResult) => {
      if (out.ok) await loadEntities(service);
      resolve(out);
    };
    const xhr = new XMLHttpRequest();
    xhr.open(
      'POST',
      apiUrl(`/api/entities/${encodeURIComponent(service)}/${encodeURIComponent(kind)}/import`)
    );
    xhr.setRequestHeader('X-Lerd-CSRF', '1');
    xhr.upload.onprogress = (e) => {
      if (!onProgress || !e.lengthComputable || !e.total) return;
      onProgress({ percent: e.loaded / e.total, uploaded: e.loaded >= e.total });
    };
    xhr.onload = () =>
      void finish(
        decodeJSONText<EntityResult>(xhr.responseText, `${xhr.status} ${xhr.statusText}`.trim())
      );
    xhr.onerror = () => resolve({ ok: false, error: m.common_requestFailed() });
    xhr.send(form);
  });
}
