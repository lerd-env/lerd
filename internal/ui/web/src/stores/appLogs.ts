import { m } from '../paraglide/messages.js';
import { apiJson, apiFetch } from '$lib/api';

export interface AppLogFile {
  name: string;
  size?: number;
  modified?: string;
}

export interface AppLogEntry {
  level?: string;
  date?: string;
  message?: string;
  detail?: string;
}

function branchQuery(branch?: string): string {
  return branch ? `?branch=${encodeURIComponent(branch)}` : '';
}

export async function listAppLogFiles(domain: string, branch?: string): Promise<AppLogFile[]> {
  try {
    const res = await apiJson<{ files?: AppLogFile[] }>(
      `/api/app-logs/${encodeURIComponent(domain)}${branchQuery(branch)}`
    );
    return Array.isArray(res.files) ? res.files : [];
  } catch {
    return [];
  }
}

export interface ClearAppLogsResult {
  ok: boolean;
  filesCleared: number;
  bytesCleared: number;
  error?: string;
}

// clearAppLogs deletes the project's log files to reclaim disk. The active log
// is recreated by the app on its next write.
export async function clearAppLogs(domain: string, branch?: string): Promise<ClearAppLogsResult> {
  try {
    const res = await apiFetch(
      `/api/app-logs/${encodeURIComponent(domain)}/clear${branchQuery(branch)}`,
      { method: 'POST' }
    );
    const data = (await res.json()) as {
      ok?: boolean;
      files_cleared?: number;
      bytes_cleared?: number;
      error?: string;
    };
    return {
      ok: Boolean(data.ok),
      filesCleared: data.files_cleared ?? 0,
      bytesCleared: data.bytes_cleared ?? 0,
      error: data.error
    };
  } catch (e) {
    return { ok: false, filesCleared: 0, bytesCleared: 0, error: e instanceof Error ? e.message : m.common_requestFailed() };
  }
}

// APP_LOG_PAGE is how many entries one request brings back; older ones are
// fetched a page at a time as the reader scrolls up.
export const APP_LOG_PAGE = 200;

export interface AppLogPage {
  entries: AppLogEntry[];
  more: boolean;
}

// loadAppLogEntries returns one page of entries, newest first, starting
// offset entries back from the newest.
export async function loadAppLogEntries(
  domain: string,
  file: string,
  offset: number,
  branch?: string
): Promise<AppLogPage> {
  try {
    const params = new URLSearchParams({ limit: String(APP_LOG_PAGE), offset: String(offset) });
    if (branch) params.set('branch', branch);
    const res = await apiJson<{ entries?: AppLogEntry[]; more?: boolean }>(
      `/api/app-logs/${encodeURIComponent(domain)}/${encodeURIComponent(file)}?${params.toString()}`
    );
    return { entries: Array.isArray(res.entries) ? res.entries : [], more: res.more === true };
  } catch {
    return { entries: [], more: false };
  }
}

function sameEntry(a: AppLogEntry, b: AppLogEntry): boolean {
  return a.date === b.date && a.level === b.level && a.message === b.message && a.detail === b.detail;
}

// mergeNewest puts a freshly polled newest page in front of what is loaded,
// keeping the older pages the reader scrolled back to. When the page no
// longer reaches the newest loaded entry, too much arrived in between to
// stitch, and the page replaces the list.
export function mergeNewest(loaded: AppLogEntry[], page: AppLogEntry[]): { entries: AppLogEntry[]; replaced: boolean } {
  if (loaded.length === 0) return { entries: page, replaced: true };
  const at = page.findIndex((e) => sameEntry(e, loaded[0]));
  if (at < 0) return { entries: page, replaced: true };
  return { entries: [...page.slice(0, at), ...loaded], replaced: false };
}
