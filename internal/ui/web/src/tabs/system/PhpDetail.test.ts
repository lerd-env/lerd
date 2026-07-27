import { render, screen } from '@testing-library/svelte';
import { describe, it, expect } from 'vitest';
import { readable } from 'svelte/store';
import { tick } from 'svelte';
import { vi } from 'vitest';

vi.mock('$stores/status', async (orig) => {
  const actual = (await orig()) as object;
  return {
    ...actual,
    status: readable({
      php_default: '8.4',
      php_fpms: [{ version: '8.4', patch: '8.4.12', running: false, xdebug_enabled: false }]
    }),
    loadStatus: vi.fn()
  };
});
vi.mock('$stores/sites', async (orig) => {
  const actual = (await orig()) as object;
  return { ...actual, sites: readable([]), sitesByPhp: readable(new Map()) };
});

import PhpDetail from './PhpDetail.svelte';

describe('PhpDetail', () => {
  it('leaves the version, default star and status pill to the card', () => {
    render(PhpDetail, { props: { version: '8.4' } });
    expect(screen.queryByText(/PHP 8\.4\.12/)).not.toBeInTheDocument();
    expect(screen.queryByText('Stopped')).not.toBeInTheDocument();
  });

  it('keeps the Xdebug control in the tab strip', () => {
    const { container } = render(PhpDetail, { props: { version: '8.4' } });
    const xdebug = screen.getByText('Xdebug');
    expect(container.querySelector('.border-b')?.contains(xdebug)).toBe(true);
  });

  // The default version has no lifecycle actions, but rebuild and the update
  // check still apply to it: it is the version most sites run on.
  it('offers rebuild and the update check on the default version', async () => {
    const { container } = render(PhpDetail, { props: { version: '8.4' } });
    expect(screen.getByText('Rebuild image')).toBeInTheDocument();
    const toggle = container.querySelector<HTMLButtonElement>('button[aria-haspopup="menu"]');
    expect(toggle).not.toBeNull();
    toggle!.click();
    await tick();
    expect(screen.getByText('Check for updates')).toBeInTheDocument();
  });
});
