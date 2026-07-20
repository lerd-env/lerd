import type { DatabaseEntry } from '$stores/databases';

const SUFFIX = '_testing';

export interface DatabasePair {
  entry: DatabaseEntry;
  // The <name>_testing sibling, present only when this entry's own database exists.
  testing?: DatabaseEntry;
}

// pairDatabases folds each "<name>_testing" database into the card of the database
// it tests. A testing database whose parent isn't there keeps its own entry, so
// nothing an engine reported ever disappears from the grid.
export function pairDatabases(list: DatabaseEntry[]): DatabasePair[] {
  const names = new Set(list.map((e) => e.name));
  const parentOf = (name: string): string | null => {
    if (!name.endsWith(SUFFIX)) return null;
    const parent = name.slice(0, -SUFFIX.length);
    return parent !== '' && names.has(parent) ? parent : null;
  };

  const pairs: DatabasePair[] = [];
  const index = new Map<string, DatabasePair>();
  for (const entry of list) {
    if (parentOf(entry.name)) continue;
    const pair: DatabasePair = { entry };
    pairs.push(pair);
    index.set(entry.name, pair);
  }
  for (const entry of list) {
    const parent = parentOf(entry.name);
    if (parent) index.get(parent)!.testing = entry;
  }
  return pairs;
}
