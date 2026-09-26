import { render, fireEvent } from '@testing-library/svelte';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import ResourcesWidget from './ResourcesWidget.svelte';
import { stats, statsLoaded } from '$stores/stats';
import { accessMode } from '$stores/accessMode';

vi.mock('$stores/stats', async (orig) => ({
  ...(await orig<typeof import('$stores/stats')>()),
  startStatsPolling: () => () => {}
}));
vi.mock('$stores/disk', async (orig) => ({
  ...(await orig<typeof import('$stores/disk')>()),
  startDiskPolling: () => () => {}
}));
const serviceAction = vi.fn(async () => true);
vi.mock('$stores/services', async (orig) => ({
  ...(await orig<typeof import('$stores/services')>()),
  serviceAction: (...a: unknown[]) => serviceAction(...(a as []))
}));

const row = (name: string, orphaned = false) => ({
  name,
  cpu_percent: 0.1,
  mem_bytes: 1024,
  mem_limit_bytes: 0,
  mem_percent: 0,
  orphaned
});

describe('ResourcesWidget orphaned containers', () => {
  beforeEach(() => {
    serviceAction.mockClear();
    statsLoaded.set(true);
    accessMode.set({ localControl: true, lanExposed: false, checked: true });
    stats.set({
      containers: [row('lerd-phpmyadmin', true), row('lerd-mysql')],
      total_cpu_percent: 0.2,
      total_mem_bytes: 2048,
      host_mem_bytes: 0,
      updated_at: '',
      available: true
    });
  });

  it('lists orphans apart from the running containers and removes one with the trash button', async () => {
    const { getByText, getByRole, container } = render(ResourcesWidget);
    const heading = getByText('orphaned');
    const section = heading.parentElement!;
    expect(section.textContent).toContain('phpmyadmin');
    expect(section.textContent).not.toContain('mysql');
    const top = container.querySelector('.max-h-44')!;
    expect(top.textContent).toContain('mysql');
    expect(top.textContent).not.toContain('phpmyadmin');

    const trash = getByRole('button', { name: 'Remove phpmyadmin' });
    expect(trash.className).toContain('w-7');
    await fireEvent.click(trash);
    expect(serviceAction).toHaveBeenCalledWith('phpmyadmin', 'remove');
  });

  it('offers no remove without local control', () => {
    accessMode.set({ localControl: false, lanExposed: true, checked: true });
    const { getByText, queryByRole } = render(ResourcesWidget);
    expect(getByText('orphaned')).toBeTruthy();
    expect(queryByRole('button', { name: 'Remove phpmyadmin' })).toBeNull();
  });
});
