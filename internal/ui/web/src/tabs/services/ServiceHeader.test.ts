import { render, screen } from '@testing-library/svelte';
import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import { idleEnabled, idleServices } from '$stores/idle';
import ServiceHeader from './ServiceHeader.svelte';
import { sites } from '$stores/sites';
import { serviceIcons } from '$stores/serviceIcons';
import type { Service } from '$stores/services';

function service(over: Partial<Service> = {}): Service {
  return { name: 'mysql', status: 'active', site_count: 0, ...over } as Service;
}

describe('ServiceHeader site links', () => {
  beforeEach(() => {
    sites.set([
      { domain: 'acme.test', name: 'acme', fpm_running: true },
      { domain: 'shop.test', name: 'shop', paused: true }
    ]);
    serviceIcons.set({ mysql: '<svg viewBox="0 0 24 24"><path d="M3 3h18v18H3z"/></svg>' });
  });

  it('leads with the mark the service store ships', () => {
    const { container } = render(ServiceHeader, { props: { svc: service({ preset: 'mysql' }) } });
    expect(container.querySelector('.mark-glyph path')).not.toBeNull();
  });

  it('leaves a worker without a mark rather than drawing the fallback glyph', () => {
    const { container } = render(ServiceHeader, {
      props: { svc: service({ name: 'queue-acme', queue_site: 'acme', preset: 'mysql' }) }
    });
    expect(container.querySelector('.mark-glyph')).toBeNull();
  });

  it('collapses the sites using the service into one button beside the action menu', () => {
    const { container } = render(ServiceHeader, {
      props: { svc: service({ site_domains: ['acme.test', 'shop.test'] }) }
    });
    const trigger = screen.getByLabelText('2 sites');
    expect(screen.queryByText('acme.test')).not.toBeInTheDocument();

    const toggle = container.querySelector('[data-testid="button-menu-toggle"]')!;
    expect(toggle.closest('div')!.parentElement!.contains(trigger)).toBe(true);
  });

  it('shows nothing when no site uses the service', () => {
    render(ServiceHeader, { props: { svc: service() } });
    expect(screen.queryByLabelText(/sites$/)).not.toBeInTheDocument();
  });

  it('keeps a worker parent site inline rather than behind the control', () => {
    render(ServiceHeader, { props: { svc: service({ name: 'queue-acme', queue_site: 'acme' }) } });
    expect(screen.getByText('acme.test')).toBeInTheDocument();
    expect(screen.queryByLabelText(/sites$/)).not.toBeInTheDocument();
  });
});

describe('ServiceHeader pin', () => {
  afterEach(() => {
    idleEnabled.set(false);
    idleServices.set(false);
  });

  it('keeps pin in the button group while services sleep when idle', () => {
    idleEnabled.set(true);
    idleServices.set(true);
    const { getByTestId } = render(ServiceHeader, { props: { svc: service() } });
    const pin = getByTestId('button-menu-inline-pin');
    expect(pin.getAttribute('aria-label')).toBe('Pin, prevent auto-stop when unused');
  });

  it('leaves pin in the menu otherwise', () => {
    idleEnabled.set(true);
    const { queryByTestId } = render(ServiceHeader, { props: { svc: service() } });
    expect(queryByTestId('button-menu-inline-pin')).toBeNull();
  });

  it('shows a pinned service as pinned in the group', () => {
    idleEnabled.set(true);
    idleServices.set(true);
    const { getByTestId } = render(ServiceHeader, { props: { svc: service({ pinned: true }) } });
    expect(getByTestId('button-menu-inline-pin').className).toMatch(/amber/);
  });
});

