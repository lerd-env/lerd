import { render } from '@testing-library/svelte';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import ResourcesWidget from './ResourcesWidget.svelte';
import { disk, type DiskSnapshot } from '$stores/disk';
import { stats, statsLoaded, type StatsResponse } from '$stores/stats';
import { tick } from 'svelte';

// The widget polls both endpoints on mount; a failed fetch would reset the very
// stores the test just seeded, so only the pollers are stubbed out.
vi.mock('$stores/stats', async (orig) => ({
  ...(await orig<typeof import('$stores/stats')>()),
  startStatsPolling: () => () => {}
}));
vi.mock('$stores/disk', async (orig) => ({
  ...(await orig<typeof import('$stores/disk')>()),
  startDiskPolling: () => () => {}
}));

function snap(over: Partial<DiskSnapshot> = {}): DiskSnapshot {
  return {
    available: true,
    used_by_lerd_bytes: 0,
    used_images: [],
    reclaimable_bytes: 0,
    lerd_bytes: 0,
    other_bytes: 0,
    images: [],
    held_bytes: 0,
    held_count: 0,
    ...over
  };
}

describe('ResourcesWidget disk block', () => {
  beforeEach(() => {
    statsLoaded.set(true);
    stats.set({
      containers: [],
      total_cpu_percent: 0,
      total_mem_bytes: 0,
      host_mem_bytes: 0,
      updated_at: '',
      available: true
    } satisfies StatsResponse);
    disk.set(snap());
  });

  // What lerd occupies is worth showing on its own: a user with nothing to
  // reclaim still wants to know the cost of the install.
  it('shows what lerd uses even when there is nothing to reclaim', () => {
    disk.set(snap({ used_by_lerd_bytes: 5 * 1024 ** 3 }));
    const { getByText, queryByText } = render(ResourcesWidget);
    expect(getByText('Disk')).toBeTruthy();
    expect(queryByText('Clean up')).toBeNull();
  });

  // Nothing to reclaim is the healthy state, and a zero next to a real number
  // reads as a problem, so the whole column goes rather than showing 0 B.
  it('hides the reclaimable figure and the button when there is nothing to reclaim', () => {
    disk.set(snap({ used_by_lerd_bytes: 5 * 1024 ** 3 }));
    const { queryByText } = render(ResourcesWidget);
    expect(queryByText('Reclaimable disk')).toBeNull();
    expect(queryByText('Clean up')).toBeNull();
  });

  it('opens the usage breakdown from the used figure', async () => {
    disk.set(
      snap({
        used_by_lerd_bytes: 900,
        used_images: [{ ref: 'lerd-php84-fpm:local', in_use: true, bytes: 900 }]
      })
    );
    const { getByText, queryByText } = render(ResourcesWidget);
    expect(queryByText('lerd-php84-fpm:local')).toBeNull();
    getByText('900 B').click();
    await tick();
    expect(getByText('lerd-php84-fpm:local')).toBeTruthy();
  });

  it('offers the button and the owner split once both sides have something', () => {
    disk.set(
      snap({
        used_by_lerd_bytes: 5 * 1024 ** 3,
        reclaimable_bytes: 3 * 1024 ** 3,
        lerd_bytes: 1 * 1024 ** 3,
        other_bytes: 2 * 1024 ** 3
      })
    );
    const { getByText } = render(ResourcesWidget);
    expect(getByText('Clean up')).toBeTruthy();
    expect(getByText('1.00 GB lerd, 2.00 GB other')).toBeTruthy();
  });

  it('stays hidden while the scan is unavailable', () => {
    disk.set(snap({ available: false, used_by_lerd_bytes: 5 * 1024 ** 3 }));
    const { queryByText } = render(ResourcesWidget);
    expect(queryByText('Disk')).toBeNull();
  });
});
