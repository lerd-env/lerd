import { render, fireEvent, waitFor } from '@testing-library/svelte';
import { it, expect, vi } from 'vitest';
import PagePluginsMenu from './PagePluginsMenu.svelte';

function serve(list: object[]) {
  const posts: unknown[] = [];
  globalThis.fetch = vi.fn(async (_url: string, init?: RequestInit) => {
    if (init?.method === 'POST') {
      const body = JSON.parse(init.body as string);
      posts.push(body);
      list = list.map((p) => {
        const q = p as { name: string; kind?: string };
        return q.name === body.plugin && (q.kind ?? 'page_script') === body.kind ? { ...p, on: body.on } : p;
      });
    }
    return new Response(JSON.stringify(list));
  }) as unknown as typeof fetch;
  return posts;
}

// The header keeps its icons for what the site uses; with no page plugin
// installed this menu has nothing to offer and must not take a slot.
it('draws nothing when no installed plugin has a page script', async () => {
  serve([]);
  const { container } = render(PagePluginsMenu, { props: { site: 'shop' } });
  await Promise.resolve();
  expect(container.querySelector('button')).toBeNull();
});

it('switches a plugin on for the site', async () => {
  const posts = serve([{ name: 'debug', description: 'Debug bar', on: false }]);
  const { findByLabelText } = render(PagePluginsMenu, { props: { site: 'shop' } });
  await fireEvent.click(await findByLabelText('Plugins on this site'));
  await fireEvent.click(await findByLabelText('debug: Pages'));
  await waitFor(() => expect(posts).toEqual([{ site: 'shop', plugin: 'debug', kind: 'page_script', on: true }]));
});

// One plugin can bring several: its name shows once, its description only on
// hover, with a checkbox per kind under a column only the kinds some plugin has get.
it('draws a plugin once with a checkbox per kind it brings, each switched on its own', async () => {
  const posts = serve([
    { name: 'stats', description: 'Request numbers', kind: 'page_script', on: true },
    { name: 'stats', description: 'Request numbers', kind: 'overview_card', on: false },
    { name: 'agents', description: 'Coding agents', kind: 'site_tab', on: true }
  ]);
  const { findByLabelText, findByText, getAllByText, queryAllByText, getByLabelText, queryByLabelText, getAllByRole } = render(PagePluginsMenu, { props: { site: 'shop2' } });
  const trigger = await findByLabelText('Plugins on this site');
  // A tab is on until hidden, so it does not light the icon the way an opt-in does.
  expect(trigger.className).toContain('emerald');
  await fireEvent.click(trigger);
  const name = await findByText('stats');
  expect(getAllByText('stats')).toHaveLength(1);
  expect(queryAllByText('Request numbers').every((el) => el.classList.contains('sr-only'))).toBe(true);
  await fireEvent.mouseEnter(name);
  expect(document.querySelector('[role="tooltip"]')?.textContent).toContain('Request numbers');
  expect(getAllByRole('columnheader').map((h) => h.textContent?.trim())).toEqual(['', 'Pages', 'Overview', 'Tab']);
  expect((getByLabelText('stats: Pages') as HTMLInputElement).checked).toBe(true);
  expect(queryByLabelText('stats: Tab')).toBeNull();
  expect(queryByLabelText('agents: Pages')).toBeNull();

  await fireEvent.click(getByLabelText('stats: Overview'));
  await waitFor(() => expect(posts).toEqual([{ site: 'shop2', plugin: 'stats', kind: 'overview_card', on: true }]));
  await waitFor(() => expect((getByLabelText('stats: Overview') as HTMLInputElement).checked).toBe(true));
  await fireEvent.click(getByLabelText('agents: Tab'));
  await waitFor(() => expect(posts[1]).toEqual({ site: 'shop2', plugin: 'agents', kind: 'site_tab', on: false }));
});

it('leaves the icon unlit when only tabs are on', async () => {
  serve([{ name: 'agents', description: 'Coding agents', kind: 'site_tab', on: true }]);
  const { findByLabelText } = render(PagePluginsMenu, { props: { site: 'shop5' } });
  expect((await findByLabelText('Plugins on this site')).className).not.toContain('emerald');
});

// Another dashboard may have switched something since the menu last looked.
it('reads the switches again each time it opens', async () => {
  serve([{ name: 'debug', description: 'Debug bar', on: false }]);
  const { findByLabelText } = render(PagePluginsMenu, { props: { site: 'shop3' } });
  const trigger = await findByLabelText('Plugins on this site');
  const before = (globalThis.fetch as unknown as { mock: { calls: unknown[] } }).mock.calls.length;
  await fireEvent.click(trigger);
  await waitFor(() => expect((globalThis.fetch as unknown as { mock: { calls: unknown[] } }).mock.calls.length).toBe(before + 1));
});

// A read that left before a switch must not land after it with the old state.
it('keeps a switch made while the open-time read is still on its way', async () => {
  let release: (r: Response) => void = () => {};
  const posts: unknown[] = [];
  let gets = 0;
  globalThis.fetch = vi.fn(async (_url: string, init?: RequestInit) => {
    if (init?.method === 'POST') {
      posts.push(JSON.parse(init.body as string));
      return new Response(JSON.stringify([{ name: 'debug', kind: 'page_script', on: true }]));
    }
    gets++;
    if (gets === 1) return new Response(JSON.stringify([{ name: 'debug', kind: 'page_script', on: false }]));
    return new Promise<Response>((r) => (release = r));
  }) as unknown as typeof fetch;
  const { findByLabelText, getByLabelText } = render(PagePluginsMenu, { props: { site: 'shop4' } });
  await fireEvent.click(await findByLabelText('Plugins on this site'));
  const box = () => getByLabelText('debug: Pages') as HTMLInputElement;
  await fireEvent.click(box());
  await waitFor(() => expect(box().checked).toBe(true));
  release(new Response(JSON.stringify([{ name: 'debug', kind: 'page_script', on: false }])));
  await new Promise((r) => setTimeout(r, 20));
  expect(box().checked).toBe(true);
});

// lerd refusing a switch leaves the box as it was, not as it was clicked.
it('puts the checkbox back when lerd refuses the switch', async () => {
  globalThis.fetch = vi.fn(async (_url: string, init?: RequestInit) =>
    init?.method === 'POST'
      ? new Response('plugin agents has no site tab', { status: 400 })
      : new Response(JSON.stringify([{ name: 'agents', kind: 'site_tab', on: true }]))
  ) as unknown as typeof fetch;
  const { findByLabelText, getByLabelText } = render(PagePluginsMenu, { props: { site: 'shop6' } });
  await fireEvent.click(await findByLabelText('Plugins on this site'));
  const box = getByLabelText('agents: Tab') as HTMLInputElement;
  await fireEvent.click(box);
  await waitFor(() => expect(box.disabled).toBe(false));
  expect(box.checked).toBe(true);
});

// The description lives in a tooltip, so a keyboard user must reach it too:
// it shows when a checkbox gets keyboard focus, and screen readers read it.
it('gives each checkbox the plugin description for keyboard and screen reader users', async () => {
  serve([
    { name: 'stats', description: 'Request numbers', kind: 'page_script', on: false },
    { name: 'stats', description: 'Request numbers', kind: 'overview_card', on: false }
  ]);
  const { findByLabelText, getByLabelText } = render(PagePluginsMenu, { props: { site: 'shop7' } });
  await fireEvent.click(await findByLabelText('Plugins on this site'));
  const said = (el: HTMLElement) => document.getElementById(el.getAttribute('aria-describedby') ?? '')?.textContent;
  const card = getByLabelText('stats: Overview');
  expect(said(card)).toBe('Request numbers');
  expect(said(getByLabelText('stats: Pages'))).toBe("Request numbers. The plugin's script runs in the site's own pages.");
  card.matches = ((s: string): boolean => s === ':focus-visible') as typeof card.matches;
  await fireEvent.focusIn(card);
  expect(document.querySelector('[role="tooltip"]')?.textContent).toContain('Request numbers');
});
