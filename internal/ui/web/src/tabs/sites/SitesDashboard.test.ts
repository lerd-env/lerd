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

  // Without workspaces the overview keeps the framework grouping it has always
  // had, so a user who never creates one loses no structure.
  it('falls back to framework sections when no workspace is configured', () => {
    sites.set([
      site({ domain: 'a.test', framework_label: 'Laravel' }),
      site({ domain: 'b.test', framework_label: 'Symfony' }),
      site({ domain: 'c.test' })
    ]);
    const { getByRole, getByText } = render(SitesDashboard);
    expect(getByRole('heading', { name: 'Laravel' })).toBeTruthy();
    expect(getByRole('heading', { name: 'Symfony' })).toBeTruthy();
    expect(getByRole('heading', { name: 'Other' })).toBeTruthy();
    expect(getByText('c.test')).toBeTruthy();
  });

  // The unknown-framework bucket trails the named ones.
  it('sorts the Other framework bucket last', () => {
    sites.set([site({ domain: 'a.test' }), site({ domain: 'b.test', framework_label: 'Laravel' })]);
    const { container } = render(SitesDashboard);
    const headings = [...container.querySelectorAll('h2')].map((h) => h.textContent);
    expect(headings).toEqual(['Laravel', 'Other']);
  });

  // Once a workspace exists the framework headings give way to it.
  it('drops the framework headings as soon as a workspace exists', () => {
    setWorkspaces(['Client Work']);
    sites.set([site({ domain: 'a.test', workspace: 'Client Work', framework_label: 'Laravel' })]);
    const { getByRole, queryByRole } = render(SitesDashboard);
    expect(getByRole('heading', { name: 'Client Work' })).toBeTruthy();
    expect(queryByRole('heading', { name: 'Laravel' })).toBeNull();
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
