import { m } from '../paraglide/messages.js';
import { writable } from 'svelte/store';
import { apiJson, apiFetch, decodeJSONResult, sharedJson } from '$lib/api';
import { readSSE } from '$lib/sse';
import { confirmDownload } from './downloadConfirm';
import type { SiteNginxBackup, LoadNginxBackupsResult, ResetNginxResult, SaveNginxResult, RestoreNginxResult } from './sites';
import { fetchBackupList, fetchBackupText } from '$lib/backups';

export const phpVersions = writable<string[]>([]);
// phpVersionsLoaded tells "not fetched yet" apart from "none installed", which
// both read as an empty list.
export const phpVersionsLoaded = writable(false);

// Shown wherever a prerelease version is offered, so it never reads as an
// ordinary choice. Short enough to sit inside the version card. Generic, since
// the stage is only known once a build exists to read it from.
export const PRERELEASE_LABEL = 'preview';

// prereleaseStage reads the stage from a probed build ("8.6.0RC1" → "RC"), which
// moves through alpha, beta and RC upstream without a lerd release.
export function prereleaseStage(build: string): string {
  const stage = /(alpha|beta|RC)\d*$/i.exec(build)?.[1];
  if (!stage) return PRERELEASE_LABEL;
  return stage.toLowerCase() === 'rc' ? 'RC' : stage.toLowerCase();
}

export interface PhpOption {
  value: string;
  disabled?: boolean;
  description?: string;
}

// cmpVersion compares two "major.minor" strings numerically.
function cmpVersion(a: string, b: string): number {
  const [aMaj, aMin] = a.split('.').map((n) => parseInt(n, 10) || 0);
  const [bMaj, bMin] = b.split('.').map((n) => parseInt(n, 10) || 0);
  return aMaj !== bMaj ? aMaj - bMaj : aMin - bMin;
}

// outOfFrameworkRange reports whether a PHP version falls outside the
// framework's [min, max] range. An empty bound means unconstrained on that side.
function outOfFrameworkRange(v: string, min?: string, max?: string): boolean {
  if (min && cmpVersion(v, min) < 0) return true;
  if (max && cmpVersion(v, max) > 0) return true;
  return false;
}

// phpOptionsForSite returns the options to offer in a site's PHP dropdown.
// FrankenPHP sites are limited to the versions dunglas/frankenphp publishes an
// image for, intersected with what's installed, plus the site's current version
// so the control never renders blank. Other runtimes offer every installed one.
// When the framework declares a PHP range (min/max), versions outside it are
// kept in the list but disabled, so the constraint is visible rather than
// silently hidden. The site's current version is never disabled. min/max are
// empty when the framework version was guessed, leaving every version enabled.
// A version upstream has not released is marked, so a beta is never picked in
// the belief it gets security updates.
export function phpOptionsForSite(
  runtime: string | undefined,
  installed: string[],
  frankenphpVersions: string[],
  current: string,
  min?: string,
  max?: string,
  prerelease: string[] = []
): PhpOption[] {
  const base =
    runtime === 'frankenphp'
      ? frankenphpVersions.filter((v) => installed.includes(v) || v === current)
      : installed;
  return base.map((v) => {
    const disabled = v !== current && outOfFrameworkRange(v, min, max);
    if (disabled) return { value: v, disabled: true, description: `needs PHP ${min || '*'} to ${max || '*'}` };
    return prerelease.includes(v) ? { value: v, description: PRERELEASE_LABEL } : { value: v };
  });
}

export async function loadPhpVersions() {
  try {
    const list = await sharedJson<string[]>('/api/php-versions');
    phpVersions.set(Array.isArray(list) ? list : []);
    phpVersionsLoaded.set(true);
  } catch {
    /* keep previous */
  }
}

// installablePhpVersions returns the supported PHP versions not yet installed,
// so the add-version modal can offer them in a dropdown. Throws on request
// failure so the caller can distinguish an error from an empty (all-installed)
// list rather than silently showing "all installed".
export async function installablePhpVersions(): Promise<string[]> {
  const list = await apiJson<string[]>('/api/php-installable');
  return Array.isArray(list) ? list : [];
}

export interface PhpInstallEvent {
  line?: string;
  done?: boolean;
  ok?: boolean;
  version?: string;
  error?: string;
}

// readBuildStream turns a build's SSE stream into log lines and the final
// done payload, shared by install and rebuild.
async function readBuildStream(res: Response, onEvent: (e: PhpInstallEvent) => void): Promise<void> {
  await readSSE(res, (event, data) => {
    if (event === 'done') {
      try {
        const r = JSON.parse(data) as { ok?: boolean; version?: string; error?: string };
        onEvent({ done: true, ok: Boolean(r.ok), version: r.version, error: r.error });
      } catch {
        onEvent({ done: true, ok: false, error: 'bad done payload' });
      }
    } else {
      onEvent({ line: data });
    }
  });
}

// streamPhpInstall POSTs to the SSE endpoint and invokes onEvent for each build
// log line and the final done payload. Mirrors streamWorktreeAdd. Pass a signal
// to abort the client read when the modal closes; the server build continues and
// reports its result via a push notification.
export async function streamPhpInstall(
  version: string,
  onEvent: (e: PhpInstallEvent) => void,
  signal?: AbortSignal
): Promise<void> {
  const res = await apiFetch('/api/php-versions/install?version=' + encodeURIComponent(version), {
    method: 'POST',
    signal
  });
  await readBuildStream(res, onEvent);
}

// confirmPhpDownload asks before a PHP build spends bandwidth on the prebuilt
// base image it starts from. Resolves true with no prompt when that base is
// already on the machine, which is the common case for a repeat rebuild.
export function confirmPhpDownload(version: string): Promise<boolean> {
  return confirmDownload('PHP ' + version, { php: version });
}

// streamPhpRebuild force-rebuilds a version's image against the current base,
// streaming the build log the same way an install does.
export async function streamPhpRebuild(
  version: string,
  onEvent: (e: PhpInstallEvent) => void,
  signal?: AbortSignal
): Promise<void> {
  const res = await apiFetch('/api/php-versions/' + encodeURIComponent(version) + '/rebuild', {
    method: 'POST',
    signal
  });
  await readBuildStream(res, onEvent);
}

export interface PhpUpdateResult {
  name: string;
  ok: boolean;
  error?: string;
}

// updatePhpVersion runs one rebuild and always answers: a failure is carried in
// the result so the rest of a batch still runs.
async function updatePhpVersion(
  version: string,
  online?: (version: string, line: string) => void
): Promise<PhpUpdateResult> {
  // Stays a failure unless the stream reaches its done frame with ok.
  let result: PhpUpdateResult = { name: version, ok: false, error: m.common_failed() };
  try {
    await streamPhpRebuild(version, (ev) => {
      if (ev.done) result = ev.ok ? { name: version, ok: true } : { name: version, ok: false, error: ev.error || m.common_failed() };
      else if (ev.line?.trim()) online?.(version, ev.line);
    });
  } catch (e) {
    result = { name: version, ok: false, error: e instanceof Error ? e.message : m.common_requestFailed() };
  }
  return result;
}

// updatePhpVersions follows the terminal: images rebuild together like `lerd
// php:rebuild`, with the server taking restarts in turn, and native builds go
// one at a time like `lerd php:update`. The batch is already confirmed.
export async function updatePhpVersions(
  versions: string[],
  parallel: boolean,
  onresult?: (result: PhpUpdateResult) => void,
  online?: (version: string, line: string) => void
): Promise<PhpUpdateResult[]> {
  const one = async (version: string) => {
    const result = await updatePhpVersion(version, online);
    onresult?.(result);
    return result;
  };
  if (parallel) return Promise.all(versions.map(one));
  const results: PhpUpdateResult[] = [];
  for (const version of versions) results.push(await one(version));
  return results;
}

// BaseImageStatus compares a version's image against the prebuilt base it was
// built from. null when there is nothing to report: no recorded base (a local
// build) or a registry that could not answer.
export interface BaseImageStatus {
  version: string;
  ref?: string;
  built_digest?: string;
  latest_digest?: string;
  stale: boolean;
}

// checkPhpUpdates forces a fresh registry read for a version's base image,
// bypassing the digest cache. ok is false when the request itself failed.
export async function checkPhpUpdates(
  version: string
): Promise<{ ok: boolean; status: BaseImageStatus | null }> {
  try {
    const res = await apiFetch('/api/php-versions/' + encodeURIComponent(version) + '/updates', {
      method: 'POST'
    });
    if (!res.ok) return { ok: false, status: null };
    const data = (await res.json().catch(() => null)) as BaseImageStatus | null;
    return { ok: true, status: data };
  } catch {
    return { ok: false, status: null };
  }
}

// openPhpShell asks the server to spawn a host terminal running an interactive
// shell inside the version's FPM container, the same session `lerd php:shell`
// gives. Returns the server's error so a missing terminal emulator is reported
// rather than looking like a click that did nothing.
export async function openPhpShell(v: string): Promise<{ ok: boolean; error?: string }> {
  try {
    const res = await apiFetch('/api/php-versions/' + encodeURIComponent(v) + '/shell', {
      method: 'POST'
    });
    const data = await decodeJSONResult<{ ok?: boolean; error?: string }>(res);
    return { ok: Boolean(data.ok), error: data.error };
  } catch (e) {
    return { ok: false, error: e instanceof Error ? e.message : m.common_requestFailed() };
  }
}

async function phpAction(v: string, action: 'set-default' | 'start' | 'stop' | 'remove'): Promise<boolean> {
  try {
    const res = await apiFetch('/api/php-versions/' + encodeURIComponent(v) + '/' + action, {
      method: 'POST'
    });
    return res.ok;
  } catch {
    return false;
  }
}

export const setDefaultPhp = (v: string) => phpAction(v, 'set-default');
export const startPhp = (v: string) => phpAction(v, 'start');
export const stopPhp = (v: string) => phpAction(v, 'stop');
export const removePhp = (v: string) => phpAction(v, 'remove');

// setFpmPorts replaces the extra host ports published on a version's shared FPM
// container. The server shifts any colliding host port to the next free one, so
// the returned list may differ from what was sent; the status broadcast carries
// the resolved set, so the caller reseeds from it rather than the response.
export async function setFpmPorts(
  version: string,
  ports: string[]
): Promise<{ ok: boolean; error?: string; ports?: string[] }> {
  try {
    const res = await apiFetch('/api/php-versions/' + encodeURIComponent(version) + '/ports', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ ports })
    });
    const data = (await res.json().catch(() => ({}))) as {
      ok?: boolean;
      error?: string;
      ports?: string[];
    };
    if (res.ok && data.ok) return { ok: true, ports: data.ports };
    return { ok: false, error: data.error || m.common_failed() };
  } catch (e) {
    return { ok: false, error: e instanceof Error ? e.message : m.common_requestFailed() };
  }
}

export interface PhpIni {
  path: string;
  content: string;
  exists: boolean;
}

function phpConfigUrl(v: string, suffix: string = ''): string {
  return '/api/php-versions/' + encodeURIComponent(v) + '/config' + (suffix ? '/' + suffix : '');
}

export async function getPhpIni(v: string): Promise<PhpIni> {
  return apiJson<PhpIni>(phpConfigUrl(v));
}

export async function savePhpIni(v: string, content: string, backup: boolean = false): Promise<SaveNginxResult> {
  try {
    const res = await apiFetch(phpConfigUrl(v), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ content, backup })
    });
    const data = await decodeJSONResult<{
      ok?: boolean;
      error?: string;
      backup_name?: string;
      content?: string;
      exists?: boolean;
    }>(res);
    return {
      ok: Boolean(data.ok),
      error: data.error,
      backupName: data.backup_name,
      content: data.content,
      exists: data.exists
    };
  } catch (e) {
    return { ok: false, error: e instanceof Error ? e.message : m.common_requestFailed() };
  }
}

export async function loadPhpIniBackups(v: string): Promise<LoadNginxBackupsResult> {
  return fetchBackupList<SiteNginxBackup>(phpConfigUrl(v, 'backups'));
}

export async function loadPhpIniBackupContent(v: string, name: string): Promise<string> {
  return fetchBackupText(phpConfigUrl(v, 'backups/' + encodeURIComponent(name)));
}

export async function resetPhpIni(v: string): Promise<ResetNginxResult> {
  try {
    const res = await apiFetch(phpConfigUrl(v, 'reset'), { method: 'POST' });
    const data = await decodeJSONResult<{ ok?: boolean; error?: string }>(res);
    return { ok: Boolean(data.ok), error: data.error };
  } catch (e) {
    return { ok: false, error: e instanceof Error ? e.message : m.common_requestFailed() };
  }
}

export async function restorePhpIni(v: string, name: string = ''): Promise<RestoreNginxResult> {
  try {
    const res = await apiFetch(phpConfigUrl(v, 'restore'), {
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

export interface PhpSetState {
  declared: string[];
  has: string[] | null;
  cannot: string[] | null;
}

export interface PhpExtensionsReport {
  version: string;
  built: boolean;
  needs_rebuild: boolean;
  extensions: PhpSetState;
  packages: PhpSetState;
  modules?: string[];
}

export interface PhpExtensionsResult {
  ok: boolean;
  error?: string;
  report?: PhpExtensionsReport;
  modules_error?: string;
}

// fetchPhpExtensions reads what a version's image actually carries. Reading
// `php -m` starts a container, so this is only called when the tab is opened;
// the backend caches the result against the image ID.
export async function fetchPhpExtensions(v: string): Promise<PhpExtensionsResult> {
  try {
    const res = await apiFetch('/api/php-versions/' + encodeURIComponent(v) + '/extensions');
    const data = await decodeJSONResult<PhpExtensionsResult>(res);
    return { ok: Boolean(data.ok), error: data.error, report: data.report, modules_error: data.modules_error };
  } catch (e) {
    return { ok: false, error: e instanceof Error ? e.message : m.common_requestFailed() };
  }
}
