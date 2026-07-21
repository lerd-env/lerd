import { render, screen } from '@testing-library/svelte';
import { describe, it, expect } from 'vitest';
import { readable } from 'svelte/store';
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
});
