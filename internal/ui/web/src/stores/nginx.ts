import { m } from '../paraglide/messages.js';
import { apiJson, apiFetch, decodeJSONResult } from '$lib/api';
import type { SiteNginxBackup, LoadNginxBackupsResult, ResetNginxResult, SaveNginxResult, RestoreNginxResult } from './sites';
import { fetchBackupList, fetchBackupText } from '$lib/backups';

/** Which global settings file a call edits: nginx's http-level override, or
 *  lerd's own web server's settings for every site. Same shapes, other base. */
export type GlobalConfigTarget = 'nginx' | 'web';
const base = (t: GlobalConfigTarget) => (t === 'web' ? '/api/web' : '/api/nginx');

export interface NginxConfig {
  path: string;
  content: string;
  exists: boolean;
}

export async function getNginxConfig(target: GlobalConfigTarget = 'nginx'): Promise<NginxConfig> {
  return apiJson<NginxConfig>(base(target) + '/config');
}

export async function saveNginxConfig(content: string, backup: boolean = false, target: GlobalConfigTarget = 'nginx'): Promise<SaveNginxResult> {
  try {
    const res = await apiFetch(base(target) + '/config', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ content, backup })
    });
    const data = await decodeJSONResult<{
      ok?: boolean;
      error?: string;
      backup_name?: string;
      validation_output?: string;
      content?: string;
      exists?: boolean;
    }>(res);
    return {
      ok: Boolean(data.ok),
      error: data.error,
      backupName: data.backup_name,
      validationOutput: data.validation_output,
      content: data.content,
      exists: data.exists
    };
  } catch (e) {
    return { ok: false, error: e instanceof Error ? e.message : m.common_requestFailed() };
  }
}

export async function loadNginxConfigBackups(target: GlobalConfigTarget = 'nginx'): Promise<LoadNginxBackupsResult> {
  return fetchBackupList<SiteNginxBackup>(base(target) + '/backups');
}

export async function loadNginxConfigBackupContent(name: string, target: GlobalConfigTarget = 'nginx'): Promise<string> {
  return fetchBackupText(base(target) + '/backups/' + encodeURIComponent(name));
}

export async function resetNginxConfig(target: GlobalConfigTarget = 'nginx'): Promise<ResetNginxResult> {
  try {
    const res = await apiFetch(base(target) + '/reset', { method: 'POST' });
    const data = await decodeJSONResult<{ ok?: boolean; error?: string }>(res);
    return { ok: Boolean(data.ok), error: data.error };
  } catch (e) {
    return { ok: false, error: e instanceof Error ? e.message : m.common_requestFailed() };
  }
}

export async function restoreNginxConfig(name: string = '', target: GlobalConfigTarget = 'nginx'): Promise<RestoreNginxResult> {
  try {
    const res = await apiFetch(base(target) + '/restore', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name })
    });
    const data = await decodeJSONResult<{ ok?: boolean; error?: string; restored?: string; content?: string }>(res);
    return { ok: Boolean(data.ok), error: data.error, restored: data.restored, content: data.content };
  } catch (e) {
    return { ok: false, error: e instanceof Error ? e.message : m.common_requestFailed() };
  }
}
