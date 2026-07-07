import { render } from '@testing-library/svelte';
import { describe, it, expect, vi } from 'vitest';
import type { Site } from '$stores/sites';

vi.mock('$stores/commands', async () => {
  const { writable } = await import('svelte/store');
  return {
    loadCommands: vi.fn().mockResolvedValue([
      { name: 'migrate', label: 'Migrate', command: 'php artisan migrate', icon: 'database' },
      { name: 'cache', label: 'Clear caches', command: 'php artisan optimize:clear', icon: 'broom' }
    ]),
    launchCommand: vi.fn(),
    commandIconPath: () => 'M0 0',
    runningName: writable<string | null>(null),
    executeCommand: vi.fn(),
    executeDoctorFix: vi.fn()
  };
});

import SiteActionCards from './SiteActionCards.svelte';

const site = { domain: 'acme.test', name: 'acme' } as Site;

describe('SiteActionCards', () => {
  it('always renders the Doctor card', () => {
    const { getByText } = render(SiteActionCards, { props: { site } });
    expect(getByText('Doctor')).toBeTruthy();
    expect(getByText('Run health checks')).toBeTruthy();
  });

  it('renders a card per loaded command', async () => {
    const { findByText } = render(SiteActionCards, { props: { site } });
    expect(await findByText('Migrate')).toBeTruthy();
    expect(await findByText('Clear caches')).toBeTruthy();
  });

  it('loads commands for the active worktree branch', async () => {
    const { loadCommands } = await import('$stores/commands');
    render(SiteActionCards, { props: { site, branch: 'staging' } });
    expect(loadCommands).toHaveBeenCalledWith('acme.test', 'staging');
  });
});
