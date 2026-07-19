import { writable } from 'svelte/store';
import { apiFetch, apiJson, apiUrl, decodeJSONResult } from '$lib/api';

export interface Snapshot {
  name: string;
  created: string;
  database: string;
  size_bytes: number;
}

export interface DatabaseEntry {
  name: string;
  size_bytes: number;
  // Domain of the linked site that owns this database, when one does.
  site?: string;
  snapshots: Snapshot[];
}

export interface DatabaseEngine {
  service: string;
  family: string;
  status: string;
  port?: number;
  icon?: string;
  connection_url?: string;
  supports_create: boolean;
  supports_snapshot: boolean;
  databases: DatabaseEntry[];
  error?: string;
}

// databases holds the engines the UI has loaded so far, keyed by service name.
// The service detail view loads only the engine it is showing and reads it back
// from here, so mutations that refresh one engine stay reactive.
export const databases = writable<DatabaseEngine[]>([]);

function upsert(engine: DatabaseEngine): void {
  databases.update((list) => {
    const next = list.filter((e) => e.service !== engine.service);
    next.push(engine);
    return next;
  });
}

// loadEngine fetches a single engine and merges it into the store. Failures keep
// the last good copy rather than blanking the view.
export async function loadEngine(service: string): Promise<void> {
  try {
    upsert(await apiJson<DatabaseEngine>(`/api/databases/${encodeURIComponent(service)}`));
  } catch {
    /* keep the last good copy */
  }
}

type Result = { ok: boolean; error?: string };

async function post(service: string, path: string, body: unknown): Promise<Result> {
  try {
    const res = await apiFetch(path, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body)
    });
    const out = await decodeJSONResult<Result>(res);
    if (out.ok) await loadEngine(service);
    return out;
  } catch (e) {
    return { ok: false, error: e instanceof Error ? e.message : String(e) };
  }
}

export function createDatabase(service: string, name: string): Promise<Result> {
  return post(service, `/api/databases/${encodeURIComponent(service)}/create`, { name });
}

export function dropDatabase(service: string, name: string): Promise<Result> {
  return post(service, `/api/databases/${encodeURIComponent(service)}/drop`, { name });
}

export function takeSnapshot(service: string, database: string, name: string): Promise<Result> {
  return post(service, `/api/databases/${encodeURIComponent(service)}/snapshot`, { database, name });
}

export function restoreSnapshot(service: string, database: string, name: string): Promise<Result> {
  return post(service, `/api/databases/${encodeURIComponent(service)}/snapshot/restore`, {
    database,
    name
  });
}

export function deleteSnapshot(service: string, database: string, name: string): Promise<Result> {
  return post(service, `/api/databases/${encodeURIComponent(service)}/snapshot/delete`, {
    database,
    name
  });
}

export async function importDatabase(service: string, database: string, file: File): Promise<Result> {
  try {
    const form = new FormData();
    form.append('database', database);
    form.append('file', file);
    const res = await apiFetch(`/api/databases/${encodeURIComponent(service)}/import`, {
      method: 'POST',
      body: form
    });
    const out = await decodeJSONResult<Result>(res);
    if (out.ok) await loadEngine(service);
    return out;
  } catch (e) {
    return { ok: false, error: e instanceof Error ? e.message : String(e) };
  }
}

// exportUrl points the browser at the streaming dump endpoint so the file is
// saved through the normal download path.
export function exportUrl(service: string, database: string): string {
  return apiUrl(
    `/api/databases/${encodeURIComponent(service)}/export?database=${encodeURIComponent(database)}`
  );
}

// snapshotExportUrl downloads a stored snapshot as a plain .sql dump.
export function snapshotExportUrl(service: string, database: string, name: string): string {
  return apiUrl(
    `/api/databases/${encodeURIComponent(service)}/snapshot/export?database=${encodeURIComponent(database)}&name=${encodeURIComponent(name)}`
  );
}

// dsnFor rewrites the engine's connection string to target one database, so a
// user can copy a ready-to-paste DSN even when no admin tool is installed.
export function dsnFor(engine: DatabaseEngine, database: string): string {
  const raw = engine.connection_url;
  if (!raw) return '';
  try {
    const u = new URL(raw);
    u.pathname = '/' + database;
    return u.toString();
  } catch {
    return raw;
  }
}
