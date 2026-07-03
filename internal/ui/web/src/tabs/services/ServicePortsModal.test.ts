import { render, fireEvent } from '@testing-library/svelte';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import ServicePortsModal from './ServicePortsModal.svelte';
import type { Service } from '$stores/services';

const { setServicePorts } = vi.hoisted(() => ({ setServicePorts: vi.fn(async () => ({ ok: true })) }));
vi.mock('$stores/services', async (orig) => {
  const actual = (await orig()) as object;
  return { ...actual, setServicePorts };
});

function svc(over: Partial<Service> = {}): Service {
  return {
    name: 'mailpit',
    status: 'active',
    site_count: 0,
    preset_owned: true,
    default_port: 1025,
    secondary_ports: [{ container: 8025, default: 8025 }],
    ...over
  } as Service;
}

describe('ServicePortsModal', () => {
  beforeEach(() => setServicePorts.mockClear());

  it('saves a changed primary port', async () => {
    const { container, getByText } = render(ServicePortsModal, {
      props: { open: true, svc: svc(), onclose: () => {} }
    });
    const input = container.querySelector('input[type="number"]') as HTMLInputElement;
    await fireEvent.input(input, { target: { value: '1026' } });
    await fireEvent.click(getByText('Save'));
    expect(setServicePorts).toHaveBeenCalledWith('mailpit', {
      published_port: 1026,
      published_ports: { '8025': 8025 },
      extra_ports: []
    });
  });

  it('saves a changed secondary port keyed by container port', async () => {
    const { container, getByText } = render(ServicePortsModal, {
      props: { open: true, svc: svc(), onclose: () => {} }
    });
    const inputs = container.querySelectorAll('input[type="number"]');
    // Second published-port field is the 8025 secondary mapping.
    await fireEvent.input(inputs[1], { target: { value: '8026' } });
    await fireEvent.click(getByText('Save'));
    expect(setServicePorts).toHaveBeenCalledWith('mailpit', {
      published_port: null,
      published_ports: { '8025': 8026 },
      extra_ports: []
    });
  });
});
