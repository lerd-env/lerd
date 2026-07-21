import { render } from '@testing-library/svelte';
import { describe, it, expect } from 'vitest';
import Harness from './SiteHeader.test.svelte';
import type { Site } from '$stores/sites';

const site = {
  domain: 'app.test',
  domains: ['app.test'],
  path: '/home/u/Code/app',
  php_version: '8.3',
  worktrees: []
} as unknown as Site;

const worktreeSite = {
  ...site,
  branch: 'main',
  worktrees: [{ branch: 'feat', domain: 'feat.app.test', path: '/home/u/Code/app-feat' }]
} as unknown as Site;

const hostProxySite = {
  ...site,
  php_version: '',
  host_proxy: true,
  host_port: 5173,
  host_has_dev_server: true
} as unknown as Site;

describe('SiteHeader', () => {
  it('offers a restart action for a host-proxy site running a dev server', () => {
    const { getByLabelText } = render(Harness, { props: { site: hostProxySite } });

    expect(getByLabelText('Restart dev server')).toBeInTheDocument();
  });

  it('has no dev-server restart action for a proxy-only site', () => {
    const proxyOnly = { ...hostProxySite, host_has_dev_server: false } as unknown as Site;
    const { queryByLabelText } = render(Harness, { props: { site: proxyOnly } });

    expect(queryByLabelText('Restart dev server')).not.toBeInTheDocument();
  });

  it('has no dev-server restart action on a plain PHP site', () => {
    const { queryByLabelText } = render(Harness, { props: { site } });

    expect(queryByLabelText('Restart dev server')).not.toBeInTheDocument();
  });

  it('puts the path on the tab row, to the right of the tabs', () => {
    const { getByText } = render(Harness, { props: { site } });

    const tabRow = getByText('Overview').parentElement?.parentElement;
    expect(tabRow).toContainElement(getByText('/home/u/Code/app'));
  });

  it('shows the path once when the site also has worktree tabs', () => {
    const { getAllByText, getByText } = render(Harness, {
      props: { site: worktreeSite }
    });

    expect(getAllByText('/home/u/Code/app')).toHaveLength(1);
    const tabRow = getByText('Overview').parentElement?.parentElement;
    expect(tabRow).toContainElement(getByText('/home/u/Code/app'));
  });

  it('shows the active worktree path rather than the parent path', () => {
    const { getByText, queryByText } = render(Harness, {
      props: { site: worktreeSite, activeWorktreeBranch: 'feat' }
    });

    expect(getByText('/home/u/Code/app-feat')).toBeInTheDocument();
    expect(queryByText('/home/u/Code/app')).not.toBeInTheDocument();
  });

  it('still shows the path when the site has no tabs', () => {
    const { getByText } = render(Harness, { props: { site, withTabs: false } });

    expect(getByText('/home/u/Code/app')).toBeInTheDocument();
  });
});
