import { writable } from 'svelte/store';
import { apiFetch, apiJson, apiUrl, decodeJSONResult, decodeJSONText } from '$lib/api';
import { m } from '../paraglide/messages.js';

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
  // Worktree branch this database is the isolated one for, when it is.
  branch?: string;
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

// ImportIssue is one complaint the engine made while swallowing a dump, with
// how often it made it. A load can come back ok and still carry these, since
// psql exits clean even when every statement in a dump failed.
export interface ImportIssue {
  message: string;
  count: number;
}

type Result = {
  ok: boolean;
  error?: string;
  errors?: number;
  issues?: ImportIssue[];
  // Distinct complaints dropped past the cap, so a trimmed list never reads as
  // the whole of what went wrong.
  omitted?: number;
  // What the daemon held back on the way in, so a load that came out clean
  // because lerd filtered it says so rather than looking untouched.
  skipped?: ImportIssue[];
};

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

// ImportProgress tracks a dump on its way into the engine. The daemon streams
// the request body straight into the container, so the bytes the browser has
// handed over are the bytes the engine has taken.
export interface ImportProgress {
  percent: number;
  uploaded: boolean;
}

// importDatabase uploads over XHR rather than fetch because only XHR reports
// how much of the dump has gone out, which is the whole of the progress the UI
// can show for an import.
export function importDatabase(
  service: string,
  database: string,
  file: File,
  onProgress?: (p: ImportProgress) => void,
  fresh = false
): Promise<Result> {
  const form = new FormData();
  // Every field goes before the file because the daemon walks the parts in order
  // and streams the file straight into the engine without buffering the body.
  form.append('database', database);
  if (fresh) form.append('fresh', 'true');
  form.append('file', file);
  return new Promise<Result>((resolve) => {
    const finish = async (out: Result) => {
      if (out.ok) await loadEngine(service);
      resolve(out);
    };
    const xhr = new XMLHttpRequest();
    xhr.open('POST', apiUrl(`/api/databases/${encodeURIComponent(service)}/import`));
    xhr.setRequestHeader('X-Lerd-CSRF', '1');
    xhr.upload.onprogress = (e) => {
      if (!onProgress || !e.lengthComputable || !e.total) return;
      onProgress({ percent: e.loaded / e.total, uploaded: e.loaded >= e.total });
    };
    xhr.onload = () =>
      void finish(decodeJSONText<Result>(xhr.responseText, `${xhr.status} ${xhr.statusText}`.trim()));
    xhr.onerror = () => resolve({ ok: false, error: m.common_requestFailed() });
    xhr.send(form);
  });
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
