import { describe, it, expect, vi } from 'vitest';
import { render } from '@testing-library/svelte';
import { status } from '$stores/status';
import { routeRest } from '$stores/route';
import SystemDetail from './SystemDetail.svelte';

describe('SystemDetail', () => {
  // The System list drops the DNS row while DNS is off, but its panel still
  // answers at #system/dns with the disabled pill and how to turn it back on.
  it('shows the DNS panel as disabled when DNS is off', () => {
    globalThis.fetch = vi.fn(async () => new Response('{}', { status: 200 })) as unknown as typeof fetch;
    status.set({ dns: { enabled: false, ok: false, tld: 'test' } } as never);
    routeRest.set('dns');

    const { container } = render(SystemDetail);

    expect(container.textContent).toContain('Disabled');
    expect(container.textContent).toContain('dns.enabled: true');
  });
});
