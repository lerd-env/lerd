import { render, screen } from '@testing-library/svelte';
import { describe, it, expect, beforeEach } from 'vitest';
import { readable } from 'svelte/store';
import { tick } from 'svelte';
import { vi } from 'vitest';

const statusStore = vi.hoisted(() => {
  // Plain store object rather than svelte/store: the import cannot be awaited
  // in a hoisted block, and PhpDetail only ever subscribes.
  let value: Record<string, unknown> = {};
  const subs = new Set<(v: Record<string, unknown>) => void>();
  return {
    set(next: Record<string, unknown>) {
      value = next;
      subs.forEach((fn) => fn(value));
    },
    subscribe(fn: (v: Record<string, unknown>) => void) {
      subs.add(fn);
      fn(value);
      return () => subs.delete(fn);
    }
  };
});

vi.mock('$stores/status', async (orig) => {
  const actual = (await orig()) as object;
  return { ...actual, status: statusStore, loadStatus: vi.fn() };
});
vi.mock('$stores/sites', async (orig) => {
  const actual = (await orig()) as object;
  return { ...actual, sites: readable([]), sitesByPhp: readable(new Map()) };
});

import PhpDetail from './PhpDetail.svelte';

function setStatus(fpm: Record<string, unknown> = {}) {
  statusStore.set({
    php_default: '8.4',
    php_fpms: [
      { version: '8.4', patch: '8.4.12', running: false, xdebug_enabled: false, ...fpm }
    ]
  });
}

function openMenu(container: HTMLElement) {
  const toggle = container.querySelector<HTMLButtonElement>('button[aria-haspopup="menu"]');
  expect(toggle).not.toBeNull();
  toggle!.click();
  return tick();
}

describe('PhpDetail', () => {
  beforeEach(() => setStatus());

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

  // A rebuild with nothing to pick up is a slow no-op, so it waits in the menu
  // rather than sitting in the button next to Xdebug inviting a click.
  it('keeps rebuild in the menu until the base has moved', async () => {
    const { container } = render(PhpDetail, { props: { version: '8.4' } });
    expect(screen.getByText('Check for updates')).toBeInTheDocument();
    expect(screen.queryByText('Rebuild image')).not.toBeInTheDocument();

    await openMenu(container);
    expect(screen.getByText('Rebuild image')).toBeInTheDocument();
  });

  it('promotes rebuild to the button once the base is republished', () => {
    setStatus({ update_available: true });
    render(PhpDetail, { props: { version: '8.4' } });
    expect(screen.getByText('Rebuild on the new base')).toBeInTheDocument();
  });
});
