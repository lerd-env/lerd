import { render, fireEvent } from '@testing-library/svelte';
import { describe, it, expect, beforeEach } from 'vitest';
import { get } from 'svelte/store';
import NotificationCenter from './NotificationCenter.svelte';
import { notificationHistory, unreadNotifications, type NotificationRecord } from '$lib/notify';

function rec(over: Partial<NotificationRecord> = {}): NotificationRecord {
  return {
    id: 1,
    kind: 'op_failed',
    title: 'Migrate failed: mariadb-11-4',
    body: 'exit 127',
    url: '#services/mariadb-11-4',
    failed: true,
    at: Date.now(),
    read: false,
    ...over
  };
}

describe('NotificationCenter', () => {
  beforeEach(() => {
    notificationHistory.set([]);
  });

  it('badges the unread count and clears it when the panel is opened', async () => {
    notificationHistory.set([rec(), rec({ id: 2, title: 'Update finished: mysql', failed: false })]);
    const { getByLabelText, getByText } = render(NotificationCenter);
    expect(get(unreadNotifications)).toBe(2);

    await fireEvent.click(getByLabelText('Notifications'));
    expect(getByText('Migrate failed: mariadb-11-4')).toBeInTheDocument();
    expect(get(unreadNotifications)).toBe(0);
  });

  it('keeps a missed failure readable after the toast is gone', async () => {
    notificationHistory.set([rec()]);
    const { getByLabelText, getByText } = render(NotificationCenter);
    await fireEvent.click(getByLabelText('Notifications'));
    expect(getByText('exit 127')).toBeInTheDocument();
  });

  it('empties the list on clear', async () => {
    notificationHistory.set([rec()]);
    const { getByLabelText, getByText } = render(NotificationCenter);
    await fireEvent.click(getByLabelText('Notifications'));
    await fireEvent.click(getByText('Clear'));
    expect(get(notificationHistory)).toHaveLength(0);
  });
});

describe('NotificationCenter severity', () => {
  beforeEach(() => {
    notificationHistory.set([]);
  });

  it('marks a detected problem as a warning in the list', async () => {
    notificationHistory.set([
      rec({ kind: 'nplusone', title: 'Possible N+1 query on acme', failed: false })
    ]);
    const { getByLabelText } = render(NotificationCenter);
    await fireEvent.click(getByLabelText('Notifications'));
    expect(document.body.querySelector('.text-amber-500')).toBeTruthy();
  });
});
