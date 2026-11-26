import { render, screen } from '@testing-library/svelte';
import { describe, it, expect, beforeEach } from 'vitest';
import SnapshotScheduleFields from './SnapshotScheduleFields.svelte';
import { autoSnapshot } from '$stores/autoSnapshot';

describe('SnapshotScheduleFields', () => {
  beforeEach(() => {
    autoSnapshot.set({ enabled: true, every: '24h0m0s', keep: 7, keep_for: '', selection: 'opt_out', sites: [] });
  });

  // The count and its unit share a border, so they have to be one height.
  it('sizes the interval count to match the unit dropdown it is joined to', () => {
    render(SnapshotScheduleFields);
    const count = screen.getByRole('spinbutton');
    const unit = screen.getByRole('button', { name: /day/ });
    expect(unit).toHaveClass('h-[34px]', 'text-sm');
    expect(count).toHaveClass('h-[34px]', 'text-sm');
  });
});
