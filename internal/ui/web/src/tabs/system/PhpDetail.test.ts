import { render, screen } from '@testing-library/svelte';
import { describe, it, expect, beforeEach } from 'vitest';
import { readable, writable } from 'svelte/store';
import { tick } from 'svelte';
import { vi } from 'vitest';

const phpRuntimeStore = vi.hoisted(() => {
  // Plain store object for the same reason statusStore is one: the import
  // cannot be awaited inside a hoisted factory.
  let value = 'container';
  const subs = new Set<(v: string) => void>();
  return {
    subscribe(fn: (v: string) => void) {
      fn(value);
      subs.add(fn);
      return () => subs.delete(fn);
    },
    set(v: string) {
      value = v;
      subs.forEach((fn) => fn(value));
    }
  };
});
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
vi.mock('$stores/phpRuntime', async (orig) => {
  const actual = (await orig()) as object;
  return { ...actual, phpRuntime: phpRuntimeStore, loadPHPRuntime: vi.fn() };
});
vi.mock('$stores/sites', async (orig) => {
  const actual = (await orig()) as object;
  return { ...actual, sites: writable([]), sitesByPhp: readable(new Map()) };
});

import PhpDetail from './PhpDetail.svelte';
import { sites } from '$stores/sites';

function setStatus(fpm: Record<string, unknown> = {}) {
  statusStore.set({
    php_default: '8.4',
    php_fpms: [
      { version: '8.4', patch: '8.4.12', running: false, xdebug_enabled: false, ...fpm }
    ]
  });
}

function openMenu(container: HTMLElement) {
  const toggle = container.querySelector<HTMLButtonElement>('[data-testid="button-menu-toggle"]');
  expect(toggle).not.toBeNull();
  toggle!.click();
  return tick();
}

describe('PhpDetail', () => {
  beforeEach(() => {
    setStatus();
    sites.set([]);
  });

  it('carries the sites on the version in the tab strip instead of a tab of its own', async () => {
    sites.set([
      { domain: 'acme.test', php_version: '8.4', fpm_running: true },
      { domain: 'shop.test', php_version: '8.4' },
      { domain: 'old.test', php_version: '8.3' }
    ]);
    const { container } = render(PhpDetail, { props: { version: '8.4' } });

    expect(screen.queryByRole('button', { name: 'Sites' })).not.toBeInTheDocument();
    const trigger = screen.getByLabelText('2 sites');
    expect(container.querySelector('.border-b')?.contains(trigger)).toBe(true);

    trigger.click();
    await tick();
    expect(screen.getByText('acme.test')).toBeInTheDocument();
    expect(screen.getByText('shop.test')).toBeInTheDocument();
    expect(screen.queryByText('old.test')).not.toBeInTheDocument();
  });

  it('leaves the tab strip without a sites control when no site is on the version', () => {
    render(PhpDetail, { props: { version: '8.4' } });
    expect(screen.queryByLabelText(/sites$/)).not.toBeInTheDocument();
  });

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
    expect(screen.queryByText('Rebuild image')).not.toBeInTheDocument();

    await openMenu(container);
    expect(screen.getByText('Rebuild image')).toBeInTheDocument();
    expect(screen.getByText('Check for updates')).toBeInTheDocument();
  });

  it('promotes rebuild to the button once the base is republished', () => {
    setStatus({ update_available: true });
    render(PhpDetail, { props: { version: '8.4' } });
    expect(screen.getByText('Rebuild on the new base')).toBeInTheDocument();
  });

  // The shell is the one action worth a click on a version that is just sitting
  // there, so it leads the group rather than waiting in the menu.
  it('offers a shell in the container, and only while it is running', async () => {
    render(PhpDetail, { props: { version: '8.4' } });
    expect(screen.getByText('Terminal').closest('button')).toBeDisabled();

    setStatus({ running: true });
    await tick();
    expect(screen.getByText('Terminal').closest('button')).not.toBeDisabled();
  });

  // Those ports are published on the FPM container's quadlet. A host pool
  // listens on the port its version owns and there is nothing to map, so the
  // tab would offer a setting that changes nothing.
  it('hides the ports tab on the native runtime', async () => {
    phpRuntimeStore.set('native');
    render(PhpDetail, { props: { version: '8.4' } });
    await tick();
    expect(screen.queryByText('Ports')).toBeNull();
    phpRuntimeStore.set('container');
  });

  it('keeps the ports tab on the container runtime', async () => {
    phpRuntimeStore.set('container');
    render(PhpDetail, { props: { version: '8.4' } });
    await tick();
    expect(screen.queryByText('Ports')).not.toBeNull();
  });
});
