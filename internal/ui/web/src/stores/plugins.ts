import { writable, derived } from 'svelte/store';
import { apiFetch, apiJson } from '$lib/api';
import { wsMessage } from '$lib/ws';

export interface PluginPage {
  title: string;
  url: string;
  icon?: string;
}

export interface PluginInfo {
  name: string;
  description?: string;
  page?: PluginPage;
  site_tab?: PluginPage;
  overview_card?: PluginPage;
  // The catalog approved a newer commit than the one installed.
  update?: boolean;
}

// Where an author learns how to get a plugin listed.
export const PLUGIN_SUBMIT_GUIDE = 'https://github.com/lerd-env/plugins/blob/main/SUBMITTING.md';

export const plugins = writable<PluginInfo[]>([]);

// Plugins that draw a page on the rail, and those that add a tab to every site.
export const railPlugins = derived(plugins, ($p) => $p.filter((p) => p.page));
export const siteTabPlugins = derived(plugins, ($p) => $p.filter((p) => p.site_tab));
// How many installed plugins have a newer approved commit, for the rail's badge.
export const pluginUpdates = derived(plugins, ($p) => $p.filter((p) => p.update).length);

// A plugin the catalog revoked for a security problem: stopped until the user
// removes it or chooses to keep using it.
export interface PluginRevocation {
  name: string;
  reason: string;
  at: string;
}

export const revokedPlugins = writable<PluginRevocation[]>([]);

export async function loadPlugins() {
  // A lerd-ui from before plugins answers this path with something else.
  try {
    const list = await apiJson<PluginInfo[]>('/api/plugins');
    plugins.set(Array.isArray(list) ? list : []);
  } catch {
    plugins.set([]);
  }
  try {
    const revoked = await apiJson<PluginRevocation[]>('/api/plugin-revocations');
    revokedPlugins.set(Array.isArray(revoked) ? revoked : []);
  } catch {
    revokedPlugins.set([]);
  }
}

// The hourly catalog check refreshes the badge and the update buttons, which
// otherwise load only on mount.
wsMessage.subscribe((msg) => {
  if (msg?.type !== 'plugins') return;
  void loadPlugins();
  loadManagedPlugins().catch(() => {});
});

// One thing a plugin can put on a site, its page script or its Overview card,
// and whether the site has it switched on.
export type PagePluginKind = 'page_script' | 'overview_card' | 'site_tab';
export interface PagePlugin {
  name: string;
  description?: string;
  kind: PagePluginKind;
  on: boolean;
}

// The switches per site name, shared so the Overview shows a card the moment
// the header's menu switches it on.
export const siteSwitches = writable<Record<string, PagePlugin[]>>({});
const keep = (site: string, list: PagePlugin[]) => {
  siteSwitches.update((all) => ({ ...all, [site]: list }));
  return list;
};
// switched counts the switches made per site, so a read that left before one
// is dropped rather than landing after it with the old state.
const switched: Record<string, number> = {};

export async function loadPagePlugins(site: string): Promise<PagePlugin[]> {
  const before = switched[site] ?? 0;
  const raw = await apiJson<PagePlugin[]>('/api/page-plugins?site=' + encodeURIComponent(site));
  // A lerd-ui from before Overview cards sends no kind: those were page scripts.
  const list = Array.isArray(raw) ? raw.map((p) => ({ ...p, kind: p.kind ?? 'page_script' })) : [];
  return (switched[site] ?? 0) === before ? keep(site, list) : list;
}

// setPagePlugin resolves to the site's switches after this one, or throws
// with lerd's reason.
export async function setPagePlugin(site: string, plugin: string, kind: PagePluginKind, on: boolean): Promise<PagePlugin[]> {
  const res = await apiFetch('/api/page-plugins', {
    method: 'POST',
    body: JSON.stringify({ site, plugin, kind, on }),
    headers: { 'Content-Type': 'application/json' }
  });
  if (!res.ok) throw new Error((await res.text()).trim() || res.statusText);
  switched[site] = (switched[site] ?? 0) + 1;
  return keep(site, await res.json());
}

// A row on the plugin manager. A plugin that failed to load has an error and
// nothing else, so it can still be removed.
export interface ManagedPlugin {
  name: string;
  version?: string;
  description?: string;
  adds?: string[];
  requires?: string[];
  error?: string;
  // Its rail icon, when it has a page.
  icon?: string;
  // Who the catalog lists it by, for one installed from the catalog.
  author?: string;
  // Where the catalog installed it from; absent when not listed.
  repo?: string;
  commit?: string;
  // A newer approved commit, when there is one, its version and GitHub's
  // comparison of the two commits.
  update?: string;
  update_version?: string;
  changes?: string;
  // Its latest failure, with a filled-in issue on its repository when known.
  failure?: PluginFailure;
  // What it adds, named the way the user meets each; its screenshots' URLs;
  // and whether it ships docs to read in the dashboard.
  features?: PluginFeature[];
  screenshots?: string[];
  has_docs?: boolean;
  // Set when the catalog revoked it; it stays stopped until removed or kept.
  revoked?: PluginRevocation;
  // Whether it may send notifications: pending until the user answers.
  notifications?: 'none' | 'pending' | 'allowed' | 'denied';
  // Whether it may run on a schedule, the same way, and each schedule with
  // how its last run went.
  schedule?: 'none' | 'pending' | 'allowed' | 'denied';
  schedules?: PluginSchedule[];
}

export interface PluginSchedule {
  name: string;
  every: string;
  last_run?: string;
  last_error?: string;
  next_run?: string;
  running?: boolean;
}

export interface PluginFeature {
  kind: string;
  name: string;
  detail?: string;
}

// The plugin's docs as an HTML fragment lerd rendered from its markdown.
export async function loadPluginDocs(name: string): Promise<string> {
  const body = await apiJson<{ html?: string }>('/api/plugin-manager/' + encodeURIComponent(name) + '/docs');
  return body?.html ?? '';
}

// The plugin's recent failures, newest first.
export async function loadPluginLogs(name: string): Promise<PluginFailure[]> {
  const list = await apiJson<PluginFailure[]>('/api/plugin-manager/' + encodeURIComponent(name) + '/logs');
  return Array.isArray(list) ? list : [];
}

export interface PluginFailure {
  surface: string;
  what: string;
  error: string;
  at: string;
  report_url?: string;
}

// An approved plugin in the catalog, as the Browse view draws it.
export interface CatalogEntry {
  name: string;
  description?: string;
  repo: string;
  commit: string;
  author?: string;
  category?: string;
  tags?: string[];
  images?: string[];
  // The approved commit's own version.
  version?: string;
  lerd?: string;
  requires?: string[];
  platforms?: string[];
  added_at?: string;
  updated_at?: string;
  stars?: number;
  installed: boolean;
  compatible: boolean;
  runs_here?: boolean;
  missing?: string[];
}

// loadCatalog throws with lerd's reason when the catalog cannot be reached.
// fresh asks lerd to fetch the catalog first, as opening Browse does.
export async function loadCatalog(fresh = false): Promise<CatalogEntry[]> {
  const res = await apiFetch('/api/plugin-manager/catalog' + (fresh ? '?fresh=1' : ''));
  if (!res.ok) throw new Error((await res.text()).trim() || res.statusText);
  const list = await res.json();
  return Array.isArray(list) ? list : [];
}

export async function installListedPlugin(name: string): Promise<string> {
  return mutate('/api/plugin-manager', {
    method: 'POST',
    body: JSON.stringify({ name }),
    headers: { 'Content-Type': 'application/json' }
  });
}

export async function updatePlugin(name: string): Promise<string> {
  return mutate('/api/plugin-manager/' + encodeURIComponent(name) + '/update', { method: 'POST' });
}

export const managedPlugins = writable<ManagedPlugin[]>([]);

export async function loadManagedPlugins() {
  const list = await apiJson<ManagedPlugin[]>('/api/plugin-manager');
  managedPlugins.set(Array.isArray(list) ? list : []);
}

// installPlugin and removePlugin resolve to an error message, or '' on success,
// and refresh both lists so the rail and site tabs follow at once.
export async function installPlugin(source: string): Promise<string> {
  return mutate('/api/plugin-manager', {
    method: 'POST',
    body: JSON.stringify({ source }),
    headers: { 'Content-Type': 'application/json' }
  });
}

// keepRevokedPlugin runs a revoked plugin again because the user chose to.
export async function keepRevokedPlugin(name: string): Promise<string> {
  return mutate('/api/plugin-manager/' + encodeURIComponent(name) + '/keep', { method: 'POST' });
}

// setPluginNotifications records the user's answer to a plugin asking to send
// notifications, from the consent prompt or a toggle.
export async function setPluginNotifications(name: string, allowed: boolean): Promise<string> {
  return mutate('/api/plugin-manager/' + encodeURIComponent(name) + '/notifications', {
    method: 'POST',
    body: JSON.stringify({ allowed }),
    headers: { 'Content-Type': 'application/json' }
  });
}

// setPluginSchedule records whether a plugin may run on a schedule, from the
// consent prompt or its card's switch.
export async function setPluginSchedule(name: string, allowed: boolean): Promise<string> {
  return mutate('/api/plugin-manager/' + encodeURIComponent(name) + '/schedule', {
    method: 'POST',
    body: JSON.stringify({ allowed }),
    headers: { 'Content-Type': 'application/json' }
  });
}

export async function removePlugin(name: string): Promise<string> {
  return mutate('/api/plugin-manager/' + encodeURIComponent(name), { method: 'DELETE' });
}

async function mutate(path: string, init: RequestInit): Promise<string> {
  const res = await apiFetch(path, init);
  const error = res.ok ? '' : (await res.text()).trim() || res.statusText;
  await Promise.all([loadManagedPlugins(), loadPlugins()]);
  return error;
}

// The GitHub account that owns a plugin's repository, which the author's name
// links to: the author field is free text, the owner is always a real account.
export function authorProfile(repo?: string): string | null {
  const m = /^https:\/\/github\.com\/([^/]+)\//.exec((repo ?? '') + '/');
  return m ? `https://github.com/${m[1]}` : null;
}
