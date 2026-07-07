import { render, fireEvent } from '@testing-library/svelte';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import ServiceToolsTab from './ServiceToolsTab.svelte';
import type { Service } from '$stores/services';

const { setServiceShim } = vi.hoisted(() => ({ setServiceShim: vi.fn() }));
vi.mock('$stores/services', async (orig) => {
  const actual = (await orig()) as object;
  return { ...actual, setServiceShim };
});

function svc(over: Partial<Service> = {}): Service {
  return {
    name: 'postgres',
    status: 'active',
    site_count: 0,
    client_shims: [{ tool: 'psql', enabled: false, host_has: false, decided: false }],
    ...over
  } as Service;
}

describe('ServiceToolsTab', () => {
  beforeEach(() => setServiceShim.mockReset());

  it('toggles a tool through the service shim store', async () => {
    setServiceShim.mockResolvedValue({ ok: true });
    const { getByRole } = render(ServiceToolsTab, { props: { svc: svc() } });
    await fireEvent.click(getByRole('button'));
    expect(setServiceShim).toHaveBeenCalledWith('postgres', { tool: 'psql', enabled: true });
  });

  it('marks a tool failed when its toggle rejects', async () => {
    setServiceShim.mockResolvedValue({ ok: false, error: 'boom' });
    const { getByRole } = render(ServiceToolsTab, { props: { svc: svc() } });
    const toggle = getByRole('button');
    await fireEvent.click(toggle);
    // A failed toggle marks that tool failing, which the control shows in red.
    expect(toggle.className).toContain('bg-red-500');
  });

  it('disables a tool managed by another service', () => {
    const s = svc({ client_shims: [{ tool: 'psql', enabled: true, host_has: false, decided: true, owner: 'postgres-16' }] });
    const { getByRole } = render(ServiceToolsTab, { props: { svc: s } });
    expect((getByRole('button') as HTMLButtonElement).disabled).toBe(true);
  });
});
