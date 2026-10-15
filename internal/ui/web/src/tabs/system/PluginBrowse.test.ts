import { render, fireEvent, waitFor, within } from '@testing-library/svelte';
import { it, expect, vi, beforeEach } from 'vitest';
import PluginBrowse from './PluginBrowse.svelte';

const A = 'a'.repeat(40);
let catalog: object[];
let posts: unknown[];

beforeEach(() => {
  catalog = [
    { name: 'old', repo: 'r', commit: A, stars: 50, updated_at: '2026-01-01', installed: false, compatible: true },
    { name: 'fresh', repo: 'r', commit: 'b'.repeat(40), stars: 2, updated_at: '2026-10-01', installed: false, images: ['https://plugins.lerd.sh/images/fresh/1.png'], compatible: true },
    { name: 'mine', repo: 'r', commit: A, installed: true, compatible: true }
  ];
  posts = [];
  globalThis.fetch = vi.fn(async (url: string, init?: RequestInit) => {
    if (init?.method === 'POST') {
      posts.push(JSON.parse(init.body as string));
      return new Response('{}');
    }
    if (String(url).endsWith('/catalog')) return new Response(JSON.stringify(catalog));
    return new Response('[]');
  }) as unknown as typeof fetch;
});

const names = (c: HTMLElement) => [...c.querySelectorAll('article')].map((a) => a.id.replace('plugin-card-', ''));

it('lists the catalog newest first, with its preview, and marks what is installed', async () => {
  const { container, findByText, getByText } = render(PluginBrowse);
  await findByText('fresh');
  expect(names(container)).toEqual(['fresh', 'old', 'mine']);
  expect(container.querySelector('#plugin-card-fresh img')?.getAttribute('src')).toBe('https://plugins.lerd.sh/images/fresh/1.png');
  expect(within(container.querySelector('#plugin-card-mine') as HTMLElement).getByText('Installed')).toBeTruthy();
  expect(getByText('★ 50')).toBeTruthy();
});

// The commit is in the question because it is the one thing the user is
// trusting: lerd installs that and nothing newer.
it('installs by name after confirming the approved commit', async () => {
  const { findByText, getAllByRole, getByRole } = render(PluginBrowse);
  await findByText('old');
  await fireEvent.click(getAllByRole('button', { name: 'Install' })[1]);
  const dialog = getByRole('dialog');
  expect(dialog.textContent).toContain('aaaaaaa');
  await fireEvent.click(within(dialog).getByRole('button', { name: 'Install' }));
  await waitFor(() => expect(posts).toEqual([{ name: 'old' }]));
});

// Offering the install would only end in a refusal from the binary.
it('names the lerd a plugin needs instead of offering to install it', async () => {
  catalog = [{ name: 'later', repo: 'r', commit: A, installed: false, compatible: false, lerd: '>=1.40' }];
  const { findByText, queryByRole } = render(PluginBrowse);
  await findByText('Needs Lerd >=1.40');
  expect(queryByRole('button', { name: 'Install' })).toBeNull();
});

// Installing would only end in lerd refusing it, so say what is missing first.
it('lists what a plugin requires and offers no install while something is missing', async () => {
  catalog = [{ name: 'needy', repo: 'r', commit: A, installed: false, compatible: true, requires: ['python3', 'jq'], missing: ['jq'] }];
  const { findByText, getByText, queryByRole } = render(PluginBrowse);
  await findByText('needy');
  expect(getByText('python3')).toBeTruthy();
  expect(getByText('jq').className).toContain('text-red');
  expect(queryByRole('button', { name: 'Install' })).toBeNull();
});

it('says why when the catalog cannot be reached', async () => {
  globalThis.fetch = vi.fn(async () => new Response('fetching the plugin catalog: no route', { status: 502 })) as unknown as typeof fetch;
  const { findByText } = render(PluginBrowse);
  await findByText('The plugin catalog could not be loaded');
  await findByText(/no route/);
});
