import { describe, it, expect } from 'vitest';
import { pairDatabases } from './databasePairs';
import type { DatabaseEntry } from '$stores/databases';

function db(name: string, size = 0, site?: string): DatabaseEntry {
  return { name, size_bytes: size, site, snapshots: [] };
}

describe('pairDatabases', () => {
  it('folds a testing database into the parent it belongs to', () => {
    const pairs = pairDatabases([db('havenly', 4096, 'havenly.test'), db('havenly_testing')]);
    expect(pairs).toHaveLength(1);
    expect(pairs[0].entry.name).toBe('havenly');
    expect(pairs[0].testing?.name).toBe('havenly_testing');
  });

  it('pairs regardless of the order the engine returned them in', () => {
    const pairs = pairDatabases([db('havenly_testing'), db('havenly')]);
    expect(pairs).toHaveLength(1);
    expect(pairs[0].entry.name).toBe('havenly');
    expect(pairs[0].testing?.name).toBe('havenly_testing');
  });

  it('keeps a testing database standalone when its parent is absent', () => {
    const pairs = pairDatabases([db('scratch_testing'), db('ledgerly')]);
    expect(pairs.map((p) => p.entry.name)).toEqual(['scratch_testing', 'ledgerly']);
    expect(pairs.every((p) => p.testing === undefined)).toBe(true);
  });

  it('leaves a parent without a testing sibling as a plain entry', () => {
    const pairs = pairDatabases([db('ledgerly', 8192)]);
    expect(pairs).toEqual([{ entry: db('ledgerly', 8192) }]);
  });

  it('preserves the order the engine returned, taking each pair at the parent position', () => {
    const pairs = pairDatabases([
      db('alpha'),
      db('beta_testing'),
      db('beta'),
      db('gamma_testing')
    ]);
    expect(pairs.map((p) => p.entry.name)).toEqual(['alpha', 'beta', 'gamma_testing']);
  });

  it('does not treat the bare suffix as a testing database of an empty name', () => {
    const pairs = pairDatabases([db('_testing')]);
    expect(pairs).toEqual([{ entry: db('_testing') }]);
  });

  it('does not chain a doubly suffixed name past its direct parent', () => {
    const pairs = pairDatabases([db('havenly_testing'), db('havenly_testing_testing')]);
    expect(pairs).toHaveLength(1);
    expect(pairs[0].entry.name).toBe('havenly_testing');
    expect(pairs[0].testing?.name).toBe('havenly_testing_testing');
  });

  it('returns nothing for an empty engine', () => {
    expect(pairDatabases([])).toEqual([]);
  });
});
