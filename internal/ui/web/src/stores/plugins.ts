import { writable, derived } from 'svelte/store';
import { apiFetch, apiJson } from '$lib/api';

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
}

export const plugins = writable<PluginInfo[]>([]);

// Plugins that draw a page on the rail, and those that add a tab to every site.
export const railPlugins = derived(plugins, ($p) => $p.filter((p) => p.page));
export const siteTabPlugins = derived(plugins, ($p) => $p.filter((p) => p.site_tab));

export async function loadPlugins() {
  // A lerd-ui from before plugins answers this path with something else.
  try {
    const list = await apiJson<PluginInfo[]>('/api/plugins');
    plugins.set(Array.isArray(list) ? list : []);
  } catch {
    plugins.set([]);
  }
}

// A plugin whose script a site's pages can carry, and whether they do.
export interface PagePlugin {
  name: string;
  description?: string;
  on: boolean;
}

export async function loadPagePlugins(site: string): Promise<PagePlugin[]> {
  const list = await apiJson<PagePlugin[]>('/api/page-plugins?site=' + encodeURIComponent(site));
  return Array.isArray(list) ? list : [];
}

// setPagePlugin resolves to the site's page plugins after the switch, or
// throws with lerd's reason.
export async function setPagePlugin(site: string, plugin: string, on: boolean): Promise<PagePlugin[]> {
  const res = await apiFetch('/api/page-plugins', {
    method: 'POST',
    body: JSON.stringify({ site, plugin, on }),
    headers: { 'Content-Type': 'application/json' }
  });
  if (!res.ok) throw new Error((await res.text()).trim() || res.statusText);
  return res.json();
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
  // The catalog commit it was installed at; absent when not listed.
  commit?: string;
  // A newer approved commit, when there is one.
  update?: string;
  // Its latest failure, with a filled-in issue on its repository when known.
  failure?: PluginFailure;
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
  lerd?: string;
  requires?: string[];
  added_at?: string;
  updated_at?: string;
  stars?: number;
  installed: boolean;
  compatible: boolean;
  missing?: string[];
}

// loadCatalog throws with lerd's reason when the catalog cannot be reached.
export async function loadCatalog(): Promise<CatalogEntry[]> {
  const res = await apiFetch('/api/plugin-manager/catalog');
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

export async function removePlugin(name: string): Promise<string> {
  return mutate('/api/plugin-manager/' + encodeURIComponent(name), { method: 'DELETE' });
}

async function mutate(path: string, init: RequestInit): Promise<string> {
  const res = await apiFetch(path, init);
  const error = res.ok ? '' : (await res.text()).trim() || res.statusText;
  await Promise.all([loadManagedPlugins(), loadPlugins()]);
  return error;
}
