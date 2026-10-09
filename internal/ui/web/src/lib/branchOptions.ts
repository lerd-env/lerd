import { relativeTime } from './relativeTime';

// branchOptions lists the branches for a branch picker, most recently
// committed first so the active ones lead, each saying when it last moved.
// Branches git gave no date for go last; ties keep local before remote.
export function branchOptions(
  local: string[],
  remote: string[],
  dates: Record<string, number> = {},
  now = Date.now()
): { value: string; label: string; description: string }[] {
  const when = (b: string) => (dates[b] ? relativeTime(dates[b] * 1000, now) : '');
  const all = [
    ...local.map((b) => ({ value: b, label: b, description: when(b) })),
    ...remote.map((b) => ({ value: b, label: b, description: ['remote', when(b)].filter(Boolean).join(' · ') }))
  ];
  return all.sort((a, b) => (dates[b.value] ?? 0) - (dates[a.value] ?? 0));
}
