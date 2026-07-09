import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest';
import { get } from 'svelte/store';
import {
  UNGROUPED,
  assignSiteWorkspace,
  createWorkspace,
  deleteWorkspace,
  renameWorkspace,
  saveWorkspaceLayout,
  toggleWorkspaceCollapse,
  workspaceCollapse
} from './workspaces';

function mockFetch(body: unknown = { ok: true }) {
  const fn = vi.fn().mockResolvedValue({ ok: true, json: async () => body } as Response);
  vi.stubGlobal('fetch', fn);
  return fn;
}

function lastCall(fn: ReturnType<typeof mockFetch>) {
  const [url, init] = fn.mock.calls.at(-1) as [string, RequestInit];
  return { url, method: init.method, body: JSON.parse(String(init.body)) };
}

describe('workspace API helpers', () => {
  afterEach(() => vi.unstubAllGlobals());

  it('creates a workspace', async () => {
    const fetchMock = mockFetch();
    await expect(createWorkspace('Client Work')).resolves.toEqual({ ok: true, error: undefined });
    expect(lastCall(fetchMock)).toEqual({
      url: '/api/workspaces',
      method: 'POST',
      body: { name: 'Client Work' }
    });
  });

  it('renames a workspace', async () => {
    const fetchMock = mockFetch();
    await renameWorkspace('A', 'B');
    expect(lastCall(fetchMock)).toEqual({
      url: '/api/workspaces/rename',
      method: 'POST',
      body: { old: 'A', new: 'B' }
    });
  });

  it('deletes a workspace', async () => {
    const fetchMock = mockFetch();
    await deleteWorkspace('A');
    expect(lastCall(fetchMock)).toEqual({ url: '/api/workspaces/delete', method: 'POST', body: { name: 'A' } });
  });

  it('assigns sites, defaulting create to false', async () => {
    const fetchMock = mockFetch();
    await assignSiteWorkspace(['blog'], 'A');
    expect(lastCall(fetchMock).body).toEqual({ sites: ['blog'], workspace: 'A', create: false });
  });

  it('ungroups a site with an empty workspace', async () => {
    const fetchMock = mockFetch();
    await assignSiteWorkspace(['blog'], '');
    expect(lastCall(fetchMock).body).toEqual({ sites: ['blog'], workspace: '', create: false });
  });

  it('sends the layout as a PUT, with an empty site order when omitted', async () => {
    const fetchMock = mockFetch();
    await saveWorkspaceLayout([{ name: 'A', sites: ['one'] }]);
    expect(lastCall(fetchMock)).toEqual({
      url: '/api/workspaces/layout',
      method: 'PUT',
      body: { workspaces: [{ name: 'A', sites: ['one'] }], site_order: [] }
    });
  });

  it('sends the site order when the manual order changed', async () => {
    const fetchMock = mockFetch();
    await saveWorkspaceLayout([{ name: 'A', sites: ['one'] }], ['one', 'two']);
    expect(lastCall(fetchMock).body.site_order).toEqual(['one', 'two']);
  });

  it('reports a server error instead of throwing', async () => {
    mockFetch({ ok: false, error: 'workspace already exists' });
    await expect(createWorkspace('A')).resolves.toEqual({ ok: false, error: 'workspace already exists' });
  });

  it('reports a network failure instead of throwing', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('offline')));
    await expect(createWorkspace('A')).resolves.toEqual({ ok: false, error: 'offline' });
  });
});

describe('workspaceCollapse', () => {
  beforeEach(() => {
    localStorage.clear();
    workspaceCollapse.set([]);
  });

  it('toggles a key on and off', () => {
    toggleWorkspaceCollapse('A');
    expect(get(workspaceCollapse)).toEqual(['A']);
    toggleWorkspaceCollapse('A');
    expect(get(workspaceCollapse)).toEqual([]);
  });

  it('persists to localStorage', () => {
    toggleWorkspaceCollapse('A');
    expect(JSON.parse(localStorage.getItem('lerd:workspaceCollapse') ?? '[]')).toEqual(['A']);
  });

  it('keeps the ungrouped section independent of a workspace named the same', () => {
    toggleWorkspaceCollapse(UNGROUPED);
    expect(get(workspaceCollapse)).toEqual([UNGROUPED]);
    expect(UNGROUPED.trim()).toBe('ungrouped');
    expect(UNGROUPED).not.toBe('ungrouped');
  });
});
