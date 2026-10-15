import { render, fireEvent, waitFor, within } from '@testing-library/svelte';
import { it, expect, beforeEach, vi } from 'vitest';
import PluginsDetail from './PluginsDetail.svelte';

let installed: object[];
const calls: { method: string; url: string; body?: string }[] = [];

beforeEach(() => {
  installed = [
    { name: 'broken', error: 'api_version 9 needs a newer lerd' },
    { name: 'hello', version: '0.1.0', description: 'Says hello', adds: ['commands', 'page'], requires: ['python3', 'jq'] }
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

it('lists plugins, with what each adds and why a broken one failed', async () => {
  const { findByText, getByText } = render(PluginsDetail);
  await findByText('hello');
  expect(getByText('Says hello')).toBeTruthy();
  expect(getByText('page')).toBeTruthy();
  expect(getByText(/api_version 9 needs a newer lerd/)).toBeTruthy();
  expect(getByText('python3')).toBeTruthy();
  expect(getByText('jq')).toBeTruthy();
});

// The author can only fix what they hear about, so a failure links straight to
// a filled-in issue on the plugin's repository.
it('shows a plugin\'s last failure with a link to report it', async () => {
  installed = [
    {
      name: 'hello',
      failure: { plugin: 'hello', surface: 'hook', what: 'site.linked', error: 'boom\nmore', at: '2026-10-15T10:00:00Z', report_url: 'https://github.com/acme/hello/issues/new?title=x' }
    }
  ];
  const { findByText, getByRole } = render(PluginsDetail);
  await findByText(/boom/);
  expect(getByRole('link', { name: 'Report to the author' }).getAttribute('href')).toBe('https://github.com/acme/hello/issues/new?title=x');
});

// A broken plugin is exactly the one a user most needs to get rid of.
it('removes a plugin after confirming, broken ones included', async () => {
  const { findByText, getAllByRole, getByRole } = render(PluginsDetail);
  await findByText('broken');
  await fireEvent.click(getAllByRole('button', { name: 'Remove' })[0]);
  await fireEvent.click(within(getByRole('dialog')).getByRole('button', { name: 'Remove' }));
  await waitFor(() => expect(calls.some((c) => c.method === 'DELETE' && c.url.endsWith('/api/plugin-manager/broken'))).toBe(true));
});

it('installs from what was typed and keeps the field when the install fails', async () => {
  const { getByLabelText, getByRole } = render(PluginsDetail);
  const input = getByLabelText('Folder path or git URL') as HTMLInputElement;
  await fireEvent.input(input, { target: { value: ' https://example.test/plugin.git ' } });
  await fireEvent.click(getByRole('button', { name: 'Install' }));
  await waitFor(() => expect(calls.some((c) => c.method === 'POST')).toBe(true));
  const post = calls.find((c) => c.method === 'POST')!;
  expect(JSON.parse(post.body!)).toEqual({ source: 'https://example.test/plugin.git' });
  expect(input.value.trim()).toBe('https://example.test/plugin.git');
});
