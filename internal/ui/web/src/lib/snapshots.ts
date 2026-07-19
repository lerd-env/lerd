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
