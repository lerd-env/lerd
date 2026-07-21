import { render, fireEvent } from '@testing-library/svelte';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { get } from 'svelte/store';
import { tick } from 'svelte';
import NotificationToasts from './NotificationToasts.svelte';
import { inAppNotifications } from '$lib/notify';

function push(over: Partial<{ id: number; kind: string; title: string; body: string; url: string; failed: boolean }> = {}) {
  inAppNotifications.update((list) => [
    ...list,
    { id: list.length + 1, kind: 'op_done', title: 'Update finished', body: '', url: '', failed: false, ...over }
  ]);
}

describe('NotificationToasts', () => {
  beforeEach(() => {
    inAppNotifications.set([]);
    vi.useRealTimers();
  });

  it('renders nothing when there is no notification', () => {
    const { container } = render(NotificationToasts);
    expect(container.textContent).toBe('');
  });

  it('shows a failure as an alert and keeps it until dismissed', async () => {
    vi.useFakeTimers();
    const { getByRole, getByLabelText } = render(NotificationToasts);
    push({ title: 'Migrate failed: mariadb-11-4', body: 'exit 127', failed: true });
    await tick();

    expect(getByRole('alert')).toHaveTextContent('Migrate failed: mariadb-11-4');
    vi.advanceTimersByTime(30000);
    expect(getByRole('alert')).toBeInTheDocument();

    await fireEvent.click(getByLabelText('Close'));
    expect(get(inAppNotifications)).toHaveLength(0);
  });

  it('clears an informational notification on its own', async () => {
    vi.useFakeTimers();
    render(NotificationToasts);
    push({ title: 'Update finished: mysql' });
    await tick();

    await vi.advanceTimersByTimeAsync(6500);
    expect(get(inAppNotifications)).toHaveLength(0);
  });
});
