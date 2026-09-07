import { render, fireEvent, waitFor } from '@testing-library/svelte';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import SnapshotSettingsModal from './SnapshotSettingsModal.svelte';
import DatabaseSnapshotsModal from '../tabs/databases/DatabaseSnapshotsModal.svelte';
import { autoSnapshot, type AutoSnapshotPolicy } from '$stores/autoSnapshot';
import type { DatabaseEngine, DatabaseEntry } from '$stores/databases';

const policy = (enabled: boolean): AutoSnapshotPolicy => ({
  enabled,
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
      covered: enabled,
      last: '2026-07-20T10:00:00Z',
      next: '2026-07-21T10:00:00Z'
    }
  ]
});

// The daemon answers a policy write with the stored policy, which is what the
// store adopts; these tests drive the real store through a stubbed fetch.
const fetchMock = vi.fn(async (_url: string, init?: RequestInit) => {
  const method = (init?.method ?? 'GET').toUpperCase();
  const body = method === 'GET' ? policy(false) : policy(true);
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { 'Content-Type': 'application/json' }
  });
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
const entry: DatabaseEntry = { name: 'havenly', size_bytes: 4096, snapshots: [] };

describe('SnapshotSettingsModal', () => {
  beforeEach(() => {
    fetchMock.mockClear();
    vi.stubGlobal('fetch', fetchMock);
    autoSnapshot.set(policy(false));
  });
  afterEach(() => vi.unstubAllGlobals());

  it('reflects the enabled schedule as soon as the write lands', async () => {
    const { getByTitle } = render(SnapshotSettingsModal, { props: { onclose: () => {} } });
    // The toggle states itself through its track colour, so that is what moving.
    expect(getByTitle('Automatic snapshots').className).toContain('bg-gray-300');

    await fireEvent.click(getByTitle('Automatic snapshots'));
    await waitFor(() =>
      expect(getByTitle('Automatic snapshots').className).toContain('bg-lerd-red')
    );
  });

  // The dialog is opened straight off a database card, so the first render can
  // land before the policy has arrived: the block must fill in on its own
  // rather than waiting for the dialog to be closed and opened again.
  it('fills in the per-database block when the policy arrives after the first render', async () => {
    autoSnapshot.set({ enabled: false, every: '24h0m0s', keep: 7, keep_for: '', selection: 'opt_out', sites: [] });
    const { queryByTitle, findByTitle } = render(DatabaseSnapshotsModal, {
      props: { engine, entry, onclose: () => {} }
    });
    expect(queryByTitle(/schedule/)).toBeNull();
    expect(await findByTitle(/schedule/)).toBeTruthy();
  });

  // Opt-in and opt-out are the same policy read two ways, so the control has to
  // say which one is in force and send the other.
  it('switches the selection mode', async () => {
    const { getByRole, getByText } = render(SnapshotSettingsModal, { props: { onclose: () => {} } });
    expect(getByText(/Every database is snapshotted unless/)).toBeTruthy();

    await fireEvent.click(getByRole('button', { name: 'Opt in' }));
    await waitFor(() => {
      const sent = JSON.parse(String(fetchMock.mock.calls.at(-1)?.[1]?.body));
      expect(sent.selection).toBe('opt_in');
    });
  });

  // Opened over a database's snapshots dialog, the write has to reach that
  // dialog too: its coverage line is read from the same policy.
  it('updates the dialog underneath it', async () => {
    const { getByLabelText, getByTitle, queryByText } = render(DatabaseSnapshotsModal, {
      props: { engine, entry, onclose: () => {} }
    });
    expect(queryByText('Disabled')).toBeTruthy();

    await fireEvent.click(getByLabelText('Snapshot settings'));
    // The schedule's own switch keeps the policy title; the dialog's per-database
    // one is named for the database, so there is no ambiguity here.
    await fireEvent.click(getByTitle('Automatic snapshots'));
    await waitFor(() => expect(queryByText('Disabled')).toBeNull());
  });
});
