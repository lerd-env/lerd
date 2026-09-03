import { render, fireEvent, waitFor } from '@testing-library/svelte';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import DatabaseSnapshotsModal from './DatabaseSnapshotsModal.svelte';
import type { DatabaseEngine, DatabaseEntry } from '$stores/databases';
import { autoSnapshot } from '$stores/autoSnapshot';

const { keepSnapshot, loadAutoSnapshot, setSiteAutoSnapshot } = vi.hoisted(() => ({
  keepSnapshot: vi.fn(async (): Promise<{ ok: boolean; error?: string }> => ({ ok: true })),
  loadAutoSnapshot: vi.fn(async () => {}),
  setSiteAutoSnapshot: vi.fn(async () => ({ ok: true }))
}));
vi.mock('$stores/autoSnapshot', async (orig) => {
  const actual = (await orig()) as object;
  return { ...actual, loadAutoSnapshot, setSiteAutoSnapshot };
});
vi.mock('$stores/databases', async (orig) => {
  const actual = (await orig()) as object;
  return { ...actual, keepSnapshot };
});

const engine: DatabaseEngine = {
  service: 'mysql',
  family: 'mysql',
  status: 'active',
  supports_create: true,
  supports_drop: true,
  supports_export: true,
  supports_import: true,
  supports_snapshot: true,
  databases: []
};

const entry: DatabaseEntry = {
  name: 'havenly',
  size_bytes: 4096,
  snapshots: [
    {
      name: 'auto-20260720-100000',
      created: '2026-07-20T10:00:00Z',
      database: 'havenly',
      size_bytes: 2048,
      auto: true,
      expires_at: '2026-07-27T10:00:00Z',
      runs_left: 7,
      estimated: true
    },
    {
      name: 'auto-kept',
      created: '2026-07-19T10:00:00Z',
      database: 'havenly',
      size_bytes: 2048,
      auto: true,
      kept: true
    },
    { name: 'by-hand', created: '2026-07-18T10:00:00Z', database: 'havenly', size_bytes: 2048 }
  ]
};

describe('snapshot retention in the snapshots modal', () => {
  beforeEach(() => {
    keepSnapshot.mockClear();
    setSiteAutoSnapshot.mockClear();
    autoSnapshot.set({
      enabled: true,
      every: '24h0m0s',
      keep: 7,
      keep_for: '',
      selection: 'opt_out',
      sites: [
        {
          site: 'havenly',
          service: 'mysql',
          database: 'havenly',
          mode: '',
          covered: true,
          last: '2026-07-20T10:00:00Z',
          next: '2026-07-21T10:00:00Z'
        }
      ]
    });
  });

  it('says this database is on the schedule, and opts it out from here', async () => {
    const { getByTitle, getByText, queryByText } = render(DatabaseSnapshotsModal, {
      props: { engine, entry, onclose: () => {} }
    });
    // Covered: the block dates the next run instead of reading as disabled.
    expect(queryByText('Disabled')).toBeNull();
    // Each timestamp gets its own row; together on one line they overflow.
    expect(getByText(/^Last:/)).toBeTruthy();
    expect(getByText(/^Next:/)).toBeTruthy();

    await fireEvent.click(getByTitle('Leave this database out of the schedule'));
    await waitFor(() => expect(setSiteAutoSnapshot).toHaveBeenCalledWith('havenly', 'off'));
  });

  it('reads as disabled while the schedule does not cover this database', () => {
    autoSnapshot.update((p) => ({ ...p, sites: p.sites.map((s) => ({ ...s, covered: false })) }));
    const { getByText } = render(DatabaseSnapshotsModal, {
      props: { engine, entry, onclose: () => {} }
    });
    expect(getByText('Disabled')).toBeTruthy();
  });

  // A database nothing points at can never be picked up, and says so rather
  // than offering a control that would do nothing.
  it('says so when no site points at the database', () => {
    autoSnapshot.update((p) => ({ ...p, sites: [] }));
    const { getByText, queryByTitle } = render(DatabaseSnapshotsModal, {
      props: { engine, entry, onclose: () => {} }
    });
    expect(getByText(/No linked site points at this database/)).toBeTruthy();
    expect(queryByTitle(/schedule/)).toBeNull();
  });

  it('says when an automatic snapshot expires, and that a kept one does not', () => {
    const { getByText, queryAllByText } = render(DatabaseSnapshotsModal, {
      props: { engine, entry, onclose: () => {} }
    });
    expect(getByText(/expires around/)).toBeTruthy();
    expect(getByText(/kept for good/)).toBeTruthy();
    // A snapshot taken by hand is never dropped, so it carries no expiry at all.
    expect(queryAllByText(/expires/).length).toBe(1);
  });

  it('offers the keep toggle only on automatic snapshots', () => {
    const { getAllByRole } = render(DatabaseSnapshotsModal, {
      props: { engine, entry, onclose: () => {} }
    });
    const keep = getAllByRole('button', { name: 'Keep for good' });
    const release = getAllByRole('button', { name: 'Put back under retention' });
    expect(keep.length).toBe(1);
    expect(release.length).toBe(1);
  });

  it('keeps a snapshot, and releases one that is already kept', async () => {
    const { getByRole } = render(DatabaseSnapshotsModal, {
      props: { engine, entry, onclose: () => {} }
    });
    await fireEvent.click(getByRole('button', { name: 'Keep for good' }));
    expect(keepSnapshot).toHaveBeenCalledWith('mysql', 'havenly', 'auto-20260720-100000', true);

    await fireEvent.click(getByRole('button', { name: 'Put back under retention' }));
    expect(keepSnapshot).toHaveBeenCalledWith('mysql', 'havenly', 'auto-kept', false);
  });
});
