import { render } from '@testing-library/svelte';
import { vi, describe, it, expect, beforeEach, afterEach } from 'vitest';
import NotifyBanner from './NotifyBanner.svelte';
import { permissionState, dismissed, notifyDelivery } from '$lib/notify';

// Get started asks these questions itself; these tests are about an install
// that never went through it.
vi.mock('$stores/setup', async () => {
  const { readable } = await import('svelte/store');
  return { bannersAllowed: readable(true) };
});


const realLocation = window.location;

function at(hostname: string, port: string) {
  Object.defineProperty(window, 'location', {
    value: { hostname, port, href: `http://${hostname}:${port}/` },
    writable: true,
    configurable: true
  });
}

describe('NotifyBanner', () => {
  beforeEach(() => {
    permissionState.set('default');
    dismissed.set(false);
    notifyDelivery.set('browser');
    at('lerd.localhost', '');
  });

  afterEach(() => {
    Object.defineProperty(window, 'location', {
      value: realLocation,
      writable: true,
      configurable: true
    });
  });

  it('asks for permission on the vhost', () => {
    const { container } = render(NotifyBanner);
    expect(container.querySelector('button')).toBeTruthy();
  });

  // Permission is per-origin and the fallback page hands over to the vhost as
  // soon as nginx is back, so granting it here would leave a second browser
  // subscribed for good, delivering everything twice.
  it('stays quiet on the loopback fallback origin', () => {
    at('127.0.0.1', '7073');
    const { container } = render(NotifyBanner);
    expect(container.querySelector('button')).toBeNull();
  });

  it('still asks on a LAN session served from the same port', () => {
    at('192.168.1.20', '7073');
    const { container } = render(NotifyBanner);
    expect(container.querySelector('button')).toBeTruthy();
  });

  it('stays quiet when the daemon delivers natively', () => {
    notifyDelivery.set('native');
    const { container } = render(NotifyBanner);
    expect(container.querySelector('button')).toBeNull();
  });
});
