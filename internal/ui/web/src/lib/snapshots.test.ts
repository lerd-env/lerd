import { describe, it, expect } from 'vitest';
import { parseSnapshotTimestamp, snapshotBaseName } from './snapshots';

describe('parseSnapshotTimestamp', () => {
  it('reads the UTC time off a stamped name', () => {
    const d = parseSnapshotTimestamp('nightly-20260719-135558');
    expect(d?.getTime()).toBe(Date.UTC(2026, 6, 19, 13, 55, 58));
  });

  it('handles an auto-generated name', () => {
    const d = parseSnapshotTimestamp('snapshot-20260101-000000');
    expect(d?.getTime()).toBe(Date.UTC(2026, 0, 1, 0, 0, 0));
  });

  it('returns null for an unstamped name', () => {
    expect(parseSnapshotTimestamp('legacy-backup')).toBeNull();
  });
});

describe('snapshotBaseName', () => {
  it('strips the trailing timestamp', () => {
    expect(snapshotBaseName('nightly-20260719-135558')).toBe('nightly');
    expect(snapshotBaseName('snapshot-20260101-000000')).toBe('snapshot');
  });

  it('leaves an unstamped name untouched', () => {
    expect(snapshotBaseName('legacy-backup')).toBe('legacy-backup');
  });
});
