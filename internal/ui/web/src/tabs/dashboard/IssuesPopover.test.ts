import { render, fireEvent, screen } from '@testing-library/svelte';
import { describe, it, expect, beforeEach } from 'vitest';
import { createRawSnippet } from 'svelte';
import IssuesPopover from './IssuesPopover.svelte';
import { sites } from '$stores/sites';
import { services, serviceLabel } from '$stores/services';

const pill = createRawSnippet(() => ({ render: () => '<span>There are some issues</span>' }));
const openList = () => fireEvent.click(screen.getByRole('button', { name: /There are some issues/ }));
const row = (name: string) => screen.getByText(name).closest('button')!;

describe('IssuesPopover', () => {
  beforeEach(() => {
    location.hash = '';
    sites.set([
      { domain: 'up.test', uses_php: true, fpm_running: true },
      { domain: 'down.test', uses_php: true },
      { domain: 'broken.test', custom_container: true, container_port: 8000, container_build_failed: true }
    ]);
    services.set([
      { name: 'mysql', status: 'active', site_count: 1 },
      { name: 'mailpit', status: 'failed', site_count: 0 },
      { name: 'staging-db', status: 'offline', site_count: 1, external: 'db.internal:3306' }
    ]);
  });

  it('counts what is not running on the pill', () => {
    render(IssuesPopover, { props: { box: '', pill } });
    expect(screen.getByRole('button', { name: /There are some issues/ }).textContent).toContain('4');
  });

  it('lists each site and service that is not running with why', async () => {
    render(IssuesPopover, { props: { box: '', pill } });
    await openList();
    expect(row('down.test').textContent).toContain('PHP not running');
    expect(row('broken.test').textContent).toContain('build failed');
    expect(row(serviceLabel('mailpit')).textContent).toContain('failed');
    expect(row(serviceLabel('staging-db')).textContent).toContain('offline');
    expect(screen.queryByText('up.test')).toBeNull();
  });

  it('opens the page that holds the fix and closes the list', async () => {
    render(IssuesPopover, { props: { box: '', pill } });
    await openList();
    await fireEvent.click(row('down.test'));
    expect(location.hash).toBe('#sites/down.test');
    expect(screen.queryByText('down.test')).toBeNull();

    await openList();
    await fireEvent.click(row(serviceLabel('staging-db')));
    expect(location.hash).toBe('#services/staging-db');
  });
});
