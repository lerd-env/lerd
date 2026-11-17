import { render, fireEvent } from '@testing-library/svelte';
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

// A plugin can ask for its tab only where it has something to show: a database
// browser on a static site would be an empty tab.
it('shows a tab that asks for a database only on sites that have one', async () => {
  plugins.set([
    {
      name: 'db-browser',
      site_tab: { title: 'Database', url: '/plugins/db-browser/ui/site.html', when: 'database', cond: { is: 'database' } }
    },
    { name: 'sync', site_tab: { title: 'Sync', url: '/plugins/sync/ui/tab.html' } }
  ]);
  const { findByRole, queryByRole, rerender } = render(SiteDetail, { props: { site: site() } });
  await findByRole('tab', { name: 'Sync' });
  expect(queryByRole('tab', { name: 'Database' })).toBeNull();

  await rerender({ site: site({ has_database: true }) });
  await findByRole('tab', { name: 'Database' });
});

// A worktree can sit on another framework major and database than the main
// checkout, so switching checkouts changes which tabs show.
it('follows the checkout picked in the worktree picker', async () => {
  plugins.set([
    {
      name: 'inspect',
      site_tab: {
        title: 'Laravel',
        url: '/plugins/inspect/ui/site.html',
        when: 'framework:laravel@>=11',
        cond: { is: 'framework', name: 'laravel', cmp: '>=', version: 11 }
      }
    },
    {
      name: 'db-browser',
      site_tab: { title: 'Database', url: '/plugins/db-browser/ui/site.html', when: 'database', cond: { is: 'database' } }
    }
  ]);
  globalThis.fetch = vi.fn(
    async (url: string) => new Response(String(url).includes('git-status') ? '{"checkouts":[]}' : '{}')
  ) as unknown as typeof fetch;
  localStorage.setItem('lerd:siteDetailTab', 'plugin:db-browser');
  const laravel = site({
    branch: 'main',
    framework: 'laravel',
    framework_version: '10',
    has_database: true,
    worktrees: [{ branch: 'upgrade', domain: 'upgrade.shop.test', framework_version: '11' }]
  });
  const { findByRole, queryByRole, getAllByText } = render(SiteDetail, { props: { site: laravel } });
  expect((await findByRole('tab', { name: 'Database' })).getAttribute('aria-selected')).toBe('true');
  expect(queryByRole('tab', { name: 'Laravel' })).toBeNull();

  const checkoutButton = (branch: string) => getAllByText(branch)[0].closest('button')!;
  await fireEvent.click(checkoutButton('upgrade'));
  await findByRole('tab', { name: 'Laravel' });
  expect(queryByRole('tab', { name: 'Database' })).toBeNull();
  // The open tab stopped matching, so the row falls back to Overview.
  expect((await findByRole('tab', { name: 'Overview' })).getAttribute('aria-selected')).toBe('true');

  await fireEvent.click(checkoutButton('main'));
  expect((await findByRole('tab', { name: 'Database' })).getAttribute('aria-selected')).toBe('true');
  expect(queryByRole('tab', { name: 'Laravel' })).toBeNull();
});
