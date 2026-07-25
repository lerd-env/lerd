import { render, fireEvent, within } from '@testing-library/svelte';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import DatabaseCard from './DatabaseCard.svelte';
import type { DatabaseEngine, DatabaseEntry } from '$stores/databases';

const { dropDatabase, exportUrl, importDatabase } = vi.hoisted(() => ({
  dropDatabase: vi.fn(async () => ({ ok: true })),
  exportUrl: vi.fn((service: string, database: string) => `/api/${service}/export?database=${database}`),
  importDatabase: vi.fn(
    async (
      _service: string,
      _database: string,
      _file: File,
      _onProgress?: (p: { percent: number; uploaded: boolean }) => void
    ): Promise<{
      ok: boolean;
      error?: string;
      errors?: number;
      issues?: { message: string; count: number }[];
      skipped?: { message: string; count: number }[];
      created?: { message: string; count: number }[];
    }> => ({ ok: true })
  )
}));
vi.mock('$stores/databases', async (orig) => {
  const actual = (await orig()) as object;
  return { ...actual, dropDatabase, exportUrl, importDatabase };
});

const engine: DatabaseEngine = {
  service: 'mysql',
  family: 'mysql',
  status: 'active',
  supports_create: true,
  supports_snapshot: true,
  databases: []
};

function db(name: string, size = 0, site?: string, branch?: string): DatabaseEntry {
  return { name, size_bytes: size, site, branch, snapshots: [] };
}

const parent = db('havenly', 4096, 'havenly.test');
const testing = db('havenly_testing', 0, 'havenly.test');

// pickDump drives the hidden file input the import button clicks, then confirms
// the dialog that stands between picking a dump and loading it.
async function pickDump(container: HTMLElement, name = 'shop.sql', fresh = false) {
  const input = container.querySelector('input[type="file"]') as HTMLInputElement;
  Object.defineProperty(input, 'files', { value: [new File(['dump'], name)], configurable: true });
  await fireEvent.change(input);
  if (fresh) await fireEvent.click(container.querySelector('input[type="checkbox"]') as HTMLElement);
  const confirm = [...container.querySelectorAll('button')].filter((b) => b.textContent?.trim() === 'Import');
  await fireEvent.click(confirm[confirm.length - 1]);
}

describe('DatabaseCard', () => {
  beforeEach(() => {
    dropDatabase.mockClear();
    exportUrl.mockClear();
    importDatabase.mockClear();
  });

  it('shows no segment when the entry has no testing sibling', () => {
    const { queryByRole } = render(DatabaseCard, { props: { engine, entry: parent } });
    expect(queryByRole('group')).not.toBeInTheDocument();
  });

  it('opens on the parent database when paired', () => {
    const { getByText, getByRole } = render(DatabaseCard, {
      props: { engine, entry: parent, testing }
    });
    expect(getByText('havenly')).toBeInTheDocument();
    expect(within(getByRole('group')).getByRole('button', { name: 'App' })).toHaveAttribute(
      'aria-pressed',
      'true'
    );
  });

  it('retargets the name, size and site link to the testing database', async () => {
    const { getByRole, getByText, queryByText } = render(DatabaseCard, {
      props: { engine, entry: db('havenly', 4096, 'havenly.test'), testing: db('havenly_testing', 16384, 'havenly.test') }
    });
    await fireEvent.click(within(getByRole('group')).getByRole('button', { name: 'Testing' }));
    expect(getByText('havenly_testing')).toBeInTheDocument();
    expect(queryByText('havenly')).not.toBeInTheDocument();
    expect(getByText('16.0 KB')).toBeInTheDocument();
    expect(getByText('havenly.test')).toBeInTheDocument();
  });

  it('drops only the half the segment points at', async () => {
    const { getByRole, getByLabelText, getAllByRole } = render(DatabaseCard, {
      props: { engine, entry: parent, testing }
    });
    await fireEvent.click(within(getByRole('group')).getByRole('button', { name: 'Testing' }));
    await fireEvent.click(getByLabelText('Drop'));
    const confirm = getAllByRole('button', { name: 'Drop' }).at(-1)!;
    await fireEvent.click(confirm);
    expect(dropDatabase).toHaveBeenCalledWith('mysql', 'havenly_testing');
  });

  it('labels a worktree database with the branch domain it belongs to', () => {
    const { getByRole } = render(DatabaseCard, {
      props: { engine, entry: db('havenly_staging', 2048, 'havenly.test', 'staging') }
    });
    expect(getByRole('button', { name: 'staging.havenly.test' })).toBeInTheDocument();
  });

  it('reports the import as it progresses and confirms it when it lands', async () => {
    let report: ((p: { percent: number; uploaded: boolean }) => void) | null = null;
    let settle: ((r: { ok: boolean; error?: string }) => void) | null = null;
    importDatabase.mockImplementationOnce(
      (_s: string, _d: string, _f: File, onProgress?: (p: { percent: number; uploaded: boolean }) => void) => {
        report = onProgress ?? null;
        return new Promise((resolve) => (settle = resolve));
      }
    );
    const { container, findByText } = render(DatabaseCard, { props: { engine, entry: parent } });
    await pickDump(container);
    report!({ percent: 0.4, uploaded: false });
    expect(await findByText('Importing shop.sql… 40%')).toBeInTheDocument();
    report!({ percent: 1, uploaded: true });
    expect(await findByText('Importing shop.sql…')).toBeInTheDocument();
    settle!({ ok: true });
    expect(await findByText('Imported shop.sql')).toBeInTheDocument();
  });

  it('warns when the engine swallowed the dump but complained', async () => {
    importDatabase.mockResolvedValueOnce({
      ok: true,
      errors: 27458,
      issues: [
        { message: 'invalid command \\N', count: 27331 },
        { message: 'ERROR:  function public.uuid_generate_v4() does not exist', count: 6 }
      ]
    });
    const { container, findAllByText, getByText } = render(DatabaseCard, {
      props: { engine, entry: parent }
    });
    await pickDump(container);
    // The summary shows on the card, the list itself opens over the page.
    expect(
      await findAllByText('Imported shop.sql, but the engine reported 27458 errors')
    ).not.toHaveLength(0);
    expect(getByText('27331×')).toBeInTheDocument();
    expect(getByText(/invalid command/)).toBeInTheDocument();
  });

  it('asks before loading, and does nothing until the dialog is confirmed', async () => {
    const { container, getByText } = render(DatabaseCard, { props: { engine, entry: parent } });
    const input = container.querySelector('input[type="file"]') as HTMLInputElement;
    Object.defineProperty(input, 'files', { value: [new File(['dump'], 'shop.sql')], configurable: true });
    await fireEvent.change(input);
    expect(getByText('Import shop.sql')).toBeInTheDocument();
    expect(importDatabase).not.toHaveBeenCalled();
  });

  it('passes the empty-first choice through to the daemon', async () => {
    const { container } = render(DatabaseCard, { props: { engine, entry: parent } });
    await pickDump(container, 'shop.sql', true);
    expect(importDatabase).toHaveBeenCalledWith('mysql', 'havenly', expect.any(File), expect.any(Function), true);
  });

  it('loads on top of what is there when the box is left unticked', async () => {
    const { container } = render(DatabaseCard, { props: { engine, entry: parent } });
    await pickDump(container);
    expect(importDatabase).toHaveBeenCalledWith('mysql', 'havenly', expect.any(File), expect.any(Function), false);
  });

  it('lists what the daemon held back on the way in', async () => {
    importDatabase.mockResolvedValueOnce({
      ok: true,
      errors: 3,
      issues: [{ message: 'ERROR:  relation "users" already exists', count: 1 }],
      skipped: [{ message: 'ownership and privilege statements the local engine has no roles for', count: 80 }]
    });
    const { container, findByText, getByText } = render(DatabaseCard, { props: { engine, entry: parent } });
    await pickDump(container);
    expect(await findByText(/ownership and privilege statements/)).toBeInTheDocument();
    expect(getByText('80×')).toBeInTheDocument();
  });

  it('names an extension it created for the dump, so nothing changes silently', async () => {
    importDatabase.mockResolvedValueOnce({
      ok: true,
      created: [{ message: 'vector', count: 1 }]
    });
    const { container, findByText } = render(DatabaseCard, { props: { engine, entry: parent } });
    await pickDump(container);
    expect(await findByText('Imported shop.sql, and created vector')).toBeInTheDocument();
  });

  it('surfaces the engine error when an import fails', async () => {
    importDatabase.mockResolvedValueOnce({ ok: false, error: 'import failed: syntax error' });
    const { container, findByText } = render(DatabaseCard, { props: { engine, entry: parent } });
    await pickDump(container);
    expect(await findByText(/import failed: syntax error/)).toBeInTheDocument();
  });

  it('points export at the selected half', async () => {
    const { getByRole, getByLabelText } = render(DatabaseCard, {
      props: { engine, entry: parent, testing }
    });
    await fireEvent.click(within(getByRole('group')).getByRole('button', { name: 'Testing' }));
    expect(getByLabelText('Export')).toHaveAttribute(
      'href',
      expect.stringContaining('havenly_testing')
    );
  });
});
