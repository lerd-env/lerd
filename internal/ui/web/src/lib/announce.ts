import { writable } from 'svelte/store';

// Text for the app's one polite live region. Screen readers read whatever lands
// here, so a change the page shows only visually (a dot turning grey) is heard too.
export const announcement = writable('');

export function announce(text: string) {
  announcement.set(text);
}

interface StatusItem {
  name: string;
  status: string;
}

export interface StatusChange {
  name: string;
  running: boolean;
}

// serviceStatusChanges lists the services whose running state flipped between
// two snapshots. A service new to the list is skipped so the first load is not
// read out, and skip() lets the caller leave out workers, which idle-suspend
// starts and stops all day.
export function serviceStatusChanges<T extends StatusItem>(
  before: T[],
  after: T[],
  skip: (s: T) => boolean = () => false
): StatusChange[] {
  const was = new Map(before.map((s) => [s.name, s.status === 'active']));
  const changes: StatusChange[] = [];
  for (const s of after) {
    if (skip(s) || !was.has(s.name)) continue;
    const running = s.status === 'active';
    if (was.get(s.name) !== running) changes.push({ name: s.name, running });
  }
  return changes;
}
