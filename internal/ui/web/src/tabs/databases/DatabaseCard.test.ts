import { render, fireEvent, within } from '@testing-library/svelte';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import DatabaseCard from './DatabaseCard.svelte';
import type { DatabaseEngine, DatabaseEntry } from '$stores/databases';

const { dropDatabase, exportUrl } = vi.hoisted(() => ({
  dropDatabase: vi.fn(async () => ({ ok: true })),
  exportUrl: vi.fn((service: string, database: string) => `/api/${service}/export?database=${database}`)
}));
vi.mock('$stores/databases', async (orig) => {
  const actual = (await orig()) as object;
  return { ...actual, dropDatabase, exportUrl };
});

const engine: DatabaseEngine = {
  service: 'mysql',
  family: 'mysql',
  status: 'active',
  supports_create: true,
  supports_snapshot: true,
  databases: []
};

function db(name: string, size = 0, site?: string): DatabaseEntry {
  return { name, size_bytes: size, site, snapshots: [] };
}

const parent = db('havenly', 4096, 'havenly.test');
const testing = db('havenly_testing', 0, 'havenly.test');

describe('DatabaseCard', () => {
  beforeEach(() => {
    dropDatabase.mockClear();
    exportUrl.mockClear();
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
