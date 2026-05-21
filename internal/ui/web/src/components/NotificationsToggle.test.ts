import { render } from '@testing-library/svelte';
import { describe, it, expect, beforeEach } from 'vitest';
import NotificationsToggle from './NotificationsToggle.svelte';
import { notifyPrefs, permissionState, autoSubscribeDisabled } from '../lib/notify';
import type { NotifyKind } from '../lib/notify';

const allKinds: Record<NotifyKind, boolean> = {
  mail: true,
  worker_failed: true,
  op_done: true,
  update_available: true,
  dump: false
};

describe('NotificationsToggle', () => {
  beforeEach(() => {
    autoSubscribeDisabled.set(false);
  });

  it('shows the on state with a pulsing dot when notifications are live', () => {
    permissionState.set('granted');
    notifyPrefs.set({ enabled: true, kinds: { ...allKinds } });
    const { container } = render(NotificationsToggle);
    expect(container.querySelector('button')!.getAttribute('title')).toMatch(/on,/i);
    expect(container.querySelector('.lerd-pulse-ping')).not.toBeNull();
  });

  it('shows the off state with no pulsing dot', () => {
    permissionState.set('granted');
    notifyPrefs.set({ enabled: false, kinds: { ...allKinds } });
    const { container } = render(NotificationsToggle);
    expect(container.querySelector('button')!.getAttribute('title')).toMatch(/off/i);
    expect(container.querySelector('.lerd-pulse-ping')).toBeNull();
  });

  it('dims and disables the toggle when the browser blocked notifications', () => {
    permissionState.set('denied');
    const { container } = render(NotificationsToggle);
    expect((container.querySelector('button') as HTMLButtonElement).disabled).toBe(true);
    expect(container.querySelector('button')!.getAttribute('title')).toMatch(/block/i);
  });
});
