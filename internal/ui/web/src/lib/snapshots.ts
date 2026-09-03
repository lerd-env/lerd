// Snapshot names carry a trailing UTC stamp, e.g. "nightly-20260719-135558".
// These helpers read the time back off the name and strip it for display.
const TS_RE = /-(\d{4})(\d{2})(\d{2})-(\d{2})(\d{2})(\d{2})$/;

// parseSnapshotTimestamp returns the time encoded in a snapshot name, or null
// when the name carries no stamp (e.g. a snapshot taken before this convention).
export function parseSnapshotTimestamp(name: string): Date | null {
  const m = TS_RE.exec(name);
  if (!m) return null;
  const [, y, mo, d, h, mi, s] = m;
  const t = Date.UTC(+y, +mo - 1, +d, +h, +mi, +s);
  return Number.isNaN(t) ? null : new Date(t);
}

// snapshotBaseName drops the trailing timestamp so the list shows the name the
// user typed (or "snapshot" for an auto-generated one).
export function snapshotBaseName(name: string): string {
  return name.replace(TS_RE, '');
}

// The schedule is stored as a Go duration; the UI speaks a count and a unit, so
// 168h reads as one week rather than 168 hours.
export const SNAPSHOT_UNIT_HOURS = { hour: 1, day: 24, week: 168, month: 720 } as const;
export type SnapshotUnit = keyof typeof SNAPSHOT_UNIT_HOURS;

// intervalParts splits a stored interval into the largest unit it divides into
// evenly. A hand-written value that is not a whole number of hours falls back to
// the daily default, which the next save normalises.
export function intervalParts(every: string): { amount: number; unit: SnapshotUnit } {
  const match = /^(\d+)h/.exec(every);
  const hours = match ? Number(match[1]) : 24;
  for (const unit of ['month', 'week', 'day'] as SnapshotUnit[]) {
    const size = SNAPSHOT_UNIT_HOURS[unit];
    if (hours >= size && hours % size === 0) return { amount: hours / size, unit };
  }
  return { amount: Math.max(1, hours), unit: 'hour' };
}

// intervalDuration is the inverse, clamped to what the picker allows.
export function intervalDuration(amount: number, unit: SnapshotUnit): string {
  const n = Math.min(999, Math.max(1, Math.round(amount || 1)));
  return n * SNAPSHOT_UNIT_HOURS[unit] + 'h';
}
