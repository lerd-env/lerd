import { render } from '@testing-library/svelte';
import { describe, it, expect, beforeEach } from 'vitest';
import SitesDashboard from './SitesDashboard.svelte';
import { sites, sitesLoaded, type Site } from '$stores/sites';
import { status } from '$stores/status';

function site(over: Partial<Site> = {}): Site {
  return { domain: 'app.test', ...over } as Site;
}

function setWorkspaces(names: string[]) {
  status.update((s) => ({ ...s, workspaces: names }));
}

describe('SitesDashboard', () => {
  beforeEach(() => {
    sites.set([]);
    sitesLoaded.set(true);
    setWorkspaces([]);
  });

  it('groups active sites under their workspace', () => {
    setWorkspaces(['Client Work', 'Side Projects']);
    sites.set([
      site({ domain: 'shop.test', workspace: 'Client Work', fpm_running: true }),
      site({ domain: 'blog.test', workspace: 'Side Projects' })
    ]);
    const { getByText } = render(SitesDashboard);
    expect(getByText('Client Work')).toBeTruthy();
    expect(getByText('Side Projects')).toBeTruthy();
    expect(getByText('shop.test')).toBeTruthy();
    expect(getByText('blog.test')).toBeTruthy();
  });

  it('keeps the framework visible on the tile rather than as a heading', () => {
    setWorkspaces(['Client Work']);
    sites.set([site({ domain: 'shop.test', workspace: 'Client Work', framework_label: 'Laravel' })]);
    const { getByText, queryByRole } = render(SitesDashboard);
    expect(getByText('Laravel')).toBeTruthy();
    expect(queryByRole('heading', { name: 'Laravel' })).toBeNull();
  });

  it('lists sites in no workspace without a heading', () => {
    setWorkspaces(['Client Work']);
    sites.set([
      site({ domain: 'shop.test', workspace: 'Client Work' }),
      site({ domain: 'static.test' })
    ]);
    const { getByText, queryByText } = render(SitesDashboard);
    expect(getByText('static.test')).toBeTruthy();
    expect(queryByText('Ungrouped')).toBeNull();
    expect(queryByText('Other')).toBeNull();
  });

  it('hides an empty workspace so the overview stays uncluttered', () => {
    setWorkspaces(['Client Work', 'Empty']);
    sites.set([site({ domain: 'shop.test', workspace: 'Client Work' })]);
    const { getByText, queryByText } = render(SitesDashboard);
    expect(getByText('Client Work')).toBeTruthy();
    expect(queryByText('Empty')).toBeNull();
  });

  it('treats a site whose workspace was deleted as ungrouped', () => {
    setWorkspaces(['Client Work']);
    sites.set([site({ domain: 'orphan.test', workspace: 'Deleted' })]);
    const { getByText, queryByText } = render(SitesDashboard);
    expect(getByText('orphan.test')).toBeTruthy();
    expect(queryByText('Deleted')).toBeNull();
  });

  it('renders a flat grid when no workspace is configured', () => {
    sites.set([site({ domain: 'a.test' }), site({ domain: 'b.test' })]);
    const { getByText, container } = render(SitesDashboard);
    expect(getByText('a.test')).toBeTruthy();
    expect(getByText('b.test')).toBeTruthy();
    expect(container.querySelectorAll('h2').length).toBe(0);
  });

  it('lists paused sites in their own section', () => {
    sites.set([site({ domain: 'old.test', paused: true })]);
    const { getByText } = render(SitesDashboard);
    expect(getByText('Paused')).toBeTruthy();
    expect(getByText('old.test')).toBeTruthy();
  });

  // The header stats read as counts, so the paused one has to too, the way its
  // "10/15 running" and "2 failing" neighbours do.
  it('summarises the paused sites as a count, not a label', () => {
    sites.set([
      site({ domain: 'a.test', fpm_running: true }),
      site({ domain: 'old.test', paused: true }),
      site({ domain: 'older.test', paused: true })
    ]);
    const { getByText, queryByText } = render(SitesDashboard);
    expect(getByText('1/3 running')).toBeTruthy();
    expect(getByText('2 paused')).toBeTruthy();
    expect(queryByText('Paused 2')).toBeNull();
  });

  it('omits the paused count when nothing is paused', () => {
    sites.set([site({ domain: 'a.test', fpm_running: true })]);
    const { queryByText } = render(SitesDashboard);
    expect(queryByText(/paused/)).toBeNull();
  });

  it('shows the empty hint when there are no sites', () => {
    sites.set([]);
    const { getByText } = render(SitesDashboard);
    expect(getByText('No sites yet')).toBeTruthy();
  });
});
