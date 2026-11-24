import { render } from '@testing-library/svelte';
import { it, expect, vi } from 'vitest';
import SiteOverview from './SiteOverview.svelte';
import type { Site } from '$stores/sites';

globalThis.fetch = vi.fn(async () => new Response('{}')) as unknown as typeof fetch;

// jsdom lays nothing out, so this pins the rule itself: the services grid takes
// its columns from the Overview pane (container queries), never from the window.
it('sizes the services grid by the pane, not the window', () => {
  const site = { name: 'shop', domain: 'shop.test', path: '/srv/shop', services: ['redis'] } as unknown as Site;
  const { getByText } = render(SiteOverview, { props: { site } });

  const grid = getByText('Services').parentElement!.nextElementSibling as HTMLElement;
  const pane = grid.closest('.\\@container');
  expect(pane).not.toBeNull();
  const cols = [...grid.classList].filter((c) => c.includes('grid-cols-'));
  expect(cols).toEqual(['grid-cols-2', '@3xl:grid-cols-3', '@5xl:grid-cols-4']);
});
