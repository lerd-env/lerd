import { render, within } from '@testing-library/svelte';
import { describe, it, expect } from 'vitest';
import ServiceStatusDot from './ServiceStatusDot.svelte';
import { idleEnabled, idleServices } from '$stores/idle';

describe('ServiceStatusDot', () => {
  it('shows the moon for a service idle-suspend put to sleep', () => {
    const { getByTestId, container } = render(ServiceStatusDot, {
      props: { svc: { status: 'inactive', idle_suspended: true } }
    });
    expect(getByTestId('service-moon').getAttribute('title')).toBe('Sleeping');
    expect(container.querySelector('.rounded-full')).toBeNull();
  });

  it('shows the green dot once it is running again, even before the flag clears', () => {
    const { queryByTestId, container } = render(ServiceStatusDot, {
      props: { svc: { status: 'active', idle_suspended: true } }
    });
    expect(queryByTestId('service-moon')).toBeNull();
    expect(container.querySelector('.bg-emerald-500')).toBeTruthy();
  });

  it('keeps the gray dot for a service that is simply stopped', () => {
    const { queryByTestId, container } = render(ServiceStatusDot, {
      props: { svc: { status: 'inactive' } }
    });
    expect(queryByTestId('service-moon')).toBeNull();
    expect(container.querySelector('.bg-gray-300')).toBeTruthy();
  });

  // A pinned service never sleeps, so the coffee takes the dot's slot and
  // still carries the running or stopped colour.
  it('shows the coffee for a pinned service, coloured by its state', () => {
    idleEnabled.set(true);
    idleServices.set(true);
    const running = render(ServiceStatusDot, { props: { svc: { status: 'active', pinned: true } } });
    expect(running.getByTestId('service-coffee').getAttribute('title')).toBe('Pinned');
    expect(running.getByTestId('service-coffee').className).toContain('text-emerald-500');
    expect(running.container.querySelector('.rounded-full')).toBeNull();
    const stopped = render(ServiceStatusDot, { props: { svc: { status: 'inactive', pinned: true } } });
    expect(within(stopped.container).getByTestId('service-coffee').className).toContain('text-gray-400');
  });

  it('shows the coffee for a service a pinned site keeps awake, naming the site', () => {
    idleEnabled.set(true);
    idleServices.set(true);
    const { getByTestId } = render(ServiceStatusDot, { props: { svc: { status: 'active', kept_awake_by: ['shop', 'blog'] } } });
    expect(getByTestId('service-coffee').getAttribute('title')).toBe('Kept awake by shop, blog');
  });

  // A pin only means something while services can sleep.
  it('keeps the plain dot on a pinned service while services never sleep', () => {
    idleEnabled.set(true);
    idleServices.set(false);
    const { queryByTestId } = render(ServiceStatusDot, { props: { svc: { status: 'active', pinned: true } } });
    expect(queryByTestId('service-coffee')).toBeNull();
  });

  // jsdom has no layout, so the shared footprint is read off the outer box:
  // the same width for both, or a sleeping row's name starts further right.
  it('gives the dot the same footprint as the moon, at either size', () => {
    for (const size of ['sm', 'xs'] as const) {
      const box = (svc: { status: string; idle_suspended?: boolean }) =>
        (render(ServiceStatusDot, { props: { svc, size } }).container.firstElementChild as HTMLElement).className.match(/\bw-\S+ h-\S+/)?.[0];
      expect(box({ status: 'active' })).toBe(box({ status: 'inactive', idle_suspended: true }));
    }
  });
});
