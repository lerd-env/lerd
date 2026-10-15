import { render, fireEvent, waitFor } from '@testing-library/svelte';
import { it, expect, vi } from 'vitest';
import PagePluginsMenu from './PagePluginsMenu.svelte';

function serve(list: object[]) {
  const posts: unknown[] = [];
  globalThis.fetch = vi.fn(async (_url: string, init?: RequestInit) => {
    if (init?.method === 'POST') {
      const body = JSON.parse(init.body as string);
      posts.push(body);
      list = list.map((p) => ((p as { name: string }).name === body.plugin ? { ...p, on: body.on } : p));
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
  const { findByLabelText, findByText, getByTitle } = render(PagePluginsMenu, { props: { site: 'shop' } });
  await fireEvent.click(await findByLabelText("Plugins on this site's pages"));
  await findByText('Debug bar');
  await fireEvent.click(getByTitle('debug'));
  await waitFor(() => expect(posts).toEqual([{ site: 'shop', plugin: 'debug', on: true }]));
});
