import { render, fireEvent, waitFor, within } from '@testing-library/svelte';
import { it, expect, beforeEach, vi } from 'vitest';
import PluginsDetail from './PluginsDetail.svelte';
import * as notify from '$lib/notify';

let installed: object[];
const calls: { method: string; url: string; body?: string }[] = [];

beforeEach(() => {
  location.hash = '#plugins';
  installed = [
    { name: 'broken', error: 'api_version 9 needs a newer lerd' },
    { name: 'hello', version: '0.1.0', description: 'Says hello', adds: ['commands', 'page'], requires: ['python3', 'jq'], icon: '/plugins/hello/ui/icon.svg' }
  ];
  calls.length = 0;
  globalThis.fetch = vi.fn(async (url: string, init?: RequestInit) => {
    const method = init?.method ?? 'GET';
    calls.push({ method, url: String(url), body: init?.body as string | undefined });
    if (method === 'DELETE') installed = installed.filter((p) => !String(url).endsWith('/' + (p as { name: string }).name));
    if (method === 'POST') return new Response('not a lerd plugin', { status: 400 });
    if (String(url).endsWith('/api/plugin-manager')) return new Response(JSON.stringify(installed));
    return new Response('[]');
  }) as unknown as typeof fetch;
});

// A card carries the name, the version and whether it loaded; what it adds and
// requires waits in its detail.
it('lists plugins as cards and opens one to show what it adds and requires', async () => {
  const { findByText, getByText, queryByText, getByRole } = render(PluginsDetail);
  await findByText('hello');
  expect(getByText('Says hello')).toBeTruthy();
  expect(getByText('Failed to load')).toBeTruthy();
  expect(queryByText('python3')).toBeNull();

  await fireEvent.click(getByRole('button', { name: /^hello/ }));
  const dialog = within(getByRole('dialog'));
  await fireEvent.click(dialog.getByRole('tab', { name: 'Features' }));
  expect(dialog.getByText('page')).toBeTruthy();
  await fireEvent.click(dialog.getByRole('tab', { name: 'Overview' }));
  expect(dialog.getByText('python3')).toBeTruthy();
  expect(dialog.getByText('jq')).toBeTruthy();
  expect(dialog.getByText('Not listed in the catalog')).toBeTruthy();
});

it('shows why a broken plugin failed in its detail', async () => {
  const { findByText, getByRole } = render(PluginsDetail);
  await fireEvent.click(await findByText('broken'));
  expect(await within(getByRole('dialog')).findByText(/api_version 9 needs a newer lerd/)).toBeTruthy();
});

// The author can only fix what they hear about, so the Logs tab lists the
// plugin's failures in full, each with a link to a filled-in issue.
it('lists a plugin\'s failures on its Logs tab, each with a link to report it', async () => {
  installed = [{ name: 'hello', failure: { plugin: 'hello', surface: 'hook', what: 'site.linked', error: 'boom', at: '2026-10-15T10:00:00Z' } }];
  const logs = [
    { plugin: 'hello', surface: 'hook', what: 'site.linked', error: 'boom\nmore', at: '2026-10-15T10:00:00Z', report_url: 'https://github.com/acme/hello/issues/new?title=x' },
    { plugin: 'hello', surface: 'api', what: 'greet', error: 'older', at: '2026-10-14T10:00:00Z' }
  ];
  const plain = globalThis.fetch;
  globalThis.fetch = vi.fn(async (url: string, init?: RequestInit) =>
    String(url).endsWith('/api/plugin-manager/hello/logs') ? new Response(JSON.stringify(logs)) : plain(url, init)
  ) as unknown as typeof fetch;
  const { findByText, getByRole } = render(PluginsDetail);
  await waitFor(() => expect(calls.some((c) => c.url.endsWith('/api/plugin-manager'))).toBe(true));
  await fireEvent.click(await findByText('hello'));
  const dialog = within(getByRole('dialog'));
  expect(dialog.queryByText(/boom/)).toBeNull();
  await fireEvent.click(dialog.getByRole('tab', { name: 'Logs' }));
  expect(await dialog.findByText(/boom\s+more/)).toBeTruthy();
  expect(dialog.getByText('older')).toBeTruthy();
  expect(dialog.getAllByRole('link', { name: 'Report to the author' })).toHaveLength(1);
});

// Docs and screenshots come from the installed plugin, so they read offline.
it('shows an installed plugin\'s features, screenshots and docs', async () => {
  installed = [{
    name: 'hello', has_docs: true, screenshots: ['/plugins/hello/docs/tab.png'],
    features: [{ kind: 'command', name: 'lerd hello:greet', detail: 'Print a greeting' }]
  }];
  const plain = globalThis.fetch;
  globalThis.fetch = vi.fn(async (url: string, init?: RequestInit) =>
    String(url).endsWith('/api/plugin-manager/hello/docs')
      ? new Response(JSON.stringify({ html: '<h1>Hello docs</h1><img src="x" onerror="alert(1)">' }))
      : plain(url, init)
  ) as unknown as typeof fetch;
  const { findByText, getByRole } = render(PluginsDetail);
  await waitFor(() => expect(calls.some((c) => c.url.endsWith('/api/plugin-manager'))).toBe(true));
  await fireEvent.click(await findByText('hello'));
  const dialog = within(getByRole('dialog'));
  expect(getByRole('dialog').querySelector('img[src="/plugins/hello/docs/tab.png"]')).toBeTruthy();
  expect(dialog.queryByText('lerd hello:greet')).toBeNull();
  await fireEvent.click(dialog.getByRole('tab', { name: 'Features' }));
  expect(dialog.getByText('lerd hello:greet')).toBeTruthy();
  expect(dialog.getByText('Print a greeting')).toBeTruthy();
  await fireEvent.click(dialog.getByRole('tab', { name: 'Documentation' }));
  expect(await dialog.findByText('Hello docs')).toBeTruthy();
  expect(getByRole('dialog').querySelector('[onerror]')).toBeNull();
});

// A broken plugin is exactly the one a user most needs to get rid of.
it('removes a plugin from its detail after confirming, broken ones included', async () => {
  const { findByText, getAllByRole } = render(PluginsDetail);
  await fireEvent.click(await findByText('broken'));
  await fireEvent.click(within(getAllByRole('dialog')[0]).getByRole('button', { name: 'Remove' }));
  const confirm = getAllByRole('dialog').at(-1)!;
  await fireEvent.click(within(confirm).getByRole('button', { name: 'Remove' }));
  await waitFor(() => expect(calls.some((c) => c.method === 'DELETE' && c.url.endsWith('/api/plugin-manager/broken'))).toBe(true));
});

// Installing from a folder or git sits in the Installed header, not behind the tile.
it('installs from a folder or git in a modal and keeps the field when the install fails', async () => {
  const { findByText, getByRole, getByLabelText } = render(PluginsDetail);
  await findByText('hello');
  await fireEvent.click(getByRole('button', { name: 'Install from a folder or git' }));
  const input = getByLabelText('Folder path or git URL') as HTMLInputElement;
  await fireEvent.input(input, { target: { value: ' https://example.test/plugin.git ' } });
  await fireEvent.click(within(getByRole('dialog')).getByRole('button', { name: 'Install' }));
  await waitFor(() => expect(calls.some((c) => c.method === 'POST')).toBe(true));
  const post = calls.find((c) => c.method === 'POST')!;
  expect(JSON.parse(post.body!)).toEqual({ source: 'https://example.test/plugin.git' });
  expect(input.value.trim()).toBe('https://example.test/plugin.git');
});

// The two views are icon buttons at the header's far end, the current one
// marked, so the header carries only the title and its actions.
it('switches between installed and browse with icon buttons in the header', async () => {
  const { getByRole } = render(PluginsDetail);
  const installedBtn = getByRole('button', { name: 'Installed' });
  const browseBtn = getByRole('button', { name: 'Browse' });
  expect(installedBtn.textContent?.trim()).toBe('');
  expect(installedBtn.getAttribute('aria-current')).toBe('page');
  expect(browseBtn.getAttribute('aria-current')).toBeNull();
  await fireEvent.click(browseBtn);
  expect(location.hash).toBe('#plugins/browse');
});

// With nothing installed the page says what plugins add and offers both ways
// to get one, rather than a bare "none".
it('shows what plugins add and both ways to get one when none is installed', async () => {
  installed = [];
  const { findByText, getByText, getByRole } = render(PluginsDetail);
  await findByText('Make Lerd do more');
  expect(getByText('Overview cards')).toBeTruthy();
  expect(getByText('lerd plugin new my-plugin')).toBeTruthy();
  expect(getByRole('button', { name: 'Install from a folder or git' })).toBeTruthy();
  await fireEvent.click(getByRole('button', { name: 'Browse plugins' }));
  expect(location.hash).toBe('#plugins/browse');
});

// With plugins installed one more tile ends the grid and leads to the catalog,
// where a user expects to find more, rather than a dialog in between.
it('takes the add tile straight to the catalog', async () => {
  const { findByText, getByRole, queryByRole } = render(PluginsDetail);
  await findByText('hello');
  await fireEvent.click(getByRole('button', { name: 'Add a plugin' }));
  expect(location.hash).toBe('#plugins/browse');
  expect(queryByRole('dialog')).toBeNull();
});

// The author links to the GitHub account that owns the plugin's repository.
it("links an installed catalog plugin's author to their GitHub profile", async () => {
  installed = [{ name: 'hello', author: 'Acme Inc', repo: 'https://github.com/acme/hello', commit: 'a'.repeat(40) }];
  const { findByText, getByRole } = render(PluginsDetail);
  await waitFor(() => expect(calls.some((c) => c.url.endsWith('/api/plugin-manager'))).toBe(true));
  await fireEvent.click(await findByText('hello'));
  const link = within(getByRole('dialog')).getByRole('link', { name: 'Acme Inc' });
  expect(link.getAttribute('href')).toBe('https://github.com/acme');
});

// A newer approved commit shows on the tile and in the card with what changed,
// and Update all updates every plugin that has one.
it('shows an update with its version and changes, and updates all', async () => {
  const A = 'a'.repeat(40), B = 'b'.repeat(40);
  installed = [
    { name: 'hello', version: '0.1.0', repo: 'https://github.com/acme/hello', commit: A, update: B, update_version: '0.2.0', changes: `https://github.com/acme/hello/compare/${A}...${B}` },
    { name: 'other', version: '1.0.0', repo: 'https://github.com/acme/other', commit: A, update: B }
  ];
  const { findByText, getByRole, getByText } = render(PluginsDetail);
  await waitFor(() => expect(calls.some((c) => c.url.endsWith('/api/plugin-manager'))).toBe(true));
  expect(await findByText('↑ 0.2.0')).toBeTruthy();
  await fireEvent.click(getByText('hello'));
  const dialog = within(getByRole('dialog'));
  expect(dialog.getByText(/0\.1\.0 → 0\.2\.0/)).toBeTruthy();
  expect(dialog.getByRole('link', { name: 'What changed' }).getAttribute('href')).toBe(`https://github.com/acme/hello/compare/${A}...${B}`);
  await fireEvent.click(dialog.getByRole('button', { name: 'Close' }));
  await fireEvent.click(getByRole('button', { name: 'Update all (2)' }));
  await waitFor(() => {
    const updates = calls.filter((c) => c.method === 'POST' && c.url.endsWith('/update')).map((c) => c.url.split('/').at(-2));
    expect(updates).toEqual(['hello', 'other']);
  });
});

// A plugin that asks to send notifications is asked about once, however it was
// installed, and the answer goes to lerd rather than staying in the page.
it('asks whether a plugin may send notifications and records the answer', async () => {
  installed = [{ name: 'hello', notifications: 'pending' }];
  const plain = globalThis.fetch;
  globalThis.fetch = vi.fn(async (url: string, init?: RequestInit) => {
    if (String(url).endsWith('/api/plugin-manager/hello/notifications')) {
      calls.push({ method: 'POST', url: String(url), body: init?.body as string });
      installed = [{ name: 'hello', notifications: 'allowed' }];
      return new Response('{"ok":true}');
    }
    return plain(url, init);
  }) as unknown as typeof fetch;

  const { findByRole, queryByRole } = render(PluginsDetail);
  const ask = within(await findByRole('dialog'));
  expect(ask.getByText(/hello asks to send you notifications/)).toBeTruthy();
  await fireEvent.click(ask.getByRole('button', { name: 'Allow' }));
  await waitFor(() => expect(queryByRole('dialog')).toBeNull());
  const answer = calls.find((c) => c.url.endsWith('/hello/notifications'));
  expect(JSON.parse(answer?.body ?? '{}')).toEqual({ allowed: true });
});

// One already answered is never asked again, an update included.
it('does not ask about a plugin whose notifications were already answered', async () => {
  installed = [{ name: 'hello', notifications: 'denied' }];
  const { findByText, queryByRole } = render(PluginsDetail);
  await findByText('hello');
  expect(queryByRole('dialog')).toBeNull();
});

// A refused keep leaves the plugin stopped, so the user has to hear why.
it('reports a keep the server refused', async () => {
  installed = [{ name: 'bad', error: 'revoked', revoked: { name: 'bad', reason: 'x', at: '2026-10-24' } }];
  const failure = vi.spyOn(notify, 'notifyLocalFailure');
  const { findByText, getByRole } = render(PluginsDetail);
  await fireEvent.click(await findByText('bad'));
  await fireEvent.click(within(getByRole('dialog')).getByRole('button', { name: 'Keep using it' }));
  await waitFor(() => expect(failure).toHaveBeenCalledWith('plugin_revoked', expect.any(String), expect.stringContaining('not a lerd plugin')));
});
