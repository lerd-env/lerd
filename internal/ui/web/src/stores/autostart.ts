import { writable } from 'svelte/store';
import { apiJson, apiFetch } from '$lib/api';

export const autostartEnabled = writable<boolean>(false);
export const startOnDashboardOpen = writable<boolean>(false);
export const trayEnabled = writable<boolean>(true);
export const betaUpdates = writable<boolean>(false);
// Whether lerd's MCP server is registered with the AI assistants on this machine.
export const mcpGlobal = writable<boolean>(false);

interface SettingsResponse {
  autostart_on_login?: boolean;
  start_on_dashboard_open?: boolean;
  tray_enabled?: boolean;
  beta_updates?: boolean;
  mcp_global?: boolean;
}

export async function loadAutostart() {
  try {
    const res = await apiJson<SettingsResponse>('/api/settings');
    autostartEnabled.set(Boolean(res.autostart_on_login));
    startOnDashboardOpen.set(Boolean(res.start_on_dashboard_open));
    trayEnabled.set(res.tray_enabled !== false);
    betaUpdates.set(Boolean(res.beta_updates));
    mcpGlobal.set(Boolean(res.mcp_global));
  } catch {
    /* keep previous */
  }
}

export async function toggleAutostart(enable: boolean): Promise<boolean> {
  try {
    const res = await apiFetch('/api/settings/autostart', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ enabled: enable })
    });
    if (res.ok) autostartEnabled.set(enable);
    return res.ok;
  } catch {
    return false;
  }
}

export async function toggleStartOnDashboardOpen(enable: boolean): Promise<boolean> {
  try {
    const res = await apiFetch('/api/settings/start-on-open', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ enabled: enable })
    });
    if (res.ok) startOnDashboardOpen.set(enable);
    return res.ok;
  } catch {
    return false;
  }
}

export async function toggleTray(enable: boolean): Promise<boolean> {
  try {
    const res = await apiFetch('/api/settings/tray', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ enabled: enable })
    });
    if (res.ok) trayEnabled.set(enable);
    return res.ok;
  } catch {
    return false;
  }
}

export async function toggleBetaUpdates(enable: boolean): Promise<boolean> {
  try {
    const res = await apiFetch('/api/settings/beta-updates', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ enabled: enable })
    });
    if (res.ok) betaUpdates.set(enable);
    return res.ok;
  } catch {
    return false;
  }
}

// Registration writes into each assistant's own config and can fail there, so
// the answer's own ok decides, not just the request getting through.
export async function toggleMCP(enable: boolean): Promise<boolean> {
  try {
    const res = await apiFetch('/api/settings/mcp', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ enabled: enable })
    });
    const body = res.ok ? await res.json() : null;
    if (body?.ok) mcpGlobal.set(enable);
    return Boolean(body?.ok);
  } catch {
    return false;
  }
}
