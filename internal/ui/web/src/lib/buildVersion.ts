export type BuildChannel = 'release' | 'beta' | 'dev';

export interface BuildVersion {
  channel: BuildChannel;
  // The tag the build sits on, without its pre-release suffix; null for a bare hash.
  base: string | null;
  commit: string | null;
}

// Versions come from `git describe --tags --always --dirty`: a clean tag is a
// release or beta, anything past a tag or dirty is a dev build.
export function parseBuildVersion(v: string): BuildVersion {
  const bare = v.match(/^([0-9a-f]{7,})(?:-dirty)?$/);
  if (bare) return { channel: 'dev', base: null, commit: bare[1] };
  const base = v.split('-')[0];
  const past = v.match(/-\d+-g([0-9a-f]{7,})(?:-dirty)?$/);
  if (past) return { channel: 'dev', base, commit: past[1] };
  if (v.endsWith('-dirty')) return { channel: 'dev', base, commit: null };
  if (/-beta\.\d+$/.test(v)) return { channel: 'beta', base, commit: null };
  return { channel: 'release', base: v, commit: null };
}
