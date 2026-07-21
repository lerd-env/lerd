import { render, fireEvent } from '@testing-library/svelte';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import DatabaseSnapshotsModal from './DatabaseSnapshotsModal.svelte';
import type { DatabaseEngine, DatabaseEntry } from '$stores/databases';

type Result = {
  ok: boolean;
  error?: string;
  errors?: number;
  issues?: { message: string; count: number }[];
};
const { restoreSnapshot, takeSnapshot, deleteSnapshot } = vi.hoisted(() => ({
  restoreSnapshot: vi.fn(
    async (): Promise<{
      ok: boolean;
      error?: string;
      errors?: number;
      issues?: { message: string; count: number }[];
    }> => ({ ok: true })
  ),
  takeSnapshot: vi.fn(async (): Promise<{ ok: boolean; error?: string }> => ({ ok: true })),
  deleteSnapshot: vi.fn(async (): Promise<{ ok: boolean; error?: string }> => ({ ok: true }))
}));
vi.mock('$stores/databases', async (orig) => {
  const actual = (await orig()) as object;
  return { ...actual, restoreSnapshot, takeSnapshot, deleteSnapshot };
});

const engine: DatabaseEngine = {
  service: 'mysql',
  family: 'mysql',
  status: 'active',
  supports_create: true,
  supports_snapshot: true,
  databases: []
};

const entry: DatabaseEntry = {
  name: 'havenly',
  size_bytes: 4096,
  snapshots: [{ name: 'before-migrate', created: '2026-07-20T10:00:00Z', database: 'havenly', size_bytes: 2048 }]
};

async function startRestore(getByRole: (role: string, opts: { name: string }) => HTMLElement) {
  await fireEvent.click(getByRole('button', { name: 'Restore' }));
  await fireEvent.click(getByRole('button', { name: 'Confirm restore' }));
}

describe('DatabaseSnapshotsModal', () => {
  beforeEach(() => {
    restoreSnapshot.mockClear();
    takeSnapshot.mockClear();
  });

  it('reports the restore while it runs and confirms it when it lands', async () => {
    let settle: ((r: { ok: boolean; error?: string }) => void) | null = null;
    restoreSnapshot.mockImplementationOnce(() => new Promise((resolve) => (settle = resolve)));
    const { getByRole, findByText } = render(DatabaseSnapshotsModal, {
      props: { engine, entry, onclose: () => {} }
    });
    await startRestore(getByRole as never);
    expect(await findByText('Restoring before-migrate…')).toBeInTheDocument();
    settle!({ ok: true });
    expect(await findByText('Restored before-migrate')).toBeInTheDocument();
  });

  it('warns when the restore ran but the engine complained', async () => {
    const warned: Result = {
      ok: true,
      errors: 25,
      issues: [{ message: 'ERROR:  role "root" does not exist', count: 25 }]
    };
    restoreSnapshot.mockResolvedValueOnce(warned);
    const { getByRole, findAllByText, getByText } = render(DatabaseSnapshotsModal, {
      props: { engine, entry, onclose: () => {} }
    });
    await startRestore(getByRole as never);
    // The status line summarises; the list itself opens over the snapshots modal.
    expect(
      await findAllByText('Restored before-migrate, but the engine reported 25 errors')
    ).not.toHaveLength(0);
    expect(getByText('25×')).toBeInTheDocument();
    expect(getByText(/role "root" does not exist/)).toBeInTheDocument();
  });

  it('surfaces the error when a restore fails', async () => {
    restoreSnapshot.mockResolvedValueOnce({ ok: false, error: 'restore failed: no such table' });
    const { getByRole, findByText } = render(DatabaseSnapshotsModal, {
      props: { engine, entry, onclose: () => {} }
    });
    await startRestore(getByRole as never);
    expect(await findByText(/restore failed: no such table/)).toBeInTheDocument();
  });

  it('reports a snapshot being taken', async () => {
    let settle: ((r: { ok: boolean }) => void) | null = null;
    takeSnapshot.mockImplementationOnce(() => new Promise((resolve) => (settle = resolve)));
    const { getByRole, findByText } = render(DatabaseSnapshotsModal, {
      props: { engine, entry, onclose: () => {} }
    });
    await fireEvent.click(getByRole('button', { name: 'Take snapshot' }));
    expect(await findByText('Taking a snapshot of havenly…')).toBeInTheDocument();
    settle!({ ok: true });
    expect(await findByText('Snapshot taken')).toBeInTheDocument();
  });
});
