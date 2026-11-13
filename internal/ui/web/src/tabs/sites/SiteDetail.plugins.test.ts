import { render } from '@testing-library/svelte';
import { it, expect, beforeEach, vi } from 'vitest';
import SiteDetail from './SiteDetail.svelte';
import { plugins } from '$stores/plugins';
import type { Site } from '$stores/sites';

beforeEach(() => {
  plugins.set([
    { name: 'agents', site_tab: { title: 'Agents', url: '/plugins/agents/ui/tab.html' } },
    { name: 'sync', site_tab: { title: 'Sync', url: '/plugins/sync/ui/tab.html' } }
  ]);
  globalThis.fetch = vi.fn(async () => new Response('{}')) as unknown as typeof fetch;
});

const site = (extra: Partial<Site> = {}) =>
  ({ name: 'shop', domain: 'shop.test', path: '/srv/shop', services: [], ...extra }) as unknown as Site;

// A plugin's tab is on every site until hidden for one; the setting comes with
// the site, so hiding it in any dashboard reaches one already open.
it('leaves out the tabs the site has hidden, and follows a change made elsewhere', async () => {
  const { findByRole, queryByRole, rerender } = render(SiteDetail, { props: { site: site({ hidden_tabs: ['agents'] }) } });
  await findByRole('tab', { name: 'Sync' });
  expect(queryByRole('tab', { name: 'Agents' })).toBeNull();

  await rerender({ site: site() });
  await findByRole('tab', { name: 'Agents' });
});

// Hiding the open plugin tab falls back to Overview, and the tab row must say
// so, or no tab is selected and the row cannot be reached by keyboard.
it('selects Overview in the tab row once the open plugin tab is hidden', async () => {
  localStorage.setItem('lerd:siteDetailTab', 'plugin:agents');
  const { findByRole, rerender } = render(SiteDetail, { props: { site: site() } });
  expect((await findByRole('tab', { name: 'Agents' })).getAttribute('aria-selected')).toBe('true');
  await rerender({ site: site({ hidden_tabs: ['agents'] }) });
  const overview = await findByRole('tab', { name: 'Overview' });
  expect(overview.getAttribute('aria-selected')).toBe('true');
  expect(overview.getAttribute('tabindex')).toBe('0');
});
