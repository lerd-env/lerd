import { render } from '@testing-library/svelte';
import { describe, it, expect, beforeEach } from 'vitest';

import LANServicesSetting from './LANServicesSetting.svelte';
import { lan } from '$stores/lan';
import { accessMode } from '$stores/accessMode';

function setLAN(over: Partial<{ exposed: boolean; servicesEnabled: boolean; servicesReachable: boolean }>) {
  lan.update((v) => ({
    ...v,
    loaded: true,
    exposed: false,
    servicesEnabled: false,
    servicesReachable: false,
    servicesLoading: false,
    servicesError: '',
    ...over
  }));
}

describe('LANServicesSetting', () => {
  beforeEach(() => {
    accessMode.set({ localControl: true, lanExposed: false, checked: true });
    setLAN({});
  });

  // The setting is what tells you managed services stay private even while
  // sites are exposed, so it has to be readable before you expose anything.
  it('renders its control once lerd is exposed', () => {
    setLAN({ exposed: true });
    const { getByRole, container } = render(LANServicesSetting);
    expect(getByRole('button')).toBeInTheDocument();
    expect(container.textContent).toContain('Managed service LAN access');
  });

  it('explains that an opted-in setting is not in effect yet', () => {
    setLAN({ exposed: false, servicesEnabled: true, servicesReachable: false });
    const { container } = render(LANServicesSetting);
    expect(container.textContent).toContain('services remain on loopback until LAN exposure is enabled');
  });

  it('warns about weak credentials once services are reachable', () => {
    setLAN({ exposed: true, servicesEnabled: true, servicesReachable: true });
    const { container } = render(LANServicesSetting);
    expect(container.textContent).toContain('trusted network');
  });

  it('shows read-only state instead of a toggle without dashboard-control authority', () => {
    accessMode.set({ localControl: false, lanExposed: true, checked: true });
    setLAN({ exposed: true });
    const { queryByTitle, container } = render(LANServicesSetting);
    expect(queryByTitle('Managed service LAN access')).toBeNull();
    expect(container.textContent).toContain('loopback only');
  });

  // Read-only state is a three-way pill: reachable, armed but inert, off.
  it('reads the three service states without dashboard-control authority', () => {
    accessMode.set({ localControl: false, lanExposed: true, checked: true });
    setLAN({ exposed: true, servicesEnabled: true, servicesReachable: true });
    expect(render(LANServicesSetting).container.textContent).toContain('enabled');

    setLAN({ exposed: false, servicesEnabled: true, servicesReachable: false });
    expect(render(LANServicesSetting).container.textContent).toContain('inert');
  });

  it('separates itself from the exposure controls when nested', () => {
    setLAN({ exposed: true });
    const { container } = render(LANServicesSetting, { props: { nested: true } });
    expect(container.querySelector('div')?.className).toContain('border-t');
  });
});

describe('LANServicesSetting while lerd is loopback only', () => {
  beforeEach(() => {
    accessMode.set({ localControl: true, lanExposed: false, checked: true });
    setLAN({ exposed: false, servicesEnabled: false });
  });

  // The setting publishes nothing in this state, so it is not shown at all.
  it('renders nothing', () => {
    const { container } = render(LANServicesSetting);
    expect(container.textContent?.trim()).toBe('');
  });

  // Re-exposing would otherwise republish databases nobody remembered arming.
  it('keeps showing an already enabled setting so it can be cleared', () => {
    setLAN({ exposed: false, servicesEnabled: true });
    const { getByRole, container } = render(LANServicesSetting);
    expect(getByRole('button')).not.toBeDisabled();
    expect(container.textContent).toContain('Managed service LAN access');
  });

  it('reappears once lerd is exposed', () => {
    setLAN({ exposed: true, servicesEnabled: false });
    const { getByRole } = render(LANServicesSetting);
    expect(getByRole('button')).not.toBeDisabled();
  });
});
